---
name: mariadb-backup-s3
description: >-
  Perform MariaDB backup, restore, retention management, and registry operations
  using the mariadb-backup-s3 CLI tool. Use when the task involves: (1) Creating
  a full MariaDB backup and uploading to S3/FTP/local storage, (2) Restoring a
  backup from storage, (3) Applying retention policies to prune old backups,
  (4) Listing or inspecting the backup registry, (5) Setting up or troubleshooting
  automated backup pipelines for MariaDB. Targets DevOps/operations workflows
  running the binary directly on a server.
license: Apache-2.0
---

# mariadb-backup-s3

CLI tool that runs `mariadb-backup`, compresses (pigz/gzip), optionally encrypts
(AES-256-GCM), and uploads to S3-compatible, FTP, or local filesystem storage.

## Subcommands

| Command | Alias | Description |
|---|--|---|
| `backup` | `b` | Full backup + upload to storage (default command) |
| `restore` | `r` | Download + decrypt + extract to target directory |
| `retention` | `ret` | Prune old backups per policy |
| `registry list` | `reg ls` | List backups in the registry |

Exit codes: `0` Success, `1` ParamsError, `2` ClientError, `3` OutputError, `4` InternalError

## Workflows

### Backup with retention

```shell
mariadb-backup-s3 backup \
  --storage-url="s3://bucket/path?endpoint=https://s3.eu-west-1.amazonaws.com" \
  --db-host=db.example.com \
  --db-user=backup \
  --retention-count=14 \
  --keep-daily=7
```

Minimal (env-only):

```shell
export MARIADB__HOST=localhost
export MARIADB__USER=root
export MARIADB__PASSWORD=secret
export STORAGE__URL=s3://my-bucket/backups
export AWS_REGION=eu-west-1
export AWS_ACCESS_KEY_ID=xxx
export AWS_SECRET_ACCESS_KEY=yyy
mariadb-backup-s3
```

### Restore latest backup

```shell
mariadb-backup-s3 restore \
  --storage-url="s3://bucket/path" \
  --target-dir=/var/lib/mysql \
  --latest
```

Restore by filename or registry ID:

```shell
mariadb-backup-s3 restore --storage-url="s3://bucket/path" \
  --target-dir=/tmp/restore \
  mariabackup-2026-07-03T12-00-00.tar.gz

mariadb-backup-s3 restore --storage-url="s3://bucket/path" \
  --target-dir=/tmp/restore \
  --backup-id="2026-07-03-12-00-00-a1b2c3d4"
```

### List backups

```shell
mariadb-backup-s3 registry list --storage-url="s3://bucket/path"
```

### Enforce retention manually

```shell
mariadb-backup-s3 retention \
  --storage-url="s3://bucket/path" \
  --retention-count=30 \
  --keep-daily=7 \
  --keep-weekly=4 \
  --keep-monthly=3
```

Preview with `--dry-run` before deleting.

## Requirements

- **External binaries**: `mariadb-backup`, `pigz` (or `gzip`), `tar` — must be in PATH
- **Temp space**: ~2× database size in `$TMPDIR` (default `/tmp`)
- **Storage**: S3 (AWS SDK v2 — reads `AWS_*` env vars), FTP, or local `file://` URL

## Encryption (optional)

Generate a key and pass it to any command:

```shell
# Use bundled script:
./scripts/gen-encryption-key.sh
```

```shell
mariadb-backup-s3 backup \
  --storage-url="s3://bucket/path" \
  --encryption-key="$(openssl rand -base64 32)"
```

Encrypted files get a `.enc` extension. Restore detects and decrypts automatically.

## Scripts

- `scripts/gen-encryption-key.sh` — generate a base64-encoded AES-256 key
- `scripts/validate-env.sh` — validate a `.env` file has required fields for backup/restore

## Reference

See `references/configuration.md` for the full flag, env-var, and storage-URL reference.
