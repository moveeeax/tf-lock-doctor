// Package lock models a Terraform state lock record and the operations that
// read or release it, independent of which backend actually stores it.
package lock

import (
	"time"
)

// Info mirrors the JSON document Terraform writes when it takes a state lock.
// The field names match Terraform's own statemgr.LockInfo so records written by
// Terraform decode cleanly.
type Info struct {
	ID        string    `json:"ID"`
	Operation string    `json:"Operation"`
	Info      string    `json:"Info"`
	Who       string    `json:"Who"`
	Version   string    `json:"Version"`
	Created   time.Time `json:"Created"`
	Path      string    `json:"Path"`
}

// Age reports how long the lock has been held, measured from now.
func (i Info) Age(now time.Time) time.Duration {
	if i.Created.IsZero() {
		return 0
	}
	return now.Sub(i.Created)
}

// User returns the login portion of the "Who" field, which Terraform formats as
// "user@host". It falls back to the whole string when there is no "@".
func (i Info) User() string {
	for idx := 0; idx < len(i.Who); idx++ {
		if i.Who[idx] == '@' {
			return i.Who[:idx]
		}
	}
	return i.Who
}

// Host returns the host portion of the "Who" field, or an empty string when the
// field carries no host.
func (i Info) Host() string {
	for idx := 0; idx < len(i.Who); idx++ {
		if i.Who[idx] == '@' {
			return i.Who[idx+1:]
		}
	}
	return ""
}
