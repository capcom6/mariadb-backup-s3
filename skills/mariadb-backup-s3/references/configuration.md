# Configuration Reference

Config loaded from: CLI flags > env vars > `.env` file (in working directory).

## Database

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--db-host`, `--host` | `MARIADB__HOST` | `localhost` | MariaDB hostname |
| `--db-port`, `--port` | `MARIADB__PORT` | `3306` | MariaDB port |
| `--db-user`, `--user` | `MARIADB__USER` | `root` | MariaDB username |
| `--db-password`, `--password` | `MARIADB__PASSWORD` | `""` | MariaDB password |
| `--backup-method` | `MARIADB__BACKUP_METHOD` | `mariadb-backup` | Backup method: `mariadb-backup` (physical) or `mariadb-dump` (logical) |
| `--db-backup-binary` | `MARIADB__BACKUP_BINARY` | auto-derived | Path to backup binary (auto-derived from `--backup-method` if not set) |
| `--db-client-binary` | `MARIADB__CLIENT_BINARY` | `mariadb` | Path to mariadb client binary (used to list databases for logical backup) |
| `--db-backup-options` | `MARIADB__BACKUP_OPTIONS` | `""` | Extra backup options (sanitized) |

## Storage

| Flag | Env var | Required | Description |
|---|---|---|---|
| `--storage-url`, `--storage` | `STORAGE__URL` | Yes | Storage URL (see formats below) |

### Storage URL formats

**S3:**

```text
s3://bucket-name/path?endpoint=https://s3.amazonaws.com&s3-force-path-style=true&part-size=20971520
```

Query parameters: `endpoint` (optional), `s3-force-path-style` (optional, default false), `part-size` (optional, default 10MB, min 5MB).

S3 authentication uses the standard AWS SDK chain (env vars, ~/.aws/credentials, etc.):

| Env var | Description |
|---|---|
| `AWS_REGION` | AWS region |
| `AWS_ACCESS_KEY_ID` | Access key |
| `AWS_SECRET_ACCESS_KEY` | Secret key |

**FTP:**

```text
ftp://username:password@host:port/path
```

Username defaults to `anonymous`.

**Filesystem:**

```text
file:///absolute/path/to/backup/directory
```

## Encryption

| Flag | Env var | Description |
|---|---|---|
| `--encryption-key` | `ENCRYPTION__KEY` | Base64-encoded 32-byte AES-256 key (optional) |

Algorithm: AES-256-GCM with HKDF-SHA256 key derivation. Encrypted files get `.enc` extension.

Generate key: `openssl rand -base64 32`

## Retention

All optional. If none set, retention is skipped.

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--retention-count` | `RETENTION__COUNT`, `BACKUP__LIMITS__MAX_COUNT` | unlimited | Number of most recent backups to keep |
| `--max-age` | `RETENTION__MAX_AGE` | unlimited | Max age (e.g. `24h`, `168h`) |
| `--keep-daily` | `RETENTION__KEEP_DAILY` | unlimited | Keep N per day |
| `--keep-weekly` | `RETENTION__KEEP_WEEKLY` | unlimited | Keep N per ISO week |
| `--keep-monthly` | `RETENTION__KEEP_MONTHLY` | unlimited | Keep N per month |
| `--skip-retention` | `BACKUP__SKIP_RETENTION` | false | Skip retention after backup |
| `--dry-run` | `RETENTION__DRY_RUN` | false | Preview without deleting |
| `--force` | `RETENTION__FORCE` | false | Continue despite errors |

## Restore

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--target-dir` | `RESTORE__TARGET_DIR` | required | Directory to restore into |
| `--latest` | — | false | Restore most recent ready backup |
| `--backup-id` | — | `""` | Restore specific backup by registry ID |

Exactly one backup selector required: positional filename, `--latest`, or `--backup-id`.

## Logging (env vars only)

| Env var | Values | Default |
|---|---|---|
| `LOG_LEVEL` | `debug`, `info`, `warn`, `error`, `fatal` | `info` |
| `LOG_FORMAT` | `human`, `json` | `human` |
| `LOG_OUTPUT` | `stdout`, `stderr`, file path | `stdout` |
| `NO_COLOR` | any non-empty | disabled |

## Example `.env`

```env
MARIADB__HOST=db.example.com
MARIADB__PORT=3306
MARIADB__USER=backup
MARIADB__PASSWORD=secret
MARIADB__BACKUP_METHOD=mariadb-backup

STORAGE__URL=s3://my-bucket/backups?endpoint=https://s3.eu-west-1.amazonaws.com

AWS_REGION=eu-west-1
AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY

# Generate with: ./scripts/gen-encryption-key.sh
ENCRYPTION__KEY=your-base64-32-byte-key

RETENTION__COUNT=14
RETENTION__KEEP_DAILY=7
RETENTION__KEEP_WEEKLY=4

LOG_LEVEL=info
LOG_FORMAT=human
```
