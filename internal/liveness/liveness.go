// Package liveness decides whether the process or CI run that holds a lock is
// still alive, so tf-lock-doctor can refuse to unlock a lock that a running job
// still depends on.
package liveness

import (
	"context"
	"time"

	"github.com/moveeeax/tf-lock-doctor/internal/lock"
)

// State describes what tf-lock-doctor could learn about the lock holder.
type State int

const (
	// Unknown means no signal was available to judge liveness.
	Unknown State = iota
	// Alive means the holding run is confirmed to still be running.
	Alive
	// Dead means the holding run is confirmed to have ended.
	Dead
)

func (s State) String() string {
	switch s {
	case Alive:
		return "alive"
	case Dead:
		return "dead"
	default:
		return "unknown"
	}
}

// Verdict bundles the liveness state with a human-readable reason.
type Verdict struct {
	State  State
	Reason string
}

// Checker reports whether the holder of a lock is still alive.
type Checker interface {
	Check(ctx context.Context, info lock.Info) Verdict
}

// AgeChecker is a backend-agnostic fallback: it treats a lock older than the
// given threshold as almost certainly abandoned, and anything younger as
// unknown (it cannot prove liveness, only staleness). It never reports Alive,
// so it can only ever relax the safety gate, never tighten it falsely.
type AgeChecker struct {
	// Threshold above which a lock is considered abandoned.
	Threshold time.Duration
	Now       func() time.Time
}

// NewAgeChecker returns an AgeChecker with the given threshold and a real clock.
func NewAgeChecker(threshold time.Duration) AgeChecker {
	return AgeChecker{Threshold: threshold, Now: time.Now}
}

// Check reports Dead when the lock is older than Threshold, otherwise Unknown.
func (a AgeChecker) Check(_ context.Context, info lock.Info) Verdict {
	now := time.Now
	if a.Now != nil {
		now = a.Now
	}
	age := info.Age(now())
	if a.Threshold > 0 && age >= a.Threshold {
		return Verdict{State: Dead, Reason: "lock older than force-after threshold (" + age.Round(time.Second).String() + ")"}
	}
	return Verdict{State: Unknown, Reason: "cannot confirm holder is gone; lock age " + age.Round(time.Second).String()}
}
