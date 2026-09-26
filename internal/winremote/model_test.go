package winremote

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/tsutomu-n/codex-phone-ops/internal/control"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func full[T any](v T) Check[T] { return Check[T]{Status: "observed", Completeness: "full", Value: &v} }
func unknown[T any]() Check[T] { return Check[T]{Status: "unavailable", Completeness: "partial"} }
func fixture() (Config, Observation) {
	c := Config{Host: "TEST", User: "TEST\\dev", SID: "S-1-5-test", AppID: "OpenAI.Test!App"}
	o := Observation{Schema: 1, RequestID: "test", CapturedAt: time.Now().UTC(), Identity: full(Identity{Host: c.Host, User: c.User, SID: c.SID}), Machine: unknown[Machine](), Sessions: full([]Session{{ID: 3, State: "Disconnected", SID: c.SID}}), Candidates: full([]Candidate{{Name: "ChatGPT", ID: c.AppID}, {Name: "ChatGPT Classic", ID: "Classic!App"}}), App: full(App{ID: c.AppID, Running: true, SessionID: 3, SID: c.SID}), RDP: full(RDP{Service: "Running", Port: 3389, Listening: true}), Tailscale: unknown[string](), Certificate: unknown[string]()}
	return c, o
}
func TestDecisions(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		change     func(*Config, *Observation)
	}{
		{"running-disconnected", "app_running", func(c *Config, o *Observation) {}},
		{"gui-absent", "gui_absent", func(c *Config, o *Observation) { o.Sessions = full([]Session{}) }},
		{"session-permission", "unknown", func(c *Config, o *Observation) {
			o.Sessions = Check[[]Session]{Status: "denied", Completeness: "partial"}
		}},
		{"partial-empty", "unknown", func(c *Config, o *Observation) { o.Sessions = full([]Session{}); o.Sessions.Completeness = "partial" }},
		{"app-absent", "app_absent", func(c *Config, o *Observation) { o.App.Value.Running = false }},
		{"multiple-unselected", "unknown", func(c *Config, o *Observation) { c.AppID = "" }},
		{"classic-not-selected", "unknown", func(c *Config, o *Observation) { o.App.Value.ID = "Classic!App" }},
		{"other-user", "unknown", func(c *Config, o *Observation) { o.App.Value.SID = "other" }},
		{"other-session", "unknown", func(c *Config, o *Observation) { o.App.Value.SessionID = 8 }},
		{"missing-app-discovery", "unknown", func(c *Config, o *Observation) { o.Candidates = full([]Candidate{}); o.App = unknown[App]() }},
		{"identity-mismatch", "trust_unknown", func(c *Config, o *Observation) { o.Identity.Value.SID = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, o := fixture()
			tc.change(&c, &o)
			s := Decide(c, o)
			if s.State != tc.want {
				t.Fatal(s)
			}
			if !strings.Contains(s.Checks["RDP"], "listener=true") {
				t.Fatal("partial failure discarded RDP")
			}
		})
	}
}
func TestParse(t *testing.T) {
	_, o := fixture()
	b, _ := json.Marshal(o)
	if _, e := Parse(append([]byte("\xef\xbb\xbf"), append(b, []byte("\r\n")...)...), "test"); e != nil {
		t.Fatal(e)
	}
	for _, b := range [][]byte{[]byte(`{}`), []byte(`<Objs/>`), []byte(`{"schemaVersion":1`), append(b, []byte(`{}`)...), []byte(strings.Repeat("x", 256*1024+1))} {
		if _, e := Parse(b, "test"); e == nil {
			t.Fatal("accepted malformed")
		}
	}
	if _, e := Parse(b, "old-request"); e == nil {
		t.Fatal("accepted stale response")
	}
	o.Sessions = Check[[]Session]{}
	b, _ = json.Marshal(o)
	if _, e := Parse(b, "test"); e == nil {
		t.Fatal("accepted missing item")
	}
}
func TestContextAndPrivacy(t *testing.T) {
	now := time.Now()
	for _, age := range []time.Duration{3 * time.Minute, -time.Minute} {
		if !(Recovery{ObservedAt: now.Add(-age)}).Stale(now) {
			t.Fatal(age)
		}
	}
	d := t.TempDir()
	r := Recovery{Schema: 1, Target: "test", Purpose: "work", LastObservation: "unknown", ObservedAt: now, Next: "RDP"}
	if e := SaveRecovery(d, r); e != nil {
		t.Fatal(e)
	}
	got, e := LoadRecovery(d)
	if e != nil || got.Target != "test" {
		t.Fatal(got, e)
	}
	s, _ := os.Stat(filepath.Join(d, "recovery.json"))
	if s.Mode().Perm() != 0600 {
		t.Fatal(s.Mode())
	}
	c, o := fixture()
	b, _ := json.Marshal(Decide(c, o))
	for _, private := range []string{c.SID, c.User, c.AppID} {
		if strings.Contains(string(b), private) {
			t.Fatal("private value in summary")
		}
	}
}
func TestPayloadLiteralAndBounded(t *testing.T) {
	c, _ := fixture()
	c.User = "日本語';$(bad)\""
	b := request(c, "test", false)
	var r struct {
		Code string         `json:"code"`
		Data map[string]any `json:"data"`
	}
	if json.Unmarshal(b, &r) != nil || r.Data["user"] != c.User || r.Code != probe {
		t.Fatal("data interpolated")
	}
	cmd := ssh(c, context.Background(), false)
	a := strings.Join(cmd.Args, " ")
	if strings.Contains(a, c.User) || !strings.Contains(a, "powershell.exe -NoLogo -NoProfile") || !strings.Contains(a, "ForwardAgent=no") {
		t.Fatal(a)
	}
	var limited bounded
	limited.Write(make([]byte, 300000))
	if !limited.overflow || limited.Len() != 256*1024 {
		t.Fatal("limit")
	}
}
func TestPowerShellContractOnLinux(t *testing.T) {
	path, e := exec.LookPath("pwsh")
	if e != nil {
		t.Skip("pwsh unavailable")
	}
	cmd := exec.Command(path, "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", encoded(bootstrap))
	c, _ := fixture()
	cmd.Stdin = strings.NewReader(string(request(c, "literal", false)))
	b, e := cmd.Output()
	if e != nil {
		t.Fatal(e)
	}
	o, e := Parse(b, "literal")
	if e != nil {
		t.Fatal(string(b), e)
	}
	if o.Identity.Full() {
		t.Fatal("Linux is not a Windows identity")
	}
}
func TestProbeFakeSSHConnectionLoss(t *testing.T) {
	d := t.TempDir()
	c, o := fixture()
	c.Alias = "test"
	c.SSHConfig = filepath.Join(d, "ssh_config")
	os.WriteFile(c.SSHConfig, []byte("Host test\n"), 0600)
	c.SSHHash, _ = control.HashFile(c.SSHConfig)
	// Real child process, fake ssh only: response requestId copied from stdin.
	raw, _ := json.Marshal(o)
	fixturePath := filepath.Join(d, "fixture.json")
	os.WriteFile(fixturePath, raw, 0600)
	script := "#!/usr/bin/env python3\nimport json,sys\nr=json.load(sys.stdin)\no=json.load(open(" + strconvQuote(fixturePath) + "))\no['requestId']=r['data']['requestId']\nprint(json.dumps(o))\nsys.exit(255)\n"
	os.WriteFile(filepath.Join(d, "ssh"), []byte(script), 0700)
	t.Setenv("PATH", d+":"+os.Getenv("PATH"))
	got, e := Probe(context.Background(), c, false)
	var re *control.RunError
	if got.Schema != 1 || !errors.As(e, &re) || re.Kind != "remote_command" {
		t.Fatal(got, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, e = Probe(ctx, c, false)
	if got.Schema != 0 || !errors.As(e, &re) || re.Kind != "canceled" {
		t.Fatal(got, e)
	}
}
func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestDeniedFixture(t *testing.T) {
	b, e := os.ReadFile("testdata/denied.json")
	if e != nil {
		t.Fatal(e)
	}
	o, e := Parse(b, "fixture")
	if e != nil {
		t.Fatal(e)
	}
	c, _ := fixture()
	s := Decide(c, o)
	if s.State != "unknown" || !strings.Contains(s.Checks["sessions"], "denied") || !o.RDP.Full() {
		t.Fatal(s)
	}
}
