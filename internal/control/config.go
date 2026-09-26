// Package control implements the Ubuntu entry prototype. It never interprets
// Codex process counts as work state and never creates a replacement thread.
package control

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

const Version = "0.2.0"

type Identity struct {
	UID       string `json:"uid"`
	User      string `json:"user"`
	Home      string `json:"home"`
	Machine   string `json:"machineIdHash"`
	CodexHome string `json:"codexHome"`
}
type Config struct {
	Schema        int      `json:"schemaVersion"`
	Label         string   `json:"label"`
	Host          string   `json:"sshAlias"`
	SSHConfig     string   `json:"sshConfig"`
	SSHConfigHash string   `json:"sshConfigHash"`
	ProjectsRoot  string   `json:"projectsRoot"`
	Codex         string   `json:"codexPath"`
	Identity      Identity `json:"identity"`
}

func DefaultDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "codex-phone-ops", "ubuntu")
}
func StateDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".local", "state", "codex-phone-ops", "ubuntu")
}
func Expand(s string) string {
	h, _ := os.UserHomeDir()
	if s == "~" {
		return h
	}
	if strings.HasPrefix(s, "~/") {
		return filepath.Join(h, s[2:])
	}
	return s
}
func HashFile(p string) (string, error) {
	b, e := os.ReadFile(p)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func ValidText(s string) bool { return !strings.ContainsAny(s, "\x00\r\n") }
func ValidAbs(s string) bool  { return filepath.IsAbs(s) && ValidText(s) }

var aliasRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func (c Config) Validate() error {
	if c.Schema != 1 {
		return errors.New("未対応の設定版です。上書きせず終了します")
	}
	if !aliasRE.MatchString(c.Host) {
		return errors.New("SSH aliasは英数字・_・-・.のみ指定できます")
	}
	if !ValidAbs(c.SSHConfig) || !ValidAbs(c.ProjectsRoot) || !ValidAbs(c.Codex) || !ValidAbs(c.Identity.Home) || !ValidAbs(c.Identity.CodexHome) {
		return errors.New("設定には改行のない絶対パスが必要です")
	}
	if c.Identity.UID == "" || c.Identity.User == "" || c.Identity.Machine == "" || c.SSHConfigHash == "" {
		return errors.New("接続先の登録情報が不完全です。setupから登録してください")
	}
	return nil
}
func Load(dir string) (Config, error) {
	var c Config
	b, e := os.ReadFile(filepath.Join(dir, "config.json"))
	if e != nil {
		return c, e
	}
	if len(b) > 65536 {
		return c, errors.New("設定が大きすぎます")
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return c, errors.New("設定を読めません。既存ファイルは変更していません")
	}
	return c, c.Validate()
}
func (c Config) TrustConfig() error {
	h, e := HashFile(c.SSHConfig)
	if e != nil {
		return errors.New("登録済みSSH設定を読めません")
	}
	if h != c.SSHConfigHash {
		return errors.New("SSH設定が登録時から変わりました。内容を確認してsetupで再登録してください")
	}
	return nil
}
func SaveConfig(dir string, c Config) error {
	if e := c.Validate(); e != nil {
		return e
	}
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	return AtomicWrite(filepath.Join(dir, "config.json"), append(b, '\n'))
}
func AtomicWrite(path string, b []byte) error {
	d := filepath.Dir(path)
	if e := os.MkdirAll(d, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(d, ".pending-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Rename(tmp, path); e != nil {
		return e
	}
	df, e := os.Open(d)
	if e != nil {
		return e
	}
	defer df.Close()
	return df.Sync()
}
func Lock(dir string) (func(), error) {
	if e := os.MkdirAll(dir, 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(dir, "instance.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("別のCodex作業画面が開いています。そちらへ戻ってください")
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
