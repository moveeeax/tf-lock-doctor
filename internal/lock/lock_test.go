package lock

import (
	"testing"
	"time"
)

func TestUserHostSplit(t *testing.T) {
	i := Info{Who: "alice@ci-runner-7"}
	if got := i.User(); got != "alice" {
		t.Errorf("User() = %q, want alice", got)
	}
	if got := i.Host(); got != "ci-runner-7" {
		t.Errorf("Host() = %q, want ci-runner-7", got)
	}
}

func TestUserHostNoAt(t *testing.T) {
	i := Info{Who: "someprocess"}
	if got := i.User(); got != "someprocess" {
		t.Errorf("User() = %q, want someprocess", got)
	}
	if got := i.Host(); got != "" {
		t.Errorf("Host() = %q, want empty", got)
	}
}

func TestAge(t *testing.T) {
	created := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	now := created.Add(90 * time.Minute)
	i := Info{Created: created}
	if got := i.Age(now); got != 90*time.Minute {
		t.Errorf("Age() = %v, want 90m", got)
	}
}

func TestAgeZeroCreated(t *testing.T) {
	i := Info{}
	if got := i.Age(time.Now()); got != 0 {
		t.Errorf("Age() with zero Created = %v, want 0", got)
	}
}
