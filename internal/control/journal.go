package control

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Journal contains no prompt, conversation, token, raw command or stderr.
// dispatching is flushed BEFORE any remote change. It is not exactly-once.
type Operation struct {
	ID             string    `json:"id"`
	Target         string    `json:"target"`
	Kind           string    `json:"kind"`
	StartedAt      time.Time `json:"startedAt"`
	Outcome        string    `json:"outcome"`
	ResponseStatus string    `json:"responseStatus,omitempty"`
}

func BeginOperation(dir, target string) (Operation, error) {
	var a [16]byte
	if _, e := rand.Read(a[:]); e != nil {
		return Operation{}, e
	}
	o := Operation{ID: hex.EncodeToString(a[:]), Target: target, Kind: "daemon-start", StartedAt: time.Now().UTC(), Outcome: "dispatching"}
	return o, saveOperation(dir, o)
}
func saveOperation(dir string, o Operation) error {
	b, e := json.MarshalIndent(o, "", "  ")
	if e != nil {
		return e
	}
	return AtomicWrite(filepath.Join(dir, "operations", o.ID+".json"), append(b, '\n'))
}
func FinishOperation(dir string, o Operation, outcome, status string) error {
	if outcome != "accepted" && outcome != "unknown" {
		return errors.New("invalid outcome")
	}
	o.Outcome = outcome
	o.ResponseStatus = status
	return saveOperation(dir, o)
}
func UnknownOperations(dir, target string) (int, error) {
	fs, e := filepath.Glob(filepath.Join(dir, "operations", "*.json"))
	if e != nil {
		return 0, e
	}
	n := 0
	for _, p := range fs {
		b, e := os.ReadFile(p)
		if e != nil {
			return 0, e
		}
		var o Operation
		if e = json.Unmarshal(b, &o); e != nil {
			return 0, errors.New("操作記録が壊れています。新たな遠隔起動は停止します")
		}
		if o.Target == target && (o.Outcome == "dispatching" || o.Outcome == "unknown") {
			n++
		}
	}
	return n, nil
}
