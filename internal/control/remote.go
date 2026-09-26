package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ShellQuote is needed even when ssh itself receives a safe argv array:
// ssh passes the remote command through the server's shell.
func ShellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func RemoteCommand(script string, args ...string) string {
	a := []string{"sh", "-c", script, "phoneops"}
	a = append(a, args...)
	for i := range a {
		a[i] = ShellQuote(a[i])
	}
	return strings.Join(a, " ")
}

type Remote struct {
	C     Config
	Local bool
}

func (r Remote) Command(ctx context.Context, tty bool, script string, args ...string) *exec.Cmd {
	var cmd *exec.Cmd
	if r.Local {
		a := []string{"-c", script, "phoneops"}
		a = append(a, args...)
		if ctx == nil {
			cmd = exec.Command("sh", a...)
		} else {
			cmd = exec.CommandContext(ctx, "sh", a...)
		}
	} else {
		a := []string{"-F", r.C.SSHConfig, "-o", "StrictHostKeyChecking=yes", "-o", "UpdateHostKeys=no", "-o", "ConnectTimeout=6", "-o", "ConnectionAttempts=1", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "-o", "ForwardAgent=no", "-o", "ControlMaster=no", "-o", "ControlPath=none"}
		if tty {
			a = append(a, "-tt")
		} else {
			a = append(a, "-T", "-o", "BatchMode=yes")
		}
		a = append(a, "--", r.C.Host)
		if script != "" {
			a = append(a, RemoteCommand(script, args...))
		}
		if ctx == nil {
			cmd = exec.Command("ssh", a...)
		} else {
			cmd = exec.CommandContext(ctx, "ssh", a...)
		}
	}
	cmd.WaitDelay = time.Second
	return cmd
}

type limitedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	left := b.limit - b.Len()
	if left < 0 {
		left = 0
	}
	if len(p) > left {
		b.overflow = true
		p = p[:left]
	}
	b.Buffer.Write(p)
	return n, nil
}
func (r Remote) Read(ctx context.Context, script string, args ...string) ([]byte, error) {
	cmd := r.Command(ctx, false, "printf 'CC_TRANSPORT_1\\000';\n"+script, args...)
	o := limitedBuffer{limit: 1024 * 1024}
	er := limitedBuffer{limit: 8192}
	cmd.Stdout = &o
	cmd.Stderr = &er
	e := cmd.Run()
	marker := []byte("CC_TRANSPORT_1\x00")
	connected := bytes.HasPrefix(o.Bytes(), marker)
	if ctx.Err() != nil {
		return nil, ClassifyRun(ctx.Err(), e, er.String(), r.Local, connected)
	}
	if e != nil {
		return nil, ClassifyRun(nil, e, er.String(), r.Local, connected)
	}
	if o.overflow {
		return nil, errors.New("応答が上限を超えました。完全な結果として扱いません")
	}
	if !connected {
		return nil, errors.New("遠隔実行の開始を確認できません")
	}
	return o.Bytes()[len(marker):], nil
}

// ClassifyRun keeps SSH failures separate from remote/local command failures.
// Exit 255 alone is ambiguous; only anchored OpenSSH diagnostics refine it.
type RunError struct {
	Kind    string
	Message string
}

func (e *RunError) Error() string { return e.Kind + ": " + e.Message }
func ClassifyRun(ctxErr, err error, stderr string, local, response bool) error {
	kind, message := "remote_command", "コマンドを完了できません。直接SSHで確認できます"
	if local {
		kind = "local_command"
	}
	var ex *exec.ExitError
	if !local && !response && errors.As(err, &ex) && ex.ExitCode() == 255 {
		kind = "ssh_unknown"
		for _, line := range strings.Split(strings.ToLower(stderr), "\n") {
			line = strings.TrimSpace(line)
			switch {
			case line == "host key verification failed.", strings.Contains(line, "remote host identification has changed"):
				kind, message = "ssh_trust", "SSHホスト鍵の照合が必要です。警告を迂回せず接続先を確認してください"
			case regexp.MustCompile(`^[^ :]+@[^ :]+: permission denied \([^\r\n]+\)\.$`).MatchString(line):
				if kind != "ssh_trust" {
					kind, message = "ssh_auth", "SSH認証に失敗しました。鍵と接続ユーザーを確認してください"
				}
			case strings.HasPrefix(line, "ssh: connect to host "), strings.HasPrefix(line, "ssh: could not resolve hostname "):
				if kind != "ssh_trust" {
					kind, message = "ssh_transport", "SSH到達を確認できません。回線・接続先を確認してください"
				}
			}
		}
	}
	if kind == "ssh_trust" {
		return &RunError{kind, message}
	}
	if errors.Is(ctxErr, context.Canceled) {
		return &RunError{"canceled", "待機を取り消しました。遠隔処理の取消成功は意味しません"}
	}
	if errors.Is(ctxErr, context.DeadlineExceeded) {
		return &RunError{"timeout", "応答を待つ時間を超えました。PC停止とは断定していません"}
	}
	if err == nil {
		return nil
	}
	return &RunError{kind, message}
}

const discoverScript = `set -eu
[ "$(uname -s)" = Linux ] || exit 70
uid=$(id -u); user=$(id -un)
home=$(cd "$HOME" && pwd -P)
ch=${CODEX_HOME:-"$HOME/.codex"}
ch=$(readlink -m -- "$ch")
c=${1:-}
if [ -z "$c" ]; then c=$(command -v codex || true); fi
if [ -z "$c" ]; then
 for p in "$HOME/.local/bin/codex" "$HOME/.codex/packages/standalone/current/bin/codex"; do
  if [ -x "$p" ]; then c=$p; break; fi
 done
fi
case "$c" in /*) ;; *) echo CC_CODEX_MISSING >&2; exit 71;; esac
[ -x "$c" ] || { echo CC_CODEX_MISSING >&2; exit 71; }
mid=$(cat /etc/machine-id)
[ -n "$mid" ] || exit 72
v=$("$c" --version)
printf 'CC_INFO_1\000%s\000%s\000%s\000%s\000%s\000%s\000%s\000' "$uid" "$user" "$home" "$mid" "$ch" "$c" "$v"
`

type Info struct {
	Identity       Identity
	Codex, Version string
}

func (r Remote) Discover(ctx context.Context, path string) (Info, error) {
	b, e := r.Read(ctx, discoverScript, path)
	if e != nil {
		return Info{}, e
	}
	return parseInfo(b)
}
func parseInfo(b []byte) (Info, error) {
	p := strings.Split(string(b), "\x00")
	if len(p) != 9 || p[0] != "CC_INFO_1" || p[8] != "" {
		return Info{}, errors.New("接続先情報の形式が合いません。誤った値は登録しません")
	}
	sum := sha256.Sum256([]byte(p[4]))
	i := Info{Identity: Identity{UID: p[1], User: p[2], Home: p[3], Machine: hex.EncodeToString(sum[:]), CodexHome: p[5]}, Codex: p[6], Version: p[7]}
	if !ValidAbs(i.Codex) || !ValidAbs(i.Identity.Home) || !ValidAbs(i.Identity.CodexHome) || i.Identity.UID == "" {
		return Info{}, errors.New("接続先の識別情報が不完全です")
	}
	if !VersionOK(i.Version) {
		return i, fmt.Errorf("Codex CLI 0.155以降が必要です（取得値: %s）", Safe(i.Version))
	}
	return i, nil
}

var verRE = regexp.MustCompile(`(?m)^(?:codex-cli |codex )?(\d+)\.(\d+)\.(\d+)(?:\s|$)`)

func VersionOK(s string) bool {
	m := verRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return false
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	return a > 0 || (a == 0 && b >= 155)
}
func (r Remote) Verify(ctx context.Context) (Info, error) {
	if !r.Local {
		if e := r.C.TrustConfig(); e != nil {
			return Info{}, e
		}
	}
	i, e := r.Discover(ctx, r.C.Codex)
	if e != nil {
		return i, e
	}
	if i.Identity != r.C.Identity {
		return i, errors.New("接続先のユーザー・HOME・Codex環境が変わりました。再登録が必要です")
	}
	return i, nil
}

// Every native action rechecks UID/HOME/CODEX_HOME in the same shell as exec.
const execScript = `set -eu
expected_uid=$1; expected_home=$2; expected_ch=$3; expected_machine=$4; c=$5; cwd=$6; shift 6
home=$(cd "$HOME" && pwd -P)
ch=$(readlink -m -- "${CODEX_HOME:-"$HOME/.codex"}")
mid=$(cat /etc/machine-id)
mhash=$(printf %s "$mid" | sha256sum); mhash=${mhash%% *}
[ "$mhash" = "$expected_machine" ] && [ "$(id -u)" = "$expected_uid" ] && [ "$home" = "$expected_home" ] && [ "$ch" = "$expected_ch" ] || { echo CC_IDENTITY_CHANGED >&2; exit 73; }
[ -x "$c" ] || { echo CC_CODEX_MISSING >&2; exit 71; }
if [ -n "$cwd" ]; then [ -d "$cwd" ] || { echo CC_CWD_MISSING >&2; exit 74; }; cd -- "$cwd"; fi
export CODEX_HOME="$ch"
exec "$c" "$@"
`

func (r Remote) args(cwd string, args ...string) []string {
	a := []string{r.C.Identity.UID, r.C.Identity.Home, r.C.Identity.CodexHome, r.C.Identity.Machine, r.C.Codex, cwd}
	return append(a, args...)
}
func (r Remote) Native(cwd string, args ...string) *exec.Cmd {
	return r.Command(nil, true, execScript, r.args(cwd, args...)...)
}
func (r Remote) CodexRead(ctx context.Context, args ...string) ([]byte, error) {
	return r.Read(ctx, execScript, r.args("", args...)...)
}

type Daemon struct {
	Status         string  `json:"status"`
	Socket         string  `json:"socketPath"`
	ManagedPath    string  `json:"managedCodexPath"`
	ManagedVersion *string `json:"managedCodexVersion"`
	AppVersion     *string `json:"appServerVersion"`
	CLIVersion     *string `json:"cliVersion"`
}

func ParseDaemon(b []byte) (Daemon, error) {
	var d Daemon
	if e := json.Unmarshal(b, &d); e != nil {
		return d, errors.New("共有サービスの応答形式が未対応です。停止とは判断していません")
	}
	if d.Status != "running" && d.Status != "notRunning" {
		return d, errors.New("共有サービスの状態が未対応です。勝手には起動しません")
	}
	if !ValidAbs(d.Socket) {
		return d, errors.New("共有接続先を特定できません")
	}
	return d, nil
}
func (r Remote) Daemon(ctx context.Context) (Daemon, error) {
	b, e := r.CodexRead(ctx, "app-server", "daemon", "version")
	if e != nil {
		return Daemon{}, e
	}
	return ParseDaemon(b)
}

const projectScript = `set -eu
root=$1
[ -d "$root" ] || { echo CC_CWD_MISSING >&2; exit 74; }
printf 'CC_DIRS_1\000'
find "$root" -mindepth 1 -maxdepth 1 -type d ! -name '.*' -print0
`

func (r Remote) Projects(ctx context.Context) ([]string, error) {
	b, e := r.Read(ctx, projectScript, r.C.ProjectsRoot)
	if e != nil {
		return nil, e
	}
	if !bytes.HasPrefix(b, []byte("CC_DIRS_1\x00")) {
		return nil, errors.New("作業フォルダー一覧の形式が合いません")
	}
	p := strings.Split(strings.TrimSuffix(string(b[len("CC_DIRS_1\x00"):]), "\x00"), "\x00")
	out := []string{}
	for _, s := range p {
		if s == "" {
			continue
		}
		if !ValidAbs(s) {
			continue
		}
		out = append(out, s)
		if len(out) > 500 {
			return nil, errors.New("フォルダーが500件を超えています。対象ルートを狭めてください")
		}
	}
	sort.Strings(out)
	return out, nil
}
func CheckContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 12*time.Second)
}
