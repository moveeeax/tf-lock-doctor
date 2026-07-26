# tf-lock-doctor

> Before you reach for force-unlock, check whether the holder is actually dead.

**Status:** 🚧 In development

## Overview

Diagnose a stuck Terraform state lock, showing who holds it and since when, and release it only after confirming the holding process is really gone.

## Features

- Reads the lock record from S3 + DynamoDB, GCS, azurerm and Consul backends
- Prints who took it, which operation, from which host and CI run, and how long ago
- Correlates the lock ID with the CI job that created it and checks whether that job is still running
- Refuses to unlock while the holding run is reported alive; `--force-after` covers genuinely abandoned locks
- Dry-run mode that shows the exact `terraform force-unlock` it would perform
- Appends an audit line for every release, so an unlock is never anonymous

## Stack

Go + `aws-sdk-go-v2` (DynamoDB, S3), `hashicorp/terraform-exec`, `hashicorp/hcl/v2` for backend config parsing, cobra.

## Usage

```bash
tf-lock-doctor inspect --chdir ./envs/prod
tf-lock-doctor release --chdir ./envs/prod --force-after 2h
```

## License

MIT
