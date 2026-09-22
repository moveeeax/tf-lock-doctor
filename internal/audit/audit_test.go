package audit

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/moveeeax/tf-lock-doctor/internal/lock"
)

func TestRecord(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	info := lock.Info{ID: "abc-123", Who: "alice@runner", Operation: "OperationTypeApply", Created: time.Now()}
	if err := w.Record("bob@laptop", "s3", info, "lock older than threshold", false); err != nil {
		t.Fatalf("Record: %v", err)
	}
	line := strings.TrimSpace(buf.String())
	var e Entry
	if err := json.Unmarshal([]byte(line), &e); err != nil {
		t.Fatalf("audit line is not valid JSON: %v", err)
	}
	if e.Actor != "bob@laptop" || e.LockID != "abc-123" || e.HeldBy != "alice@runner" {
		t.Fatalf("unexpected entry: %+v", e)
	}
	if e.DryRun {
		t.Fatal("DryRun should be false")
	}
}

func TestDefaultActorFallback(t *testing.T) {
	t.Setenv("GITHUB_RUN_ID", "")
	t.Setenv("CI_JOB_URL", "")
	t.Setenv("USER", "carol")
	got := DefaultActor()
	if !strings.HasPrefix(got, "carol@") {
		t.Fatalf("DefaultActor = %q, want carol@<host>", got)
	}
}

func TestDefaultActorGitHub(t *testing.T) {
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_REPOSITORY", "moveeeax/tf-lock-doctor")
	t.Setenv("GITHUB_RUN_ID", "42")
	got := DefaultActor()
	want := "https://github.com/moveeeax/tf-lock-doctor/actions/runs/42"
	if got != want {
		t.Fatalf("DefaultActor = %q, want %q", got, want)
	}
}
