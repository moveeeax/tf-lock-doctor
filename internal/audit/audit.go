// Package audit records every lock release so that an unlock is never anonymous.
package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/moveeeax/tf-lock-doctor/internal/lock"
)

// Entry is one appended audit record describing a release.
type Entry struct {
	Time      time.Time `json:"time"`
	Actor     string    `json:"actor"`
	Backend   string    `json:"backend"`
	LockID    string    `json:"lock_id"`
	HeldBy    string    `json:"held_by"`
	Operation string    `json:"operation"`
	Reason    string    `json:"reason"`
	DryRun    bool      `json:"dry_run"`
}

// Writer appends audit entries as JSON lines to an underlying stream.
type Writer struct {
	w io.Writer
}

// NewWriter wraps w for JSON-line audit output.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// OpenFile opens (creating if needed) path for append-only audit logging.
func OpenFile(path string) (*Writer, *os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open audit log: %w", err)
	}
	return &Writer{w: f}, f, nil
}

// Record writes one audit line. The caller supplies the actor (who ran the
// release) and the lock that was held.
func (w *Writer) Record(actor string, backend string, info lock.Info, reason string, dryRun bool) error {
	e := Entry{
		Time:      time.Now().UTC(),
		Actor:     actor,
		Backend:   backend,
		LockID:    info.ID,
		HeldBy:    info.Who,
		Operation: info.Operation,
		Reason:    reason,
		DryRun:    dryRun,
	}
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal audit entry: %w", err)
	}
	if _, err := fmt.Fprintln(w.w, string(b)); err != nil {
		return fmt.Errorf("write audit entry: %w", err)
	}
	return nil
}

// DefaultActor derives an actor label from the environment: the CI run URL when
// present, otherwise user@host.
func DefaultActor() string {
	if run := ciRunURL(); run != "" {
		return run
	}
	user := firstEnv("USER", "USERNAME")
	host, _ := os.Hostname()
	if user == "" {
		user = "unknown"
	}
	if host == "" {
		host = "unknown-host"
	}
	return user + "@" + host
}

func ciRunURL() string {
	// GitHub Actions exposes enough to reconstruct the run URL.
	server := os.Getenv("GITHUB_SERVER_URL")
	repo := os.Getenv("GITHUB_REPOSITORY")
	runID := os.Getenv("GITHUB_RUN_ID")
	if server != "" && repo != "" && runID != "" {
		return fmt.Sprintf("%s/%s/actions/runs/%s", server, repo, runID)
	}
	// GitLab CI.
	if url := os.Getenv("CI_JOB_URL"); url != "" {
		return url
	}
	return ""
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
