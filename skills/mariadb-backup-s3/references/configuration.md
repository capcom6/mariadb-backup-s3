# Configuration Reference

Config loaded from: CLI flags > env vars > `.env` file (in working directory).

`DB__*` env vars are preferred; legacy `MARIADB__*` names (HOST, PORT, USER, PASSWORD, BACKUP_METHOD, BACKUP_BINARY, CLIENT_BINARY, BACKUP_OPTIONS) still work as fallback when the `DB__*` variable is unset.

Two Docker images: `ghcr.io/capcom6/mariadb-backup-s3` (MariaDB tools + pigz) and `ghcr.io/capcom6/mongodb-backup-s3` (mongodump + pigz). Native installs need MongoDB Database Tools separately for the `mongodb` engine; `pigz` is required for all engines. MongoDB backups are extracted to a directory; install `mongorestore` separately if you need to import the dump.

## Database

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--db-engine` | `DB__ENGINE` | `mariadb` | Engine: `mariadb` or `mongodb` |
| `--db-host`, `--host` | `DB__HOST` | `localhost` | Database hostname |
| `--db-port`, `--port` | `DB__PORT` | `3306` / `27017` (engine) | Database port |
| `--db-user`, `--user` | `DB__USER` | `root` (mariadb), empty (mongodb) | Database username |
| `--db-password`, `--password` | `DB__PASSWORD` | `""` | Database password |
| `--backup-method` | `DB__BACKUP_METHOD` | derived from `--db-engine` | Backup method: `mariadb-backup` (physical), `mariadb-dump` (logical), or `mongodump` |
| `--db-backup-binary` | `DB__BACKUP_BINARY` | auto-derived | Path to backup binary (derived from engine and method) |
| `--db-client-binary` | `DB__CLIENT_BINARY` | `mariadb` (mariadb) | Path to client binary (database discovery for `mariadb-dump`; unused for mongodb) |
| `--db-backup-options` | `DB__BACKUP_OPTIONS` | `""` | Extra backup options (sanitized) |
| `--mongo-uri` | `MONGODB__URI` | `""` | MongoDB connection URI (replaces host/port/user) |
| `--mongo-auth-source` | `MONGODB__AUTH_SOURCE` | `admin` (mongodb) | MongoDB authentication source database |
| `--mongo-tls` | `MONGODB__TLS` | `false` | Enable MongoDB TLS |
| `--mongo-tls-ca-file` | `MONGODB__TLS_CA_FILE` | `""` | MongoDB TLS CA file |
| `--mongo-tls-certificate-key-file` | `MONGODB__TLS_CERTIFICATE_KEY_FILE` | `""` | MongoDB TLS certificate key file |

Engine defaults (when unset): mariadb -> port `3306`, method `mariadb-backup`, client binary `mariadb`; mongodb -> port `27017`, method `mongodump`, backup binary `mongodump`, client binary empty, auth source `admin`.

A database name in the `MONGODB__URI` path is rejected (backups dump all databases). Select a different authentication database with `?authSource=<db>` — or `&authSource=<db>` when the URI already carries options such as `?replicaSet=rs0`; when the URI carries `authSource`, `MONGODB__AUTH_SOURCE` is not passed to `mongodump`.

Backup method must match the engine (`mariadb-backup`/`mariadb-dump` -> mariadb, `mongodump` -> mongodb) or validation fails.

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
| `--retention-count` | `RETENTION__COUNT` (deprecated alias: `BACKUP__LIMITS__MAX_COUNT`) | unlimited | Number of most recent backups to keep |
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
| `--target-dir` | `RESTORE__TARGET_DIR` | required | Directory to extract backup files into (all engines) |
| `--latest` | - | false | Extract most recent ready backup |
| `--backup-id` | - | `""` | Extract backup selected by registry backup ID |

Exactly one backup selector required: positional filename, `--latest`, or `--backup-id`.

Restore extracts files only and does not connect to either database engine. MongoDB dump imports, when needed, are performed separately using `mongorestore`.

## Scheduler YAML

Per-job database section `database:` supports both engines; legacy `mariadb:` still works. Setting both in one job is an error.

`database:` keys: `engine`, `host`, `port`, `user`, `password`, `backup_method`, `backup_binary`, `client_binary`, `backup_options`, `uri`, `auth_source`, `tls`, `tls_ca_file`, `tls_certificate_key_file`.

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

MongoDB variant (engine + connection):

```env
DB__ENGINE=mongodb
DB__HOST=db.example.com
DB__PORT=27017
DB__USER=backup
DB__PASSWORD=secret
# or: MONGODB__URI=mongodb://backup:secret@db.example.com:27017

STORAGE__URL=s3://my-bucket/mongodb-backups
```
