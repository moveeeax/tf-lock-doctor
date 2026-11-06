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

Requires Go 1.26 or newer.

```bash
go install github.com/moveeeax/tf-lock-doctor/cmd/tf-lock-doctor@latest
```

or build from source:

```bash
make build   # produces ./bin/tf-lock-doctor
```

## Usage

The backend config is read from the Terraform module directory you point `--chdir` at,
merged with any `--backend-config` items (see [Partial backend configuration](#partial-backend-configuration)).

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

### Workspaces

By default the lock of the `default` workspace is used. Pass `--workspace` (a
global flag, accepted by every command) to act on another workspace's lock:

```bash
tf-lock-doctor inspect --chdir ./envs/prod --workspace staging
tf-lock-doctor release --chdir ./envs/prod --workspace staging --force-after 2h --dry-run
```

For a non-default workspace the lock item is looked up under the backend's
`workspace_key_prefix`, i.e. `<bucket>/<workspace_key_prefix>/<workspace>/<key>`.
The prefix defaults to `env:`, as in Terraform, and can come from the backend
block, a `--backend-config` item or the recorded init state.

### Partial backend configuration

Terraform recommends keeping `bucket`, `key` and `region` out of the backend
block and passing them to `terraform init -backend-config`. `tf-lock-doctor`
takes the same flag. `--backend-config` is global and repeatable; each item is
either `key=value` or the path of a file of top-level attributes (a
`*.tfbackend` file). Items are applied in order on top of the backend block, so
a later value wins, the same merge order as `terraform init`. A relative file
path is resolved against `--chdir` first, like `terraform -chdir`, then against
the current directory.

```bash
# backend "s3" {} in the module, the rest in a file plus one override
tf-lock-doctor inspect --chdir ./envs/prod \
  --backend-config prod.s3.tfbackend \
  --backend-config key=prod/network.tfstate
```

If the module is already initialised, `--from-init` reads the backend config
`terraform init` recorded in `.terraform/terraform.tfstate` (or under
`$TF_DATA_DIR`), which already has every `-backend-config` item merged in. The
backend block is not read then; `--backend-config` items still apply on top.

```bash
terraform -chdir=envs/prod init -backend-config=prod.s3.tfbackend
tf-lock-doctor inspect --chdir ./envs/prod --from-init
```

Only string values are used, from the block, the files and the init state
alike; a value built from a function or reference in the block is skipped.

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
