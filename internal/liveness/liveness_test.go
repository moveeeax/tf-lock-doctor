package liveness

import (
	"context"
	"testing"
	"time"

	"github.com/moveeeax/tf-lock-doctor/internal/lock"
)

func fixedNow(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestAgeCheckerDead(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := created.Add(3 * time.Hour)
	c := AgeChecker{Threshold: 2 * time.Hour, Now: fixedNow(now)}
	v := c.Check(context.Background(), lock.Info{Created: created})
	if v.State != Dead {
		t.Fatalf("state = %v, want Dead", v.State)
	}
}

func TestAgeCheckerUnknownWhenYoung(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := created.Add(30 * time.Minute)
	c := AgeChecker{Threshold: 2 * time.Hour, Now: fixedNow(now)}
	v := c.Check(context.Background(), lock.Info{Created: created})
	if v.State != Unknown {
		t.Fatalf("state = %v, want Unknown", v.State)
	}
}

func TestAgeCheckerNeverAlive(t *testing.T) {
	// With no threshold set, the checker must stay Unknown, never Alive.
	c := AgeChecker{Threshold: 0, Now: fixedNow(time.Now())}
	v := c.Check(context.Background(), lock.Info{Created: time.Now().Add(-100 * time.Hour)})
	if v.State == Alive {
		t.Fatalf("AgeChecker must never report Alive, got %v", v.State)
	}
	if v.State != Unknown {
		t.Fatalf("state = %v, want Unknown when threshold is 0", v.State)
	}
}
