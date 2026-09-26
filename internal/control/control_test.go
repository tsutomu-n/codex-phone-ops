package control

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRemoteShellRoundTrip(t *testing.T) {
	cases := []string{"normal", "日本語 worktree", "a'b", "$(touch SHOULD_NOT_EXIST)", "`;echo injected;#", "* [ ] ?", "--help", "a\tb", "a\nb", ""}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", RemoteCommand(`printf '%s\000' "$@"`, cases...))
	cmd.Dir = t.TempDir()
	b, e := cmd.Output()
	if e != nil {
		t.Fatal(e)
	}
	got := strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00")
	if !reflect.DeepEqual(got, cases) {
		t.Fatalf("%q != %q", got, cases)
	}
	if _, e := os.Stat(filepath.Join(cmd.Dir, "SHOULD_NOT_EXIST")); !os.IsNotExist(e) {
		t.Fatal("shell injection")
	}
}
func TestVersions(t *testing.T) {
	for s, want := range map[string]bool{"codex-cli 0.155.1": true, "0.155.0": true, "codex-cli 0.156.0": true, "codex-cli 0.154.9": false, "0.155.0-alpha.1": false, "v0.155": false, "unknown": false} {
		if VersionOK(s) != want {
			t.Errorf("%s", s)
		}
	}
}
func TestDaemonNotConfusedWithMissing(t *testing.T) {
	for _, s := range []string{`{}`, `not json`, `{"status":"stopped","socketPath":"/tmp/s"}`, `{"status":"running","socketPath":"ws://other"}`} {
		if _, e := ParseDaemon([]byte(s)); e == nil {
			t.Errorf("accepted %s", s)
		}
	}
	d, e := ParseDaemon([]byte(`{"status":"notRunning","socketPath":"/tmp/s","future":true}`))
	if e != nil || d.Status != "notRunning" {
		t.Fatal(d, e)
	}
}
func TestDiscoveryAndIdentity(t *testing.T) {
	d := t.TempDir()
	bin := filepath.Join(d, "codex ' safe")
	if e := os.WriteFile(bin, []byte("#!/bin/sh\nprintf 'codex-cli 0.155.1\\n'\n"), 0700); e != nil {
		t.Fatal(e)
	}
	r := Remote{Local: true}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	i, e := r.Discover(ctx, bin)
	if e != nil {
		t.Fatal(e)
	}
	r.C = Config{Codex: bin, Identity: i.Identity}
	if _, e = r.Verify(ctx); e != nil {
		t.Fatal(e)
	}
	r.C.Identity.UID = "other"
	if _, e = r.Verify(ctx); e == nil {
		t.Fatal("accepted wrong identity")
	}
	_, e = r.Read(ctx, execScript, r.args("", "--version")...)
	if e == nil {
		t.Fatal("exec did not reject wrong identity")
	}
}
func TestProjectDiscoveryImmediateAndLiteral(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"zeta", "日本語 ' $(false)", ".hidden", "a"} {
		os.Mkdir(filepath.Join(root, n), 0700)
	}
	os.Mkdir(filepath.Join(root, "a", "nested"), 0700)
	ctx, cancel := CheckContext()
	defer cancel()
	p, e := (Remote{Local: true, C: Config{ProjectsRoot: root}}).Projects(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if len(p) != 3 {
		t.Fatal(p)
	}
	for _, v := range p {
		if strings.Contains(v, "nested") || strings.Contains(v, ".hidden") {
			t.Fatal(p)
		}
	}
}
func TestReadTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, e := (Remote{Local: true}).Read(ctx, "exec sleep 10")
	if e == nil {
		t.Fatal("no timeout")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("timeout not bounded")
	}
}
func TestConfigAndJournal(t *testing.T) {
	dir := t.TempDir()
	c := Config{Schema: 1, Label: "Ubuntu", Host: "ubuntu", SSHConfig: "/tmp/ssh_config", SSHConfigHash: "hash", ProjectsRoot: "/home/user/projects", Codex: "/bin/codex", Identity: Identity{UID: "1000", User: "user", Home: "/home/user", CodexHome: "/home/user/.codex", Machine: "hash"}}
	if e := SaveConfig(dir, c); e != nil {
		t.Fatal(e)
	}
	got, e := Load(dir)
	if e != nil || !reflect.DeepEqual(got, c) {
		t.Fatal(got, e)
	}
	o, e := BeginOperation(dir, "ubuntu")
	if e != nil {
		t.Fatal(e)
	}
	n, e := UnknownOperations(dir, "ubuntu")
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if e = FinishOperation(dir, o, "accepted", "started"); e != nil {
		t.Fatal(e)
	}
	n, e = UnknownOperations(dir, "ubuntu")
	if n != 0 || e != nil {
		t.Fatal(n, e)
	}
	// Truncation/corruption must fail closed instead of clearing an unknown request.
	if e = os.WriteFile(filepath.Join(dir, "operations", o.ID+".json"), []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = UnknownOperations(dir, "ubuntu"); e == nil {
		t.Fatal("ignored corrupt journal")
	}
}
func TestUnknownOutcomeSurvivesReload(t *testing.T) {
	dir := t.TempDir()
	o, e := BeginOperation(dir, "ubuntu")
	if e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "operations", o.ID+".json"))
	var disk Operation
	json.Unmarshal(b, &disk)
	if disk.Outcome != "dispatching" {
		t.Fatal(disk)
	}
	n, e := UnknownOperations(dir, "ubuntu")
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
}
func TestExclusiveForeground(t *testing.T) {
	d := t.TempDir()
	u, e := Lock(d)
	if e != nil {
		t.Fatal(e)
	}
	defer u()
	if u2, e := Lock(d); e == nil {
		u2()
		t.Fatal("second instance accepted")
	}
}
func TestTerminalUntrustedTextAndNarrowWidth(t *testing.T) {
	cases := []string{"普通の日本語プロジェクト名と長いフォルダー名でスクロールを確認する", "x\x1b]52;c;secret\ax\x1b[2J", "abc\n\r\tdef", "A\u202eb"}
	for _, s := range cases {
		clean := Safe(s)
		if strings.ContainsAny(clean, "\x1b\r\n\t") || strings.ContainsRune(clean, '\u202e') {
			t.Fatal(clean)
		}
		for _, w := range []int{20, 40, 60} {
			if Width(Clip(s, w)) > w {
				t.Fatal("overflow")
			}
			for _, l := range Wrap(s, w) {
				if Width(l) > w {
					t.Fatal("wrap overflow")
				}
			}
		}
	}
}
func TestNoAccidentalResumeOrRemoteMutation(t *testing.T) {
	c := Config{Identity: Identity{UID: "1000", Home: "/home/u", CodexHome: "/home/u/.codex"}, Codex: "/home/u/.local/bin/codex", SSHConfig: "/tmp/config", Host: "ubuntu"}
	r := Remote{C: c}
	cmd := r.Native("", "agents", "--remote", "unix:///tmp/sock", "--no-alt-screen")
	s := strings.Join(cmd.Args, " ")
	for _, bad := range []string{"StrictHostKeyChecking=no", "--last", "daemon restart", "daemon update", "remote-control start"} {
		if strings.Contains(s, bad) {
			t.Fatal(s)
		}
	}
	if !strings.Contains(s, "agents") || !strings.Contains(s, "-tt") || !strings.Contains(s, "StrictHostKeyChecking=yes") {
		t.Fatal(s)
	}
}

func TestFailureClassification(t *testing.T) {
	exit := func(code string) error { return exec.Command("sh", "-c", "exit "+code).Run() }
	for _, tc := range []struct {
		stderr, code, want string
		local, response    bool
	}{
		{"Permission denied", "1", "remote_command", false, false},
		{"Permission denied", "1", "local_command", true, false},
		{"user@host: Permission denied (publickey).", "255", "ssh_auth", false, false},
		{"user@host: Permission denied (publickey).", "255", "remote_command", false, true},
		{"powershell: Permission denied", "255", "ssh_unknown", false, false},
		{"Host key verification failed.", "255", "ssh_trust", false, false},
		{"ssh: connect to host test port 22: Connection refused", "255", "ssh_transport", false, false},
	} {
		e := ClassifyRun(nil, exit(tc.code), tc.stderr, tc.local, tc.response)
		if e.(*RunError).Kind != tc.want {
			t.Fatal(tc, e)
		}
	}
	for ctx, want := range map[error]string{context.Canceled: "canceled", context.DeadlineExceeded: "timeout"} {
		if ClassifyRun(ctx, exit("1"), "", false, false).(*RunError).Kind != want {
			t.Fatal(want)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := (Remote{Local: true}).Read(ctx, "true")
	if e.(*RunError).Kind != "canceled" {
		t.Fatal(e)
	}
}
func TestLostOperationRemainsUnknownAndLocked(t *testing.T) {
	d := t.TempDir()
	unlock, e := Lock(d)
	if e != nil {
		t.Fatal(e)
	}
	defer unlock()
	o, e := BeginOperation(d, "test")
	if e != nil {
		t.Fatal(e)
	}
	if e = FinishOperation(d, o, "unknown", ""); e != nil {
		t.Fatal(e)
	}
	if n, e := UnknownOperations(d, "test"); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if unlock2, e := Lock(d); e == nil {
		unlock2()
		t.Fatal("duplicate writer permitted")
	}
}

func TestRestoreFailureStopsApp(t *testing.T) {
	old := os.Stdin
	f, e := os.Open(os.DevNull)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	os.Stdin = f
	defer func() { os.Stdin = old }()
	a := App{T: &Terminal{}}
	a.restore()
	if a.Fatal == nil {
		t.Fatal("restore error ignored")
	}
}
func TestRemotePermissionAfterDispatch(t *testing.T) {
	d := t.TempDir()
	fake := filepath.Join(d, "ssh")
	script := "#!/bin/sh\nprintf 'CC_TRANSPORT_1\\000'\necho 'user@host: Permission denied (publickey).' >&2\nexit 255\n"
	if e := os.WriteFile(fake, []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	_, e := (Remote{}).Read(context.Background(), "true")
	if e.(*RunError).Kind != "remote_command" {
		t.Fatal(e)
	}
}
