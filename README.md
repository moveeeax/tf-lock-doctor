# tf-lock-doctor

> Before you reach for `force-unlock`, check whether the holder is actually dead.

Diagnose a stuck Terraform state lock — who holds it, since when, and for which
operation — and release it only after confirming the holding run is really gone.

## Why

A crashed `terraform apply` leaves the state lock behind. `terraform force-unlock`
demands the lock ID from a log you may no longer have, and it happily unlocks a
lock whose run is *still alive* — corrupting state for whoever is mid-apply.
`tf-lock-doctor` reads the lock straight from the backend, shows you who holds
it, and refuses to release it unless it can justify that the holder is gone.

## What it does

- Reads the lock record directly from the backend (no `terraform` invocation needed)
- Prints who took it, which operation, from which host/CI run, and how long ago
- Refuses to release while it cannot confirm the holder is gone; `--force-after`
  authorises releasing a lock older than a stated age (genuinely abandoned locks)
- Verifies the stored lock ID still matches before deleting, so a concurrent
  re-lock is never clobbered
- Dry-run mode prints the exact `terraform force-unlock` it would perform
- Appends a JSON audit line for every release, so an unlock is never anonymous

## Backend support

| Backend  | Status        |
|----------|---------------|
| `s3` (+ DynamoDB lock table) | ✅ implemented |
| `gcs`, `azurerm`, `consul`   | planned (behind the same `Backend` interface) |

The `s3` backend requires `dynamodb_table` — that is where Terraform stores the
lock. A backend with no lock table has no lock to doctor.

## Install

```bash
go install github.com/moveeeax/tf-lock-doctor/cmd/tf-lock-doctor@latest
```

or build from source:

```bash
make build   # produces ./bin/tf-lock-doctor
```

## Usage

The backend config is read from the Terraform module directory you point `--chdir` at.

```bash
# See who holds the lock, if anyone
tf-lock-doctor inspect --chdir ./envs/prod

# Machine-readable
tf-lock-doctor inspect --chdir ./envs/prod --json

# Preview the release of a lock abandoned for over 2 hours
tf-lock-doctor release --chdir ./envs/prod --force-after 2h --dry-run

# Actually release it (requires confirmation)
tf-lock-doctor release --chdir ./envs/prod --force-after 2h --yes
```

Without `--force-after`, `release` will refuse to act: it has no evidence the
holder is dead. That is the safe default.

### AWS credentials

`tf-lock-doctor` uses the standard AWS credential chain (environment variables,
shared config/credentials files, or an instance/role profile). It needs
`dynamodb:GetItem` and `dynamodb:DeleteItem` on the lock table.

### Audit log

Every release (including dry-runs) appends one JSON line to
`tf-lock-doctor-audit.log` (override with `--audit-log` or
`TF_LOCK_DOCTOR_AUDIT_LOG`):

```json
{"time":"2026-09-22T17:00:00Z","actor":"https://github.com/acme/infra/actions/runs/42","backend":"s3","lock_id":"abc-123","held_by":"alice@runner","operation":"OperationTypeApply","reason":"lock older than force-after threshold (3h2m)","dry_run":false}
```

## Development

```bash
make test   # go test ./...
make vet    # go vet ./...
make build
```

## License

MIT
