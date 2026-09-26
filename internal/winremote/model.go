// Package winremote provides read-only Windows recovery observations.
package winremote

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tsutomu-n/codex-phone-ops/internal/control"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	Schema     int    `json:"schemaVersion"`
	Label      string `json:"label"`
	Alias      string `json:"sshAlias"`
	SSHConfig  string `json:"sshConfig"`
	SSHHash    string `json:"sshConfigHash"`
	Host       string `json:"expectedHost"`
	User       string `json:"expectedUser"`
	SID        string `json:"sid"`
	Machine    string `json:"machine"`
	AppID      string `json:"appId"`
	RDPName    string `json:"rdpName"`
	RDPAddress string `json:"rdpAddress"`
	Purpose    string `json:"purpose"`
}

func ConfigDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config", "codex-phone-ops", "win11")
}
func StateDir() string {
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".local", "state", "codex-phone-ops", "win11")
}
func (c Config) Validate(registered bool) error {
	if c.Schema != 1 || !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`).MatchString(c.Alias) || !control.ValidAbs(c.SSHConfig) {
		return errors.New("Windows設定の版・SSH alias・絶対パスを確認してください")
	}
	for _, s := range []string{c.Label, c.Host, c.User, c.SID, c.Machine, c.AppID, c.RDPName, c.RDPAddress, c.Purpose} {
		if !control.ValidText(s) || len(s) > 2048 || control.Safe(s) != s {
			return errors.New("設定に未対応の文字があります")
		}
	}
	if c.Host == "" || c.User == "" || c.RDPName == "" || c.RDPAddress == "" || c.Purpose == "" || c.SSHHash == "" || (registered && c.SID == "") {
		return errors.New("対象情報が不足しています")
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
		return c, errors.New("設定を解析できません。cardは利用できます")
	}
	return c, c.Validate(true)
}
func save(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return control.AtomicWrite(path, append(b, '\n'))
}
func (c Config) Trust() error {
	h, e := control.HashFile(c.SSHConfig)
	if e != nil || h != c.SSHHash {
		return &control.RunError{Kind: "ssh_trust", Message: "SSH設定が登録時と異なるか読めません。内容を確認しsetupで再登録してください"}
	}
	return nil
}

type Check[T any] struct {
	Status       string `json:"status"`
	Completeness string `json:"completeness"`
	Value        *T     `json:"value"`
}

func (c Check[T]) Full() bool {
	return c.Status == "observed" && c.Completeness == "full" && c.Value != nil
}
func (c Check[T]) valid() bool {
	return (c.Status == "observed" || c.Status == "denied" || c.Status == "unsupported" || c.Status == "unavailable") && (c.Completeness == "full" || c.Completeness == "partial") && (c.Status != "observed" || c.Value != nil)
}

type Identity struct {
	Host string `json:"host"`
	User string `json:"user"`
	SID  string `json:"sid"`
}
type Machine struct {
	Hash string `json:"hash"`
	Boot string `json:"boot"`
}
type Session struct {
	ID    int    `json:"id"`
	State string `json:"state"`
	SID   string `json:"sid"`
}
type Candidate struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}
type App struct {
	ID        string `json:"id"`
	Running   bool   `json:"running"`
	SessionID int    `json:"sessionId"`
	SID       string `json:"sid"`
}
type RDP struct {
	Service   string `json:"service"`
	Port      int    `json:"port"`
	Listening bool   `json:"listening"`
}
type Observation struct {
	Schema      int                `json:"schemaVersion"`
	RequestID   string             `json:"requestId"`
	CapturedAt  time.Time          `json:"capturedAt"`
	Identity    Check[Identity]    `json:"identity"`
	Machine     Check[Machine]     `json:"machine"`
	Sessions    Check[[]Session]   `json:"sessions"`
	Candidates  Check[[]Candidate] `json:"candidates"`
	App         Check[App]         `json:"app"`
	RDP         Check[RDP]         `json:"rdp"`
	Tailscale   Check[string]      `json:"tailscale"`
	Certificate Check[string]      `json:"certificate"`
}

func Parse(b []byte, id string) (Observation, error) {
	var o Observation
	b = []byte(strings.TrimPrefix(string(b), "\ufeff"))
	if len(b) > 256*1024 || json.Unmarshal(b, &o) != nil || o.Schema != 1 || o.RequestID != id || o.CapturedAt.IsZero() {
		return o, &control.RunError{Kind: "diagnostic_parse", Message: "診断応答の版・要求ID・形式が不正です。以前の良好状態では補いません"}
	}
	if !o.Identity.valid() || !o.Machine.valid() || !o.Sessions.valid() || !o.Candidates.valid() || !o.App.valid() || !o.RDP.valid() || !o.Tailscale.valid() || !o.Certificate.valid() {
		return Observation{}, &control.RunError{Kind: "diagnostic_parse", Message: "診断項目が欠落・不正です"}
	}
	if o.Identity.Full() && (o.Identity.Value.Host == "" || o.Identity.Value.User == "" || o.Identity.Value.SID == "") {
		return Observation{}, errors.New("diagnostic_parse: identityが不完全です")
	}
	if o.Sessions.Full() {
		seen := map[int]bool{}
		for _, session := range *o.Sessions.Value {
			if session.ID < 0 || session.SID == "" || seen[session.ID] || (session.State != "Active" && session.State != "Disconnected" && session.State != "Other") {
				return Observation{}, errors.New("diagnostic_parse: sessionの形式が不正です")
			}
			seen[session.ID] = true
		}
	}
	if o.App.Full() && (o.App.Value.ID == "" || o.App.Value.SID == "" || (o.App.Value.Running && o.App.Value.SessionID < 0)) {
		return Observation{}, errors.New("diagnostic_parse: appの形式が不正です")
	}
	if o.RDP.Full() && (o.RDP.Value.Port < 1 || o.RDP.Value.Port > 65535 || o.RDP.Value.Service == "") {
		return Observation{}, errors.New("diagnostic_parse: RDPの形式が不正です")
	}
	return o, nil
}
func Verify(c Config, o Observation) error {
	if !o.Identity.Full() {
		return errors.New("identity_unknown: 接続主体を確認できません。自動判定を止めます")
	}
	i := o.Identity.Value
	if !strings.EqualFold(i.Host, c.Host) || !strings.EqualFold(i.User, c.User) || (c.SID != "" && c.SID != i.SID) || (c.Machine != "" && (!o.Machine.Full() || o.Machine.Value.Hash != c.Machine)) {
		return &control.RunError{Kind: "ssh_trust", Message: "登録したホスト・ユーザー・SID・machine identityと一致しません。迂回せず対象を照合してください"}
	}
	return nil
}

type Summary struct {
	State      string            `json:"state"`
	Message    string            `json:"message"`
	Next       string            `json:"next"`
	ObservedAt time.Time         `json:"observedAt"`
	Checks     map[string]string `json:"checks"`
}

func status[T any](c Check[T]) string {
	if c.Full() {
		return "observed"
	}
	return "UNKNOWN (" + c.Status + "/" + c.Completeness + ")"
}
func Decide(c Config, o Observation) Summary {
	s := Summary{State: "unknown", Message: "PCには接続済み。対象のログイン・アプリ状態は未確認です。", Next: "保存済みRDPで対象を目視確認してください。", ObservedAt: o.CapturedAt, Checks: map[string]string{"SSH": "observed", "identity": status(o.Identity), "sessions": status(o.Sessions), "app": status(o.App), "candidates": status(o.Candidates), "RDP": status(o.RDP), "machine": status(o.Machine), "Tailscale": status(o.Tailscale), "certificate": status(o.Certificate)}}
	if o.RDP.Full() {
		s.Checks["RDP"] = fmt.Sprintf("service=%s / port=%d / listener=%t（経路・認証は未確認）", o.RDP.Value.Service, o.RDP.Value.Port, o.RDP.Value.Listening)
	}
	if o.Sessions.Full() {
		states := []string{}
		for _, session := range *o.Sessions.Value {
			states = append(states, session.State)
		}
		s.Checks["sessions"] = fmt.Sprintf("observed / %d sessions / %s", len(states), strings.Join(states, ", "))
	}
	if e := Verify(c, o); e != nil {
		s.State = "trust_unknown"
		s.Message = e.Error()
		s.Next = "接続先を照合してください。警告を無視してRDPへ迂回しないでください。"
		return s
	}
	if !o.Sessions.Full() {
		return s
	}
	sessions := *o.Sessions.Value
	for _, v := range sessions {
		if v.SID != c.SID {
			return s
		}
	}
	if len(sessions) == 0 {
		s.State = "gui_absent"
		s.Message = "対象Windowsには接続できています。画面ログインが必要です。"
		s.Next = "保存済みRDPでログインし、対象ChatGPTを起動してください。"
		return s
	}
	if c.AppID == "" || !o.App.Full() || o.App.Value.ID != c.AppID {
		s.Message = "Windowsの対象セッションを確認。対象ChatGPTはUNKNOWNです。"
		return s
	}
	app := o.App.Value
	matched := false
	for _, v := range sessions {
		if v.ID == app.SessionID && app.SID == c.SID {
			matched = true
		}
	}
	if app.Running && matched {
		s.State = "app_running"
		s.Message = "対象ユーザーのChatGPT起動を確認。Remote実接続は未確認です。"
		s.Next = "Android ChatGPT → Remoteで登録したWindowsと目的の作業を確認してください。"
	} else if !app.Running && app.SID == c.SID {
		s.State = "app_absent"
		s.Message = "Windowsにはログイン済みです。対象ChatGPTは未起動です。"
		s.Next = "RDPで登録済みChatGPTを起動してください。"
	}
	return s
}

type Recovery struct {
	Schema          int       `json:"schemaVersion"`
	Target          string    `json:"target"`
	Purpose         string    `json:"purpose"`
	LastObservation string    `json:"lastObservation"`
	ObservedAt      time.Time `json:"observedAt"`
	Next            string    `json:"nextSuggestedAction"`
	Result          string    `json:"userResult,omitempty"`
	ConfirmedAt     time.Time `json:"confirmedAt,omitempty"`
}

func (r Recovery) Stale(now time.Time) bool {
	return r.ObservedAt.IsZero() || now.Before(r.ObservedAt) || now.Sub(r.ObservedAt) > 2*time.Minute
}
func SaveRecovery(dir string, r Recovery) error { return save(filepath.Join(dir, "recovery.json"), r) }
func LoadRecovery(dir string) (Recovery, error) {
	var r Recovery
	b, e := os.ReadFile(filepath.Join(dir, "recovery.json"))
	if e != nil {
		return r, e
	}
	if len(b) > 16384 {
		return r, errors.New("復旧メモが大きすぎます")
	}
	e = json.Unmarshal(b, &r)
	if e == nil && r.Schema != 1 {
		e = errors.New("復旧メモの版が未対応です")
	}
	return r, e
}
