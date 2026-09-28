package winremote

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/tsutomu-n/codex-phone-ops/internal/control"
	"os/exec"
	"strings"
	"time"
	"unicode/utf16"
)

//go:embed probe.ps1
var probe string

const bootstrap = `$ErrorActionPreference='Stop';[Console]::InputEncoding=New-Object System.Text.UTF8Encoding($false);$r=[Console]::In.ReadToEnd().TrimStart([char]0xFEFF)|ConvertFrom-Json;& ([ScriptBlock]::Create($r.code)) $r.data`

func encoded(s string) string {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2*len(u))
	for i, v := range u {
		b[2*i] = byte(v)
		b[2*i+1] = byte(v >> 8)
	}
	return base64.StdEncoding.EncodeToString(b)
}
func request(c Config, id string, cert bool) []byte {
	b, _ := json.Marshal(map[string]any{"code": probe, "data": map[string]any{"requestId": id, "user": c.User, "sid": c.SID, "appId": c.AppID, "certificate": cert}})
	return b
}
func ssh(c Config, ctx context.Context, interactive bool) *exec.Cmd {
	r := control.Remote{C: control.Config{Host: c.Alias, SSHConfig: c.SSHConfig}}
	cmd := r.Command(ctx, interactive, "")
	if !interactive {
		cmd.Args = append(cmd.Args, "powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand "+encoded(bootstrap))
	}
	return cmd
}

type bounded struct {
	bytes.Buffer
	overflow bool
}

func (b *bounded) Write(p []byte) (int, error) {
	n := len(p)
	left := 256*1024 - b.Len()
	if len(p) > left {
		b.overflow = true
		p = p[:left]
	}
	b.Buffer.Write(p)
	return n, nil
}
func started(stderr, id string) bool {
	needle := "CPO_PROBE_STARTED_V1:" + id
	count := 0
	if !strings.HasSuffix(stderr, "\n") {
		return false
	}
	for _, line := range strings.Split(stderr, "\n") {
		if strings.TrimSuffix(line, "\r") == needle {
			count++
		}
	}
	return count == 1
}
func Probe(ctx context.Context, c Config, cert bool) (Observation, error) {
	if e := c.Trust(); e != nil {
		return Observation{}, e
	}
	var nonce [16]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		return Observation{}, e
	}
	id := hex.EncodeToString(nonce[:])
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	cmd := ssh(c, ctx, false)
	cmd.Stdin = bytes.NewReader(request(c, id, cert))
	var out, er bounded
	cmd.Stdout = &out
	cmd.Stderr = &er
	e := cmd.Run()
	// An explicit host-key failure wins even if a response was printed.
	if trust := control.ClassifyRun(ctx.Err(), e, er.String(), false, false); trust != nil {
		if re, ok := trust.(*control.RunError); ok && re.Kind == "ssh_trust" {
			return Observation{}, trust
		}
	}
	if out.overflow || er.overflow {
		return Observation{}, &control.RunError{Kind: "diagnostic_parse", Message: "診断出力が256KiBを超えました"}
	}
	o, parseErr := Parse(out.Bytes(), id)
	runErr := control.ClassifyRun(ctx.Err(), e, er.String(), false, parseErr == nil || started(er.String(), id))
	if parseErr == nil {
		return o, runErr
	}
	if runErr != nil {
		return Observation{}, runErr
	}
	return Observation{}, parseErr
}
