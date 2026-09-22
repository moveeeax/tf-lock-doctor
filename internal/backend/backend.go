// Package backend abstracts the storage that holds a Terraform state lock so the
// rest of tf-lock-doctor works against one interface regardless of provider.
package backend

import (
	"context"
	"errors"

	"github.com/moveeeax/tf-lock-doctor/internal/lock"
)

// ErrNoLock is returned by Read when the backend holds no lock. A missing lock
// is a normal, expected state, not a failure.
var ErrNoLock = errors.New("no state lock present")

// Backend reads and releases the lock record for a single Terraform state.
type Backend interface {
	// Name identifies the backend implementation, e.g. "s3".
	Name() string

	// Read returns the current lock, or ErrNoLock if the state is unlocked.
	Read(ctx context.Context) (lock.Info, error)

	// Release removes the lock identified by id. Implementations must refuse to
	// remove a lock whose stored ID does not match id, so a caller can never
	// unlock a lock it did not observe.
	Release(ctx context.Context, id string) error
}
