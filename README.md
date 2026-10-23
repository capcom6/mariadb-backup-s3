<a id="readme-top"></a>

<!-- PROJECT SHIELDS -->
[![Contributors][contributors-shield]][contributors-url]
[![Forks][forks-shield]][forks-url]
[![Stargazers][stars-shield]][stars-url]
[![Issues][issues-shield]][issues-url]
[![License][license-shield]][license-url]
![Go Version][go-version-shield]

<!-- PROJECT HEADER -->
<br />
<div align="center">
  <h1>MariaDB Backup to S3</h1>
  <p>
    Automated MariaDB and MongoDB backups with S3-compatible storage support.<br />
    Physical (<code>mariadb-backup</code>), logical (<code>mariadb-dump</code>), or MongoDB (<code>mongodump</code>) backups, compressed, optionally encrypted, stored locally or in S3/FTP.
  </p>

  <a href="https://github.com/capcom6/mariadb-backup-s3/releases/latest">Download</a>
  &middot;
  <a href="https://github.com/capcom6/mariadb-backup-s3/issues/new?labels=bug&template=bug-report---.md">Report Bug</a>
  &middot;
  <a href="https://github.com/capcom6/mariadb-backup-s3/issues/new?labels=enhancement&template=feature-request---.md">Request Feature</a>
</div>

<!-- TABLE OF CONTENTS -->
- [About The Project](#about-the-project)
  - [Built With](#built-with)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
- [Usage](#usage)
  - [Backup](#backup)
  - [Restore](#restore)
  - [Retention](#retention)
  - [Registry](#registry)
  - [Scheduler](#scheduler)
- [Storage Types](#storage-types)
  - [S3 Storage](#s3-storage)
  - [FTP Storage](#ftp-storage)
  - [Filesystem Storage](#filesystem-storage)
- [Encryption](#encryption)
- [Examples](#examples)
- [Contributing](#contributing)
- [License](#license)
- [Contact](#contact)
- [Acknowledgments](#acknowledgments)


<!-- ABOUT THE PROJECT -->
## About The Project

MariaDB Backup to S3 is a CLI tool that backs up MariaDB and MongoDB databases, compresses them, optionally encrypts them with AES-256-GCM, and uploads to S3-compatible, FTP, or local filesystem storage.

**Three backup methods are supported:**

| Method                 | Binary           | Description                                                                                                             |
| ---------------------- | ---------------- | ----------------------------------------------------------------------------------------------------------------------- |
| **Physical** (default) | `mariadb-backup` | Hot backup at the storage engine level. Produces a tar.gz archive. Restore by copying files back to the data directory. |
| **Logical**            | `mariadb-dump`   | SQL dump of each database as a separate `.sql` file, tarred together. Restore by importing the `.sql` files you need.   |
| **MongoDB**            | `mongodump`      | Logical BSON dump of all databases via MongoDB Database Tools. Restore with `mongorestore`.                             |

**Key features:**

- Compress backups with pigz (parallel gzip)
- MongoDB engine support alongside MariaDB (`mongodump` / `mongorestore`)
- Optional AES-256-GCM client-side encryption
- Multiple storage backends: S3-compatible, FTP, filesystem
- Automatic backup rotation with configurable retention policies
- Backup registry for tracking and managing backups
- Built-in scheduler daemon for automated scheduled backups without external cron
- Docker container support

<p align="right">(<a href="#readme-top">back to top</a>)</p>

### Built With

- [Go](https://go.dev/)
- [mariadb-backup](https://mariadb.com/docs/server/server-usage/backup-and-restore/backup-and-restore-overview#mariadb-backup) / [mariadb-dump](https://mariadb.com/docs/server/server-usage/backup-and-restore/backup-and-restore-overview#mariadb-dump)
- MongoDB Database Tools (`mongodump` / `mongorestore`)
- [AWS SDK for Go v2](https://aws.github.io/aws-sdk-go-v2/)
- [pigz](https://zlib.net/pigz/) (parallel gzip)

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- GETTING STARTED -->
## Getting Started

### Prerequisites

- Go 1.25+ (for building from source)
- MariaDB server with `mariadb-backup` (for physical) or `mariadb-dump` (for logical) available in PATH, and `mariadb` client binary for listing databases (logical backup only)
- For the `mongodb` engine: MongoDB Database Tools (`mongodump` for backup, `mongorestore` for restore) available in PATH; native installs must provide them separately (the Docker images bundle the tools for their engine)
- `pigz` in PATH (required for all engines; bundled in both Docker images)
- `tar` in PATH
- At least 2x the database size in free temp space
- Storage backend credentials (depending on chosen storage type)

### Installation

**Binary (recommended):**

1. Visit the [Releases page](https://github.com/capcom6/mariadb-backup-s3/releases/latest)
2. Download the appropriate binary for your OS
3. Make executable and move to PATH:
   ```sh
   chmod +x mariadb-backup-s3
   sudo mv mariadb-backup-s3 /usr/local/bin/
   ```

**Go install:**

```sh
go install github.com/capcom6/mariadb-backup-s3@latest
```

**Docker:**

Two images are published:

```sh
# MariaDB tools (mariadb-backup, mariadb-dump, mariadb) + pigz
docker pull ghcr.io/capcom6/mariadb-backup-s3:latest

# MongoDB Database Tools (mongodump, mongorestore) + pigz
docker pull ghcr.io/capcom6/mongodb-backup-s3:latest
```

| Image                               | Base           | Contents                        |
| ----------------------------------- | -------------- | ------------------------------- |
| `ghcr.io/capcom6/mariadb-backup-s3` | `mariadb:lts`  | MariaDB tools + `pigz`          |
| `ghcr.io/capcom6/mongodb-backup-s3` | `ubuntu:24.04` | MongoDB Database Tools + `pigz` |

> **Note**
> The MariaDB image uses MariaDB's `lts` version; the MongoDB image bundles a pinned MongoDB Database Tools version. For specific versions, clone the repo, modify `Dockerfile.goreleaser` or `Dockerfile.mongodb.goreleaser`, and build a custom image.

**From source:**

```sh
git clone https://github.com/capcom6/mariadb-backup-s3.git
cd mariadb-backup-s3
go build -o mariadb-backup-s3
```

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- USAGE -->
## Usage

```sh
mariadb-backup-s3 [global options] command [command options] [arguments...]
```

Configuration is loaded from (highest priority first): CLI flags > environment variables > `.env` file in the current directory.

### Backup

```sh
mariadb-backup-s3 backup [options]
```

**Options:**

| Option                             | Env Var                             | Description                                                           | Default                              |
| ---------------------------------- | ----------------------------------- | --------------------------------------------------------------------- | ------------------------------------ |
| **Database**                       |                                     |                                                                       |                                      |
| `--db-engine`                      | `DB__ENGINE`                        | Database engine: `mariadb` or `mongodb`                               | `mariadb`                            |
| `--db-host`, `--host`              | `DB__HOST`                          | Database hostname                                                     | `localhost`                          |
| `--db-port`, `--port`              | `DB__PORT`                          | Database port (engine-dependent)                                      | `3306` / `27017`                     |
| `--db-user`, `--user`              | `DB__USER`                          | Database username                                                     | `root` (mariadb), empty (mongodb)    |
| `--db-password`, `--password`      | `DB__PASSWORD`                      | Database password                                                     | `""`                                 |
| `--backup-method`                  | `DB__BACKUP_METHOD`                 | `mariadb-backup` (physical), `mariadb-dump` (logical), or `mongodump` | derived from `--db-engine`           |
| `--db-backup-binary`               | `DB__BACKUP_BINARY`                 | Backup binary path (auto-derived from engine and method)              | auto-derived from engine and method  |
| `--db-client-binary`               | `DB__CLIENT_BINARY`                 | Client binary (lists databases for logical mariadb-dump backup)       | `mariadb` (mariadb), empty (mongodb) |
| `--db-backup-options`              | `DB__BACKUP_OPTIONS`                | Extra backup options (sanitized)                                      | `""`                                 |
| `--db-restore-binary`              | `DB__RESTORE_BINARY`                | Restore binary (used by mongodb; mariadb restore is extract-only)     | `mongorestore` (mongodb)             |
| `--db-restore-options`             | `DB__RESTORE_OPTIONS`               | Extra restore tool options (sanitized)                                | `""`                                 |
| `--mongo-uri`                      | `MONGODB__URI`                      | MongoDB connection URI (replaces host/port/user)                      | `""`                                 |
| `--mongo-auth-source`              | `MONGODB__AUTH_SOURCE`              | MongoDB authentication source database                                | `admin` (mongodb)                    |
| `--mongo-tls`                      | `MONGODB__TLS`                      | Enable MongoDB TLS                                                    | `false`                              |
| `--mongo-tls-ca-file`              | `MONGODB__TLS_CA_FILE`              | MongoDB TLS CA file                                                   | `""`                                 |
| `--mongo-tls-certificate-key-file` | `MONGODB__TLS_CERTIFICATE_KEY_FILE` | MongoDB TLS certificate key file                                      | `""`                                 |
| **Storage**                        |                                     |                                                                       |                                      |
| `--storage-url`, `--storage`       | `STORAGE__URL`                      | Storage URL (see [Storage Types](#storage-types))                     | **required**                         |
| **Encryption**                     |                                     |                                                                       |                                      |
| `--encryption-key`                 | `ENCRYPTION__KEY`                   | Base64-encoded AES-256 key                                            | `""`                                 |
| **Retention**                      |                                     |                                                                       |                                      |
| `--retention-count`                | `RETENTION__COUNT`                  | Number of backups to retain (0 = unlimited)                           | `0`                                  |
| `--max-age`                        | `RETENTION__MAX_AGE`                | Maximum age of backups (e.g. `24h`, `168h`)                           | unlimited                            |
| `--keep-daily`                     | `RETENTION__KEEP_DAILY`             | Number of daily backups to keep                                       | `0` (disabled)                       |
| `--keep-weekly`                    | `RETENTION__KEEP_WEEKLY`            | Number of weekly backups to keep                                      | `0` (disabled)                       |
| `--keep-monthly`                   | `RETENTION__KEEP_MONTHLY`           | Number of monthly backups to keep                                     | `0` (disabled)                       |
| `--skip-retention`                 | `BACKUP__SKIP_RETENTION`            | Skip retention policy after backup                                    | `false`                              |

> **Note**
> `DB__*` variables are preferred. Legacy `MARIADB__*` names (`MARIADB__HOST`, `MARIADB__PORT`, `MARIADB__USER`, `MARIADB__PASSWORD`, `MARIADB__BACKUP_METHOD`, `MARIADB__BACKUP_BINARY`, `MARIADB__CLIENT_BINARY`, `MARIADB__BACKUP_OPTIONS`) still work as fallback when the `DB__*` variable is unset. `DB__RESTORE_BINARY` and `DB__RESTORE_OPTIONS` have no legacy equivalent.

**Physical backup example:**

```sh
mariadb-backup-s3 backup \
  --db-host=mariadb.example.com \
  --db-user=backup \
  --storage-url="s3://my-bucket/backups?endpoint=https://s3.eu-west-1.amazonaws.com" \
  --retention-count=14 \
  --keep-daily=7
```

**Logical backup example:**

```sh
mariadb-backup-s3 backup \
  --backup-method=mariadb-dump \
  --db-host=mariadb.example.com \
  --db-user=backup \
  --storage-url="s3://my-bucket/backups"
```

By default `mariadb-dump` locks tables per database using `READ LOCAL` locks, so concurrent inserts into non-transactional tables (e.g. MyISAM/Aria) may still occur while a table is dumped, and cross-database consistency is not guaranteed. Each database is dumped in a separate invocation, so the generated `.sql` files are not point-in-time consistent with each other; as a result, logical (`mariadb-dump`) backups do not provide a global snapshot of all databases. The virtual schemas `information_schema` and `performance_schema` are skipped. For non-blocking dumps (InnoDB only), use `--db-backup-options="--single-transaction"`, accepting that non-transactional tables are then not guaranteed consistent.

**MongoDB backup example:**

```sh
mariadb-backup-s3 backup \
  --db-engine=mongodb \
  --db-host=mongo.example.com \
  --db-port=27017 \
  --db-user=backup \
  --db-password=secret \
  --storage-url="s3://my-bucket/mongodb-backups"
```

With `--db-engine=mongodb`, the method defaults to `mongodump` and a single `mongodump` invocation dumps all databases. Credentials are passed to `mongodump` through a temporary config file outside the backup directory, so they never reach the command line or the archive. Use `--mongo-uri` instead of host/port/user to connect via a MongoDB connection URI.

**MongoDB limitations and consistency:**

- `mongodump` is a logical backup, not a physical snapshot or PITR solution.
- Without `--oplog`, a dump is not a globally consistent point-in-time snapshot. For replica sets, opt in with `--db-backup-options="--oplog"`.
- Sharded clusters are out of scope: they need per-shard handling.
- Mixed-engine buckets are supported through registry metadata, but separate storage prefixes (or explicit `--backup-id`) are recommended for strict separation.

**Environment-only (minimal):**

```sh
export MARIADB__HOST=localhost
export MARIADB__USER=root
export MARIADB__PASSWORD=secret
export MARIADB__BACKUP_METHOD=mariadb-backup
export STORAGE__URL=s3://my-bucket/backups
export AWS_REGION=eu-west-1
export AWS_ACCESS_KEY_ID=xxx
export AWS_SECRET_ACCESS_KEY=yyy
mariadb-backup-s3
```

<p align="right">(<a href="#readme-top">back to top</a>)</p>

### Restore

```sh
mariadb-backup-s3 restore [options] [backup_name.tar.gz | --latest | --backup-id=<id>]
```

Exactly one backup selector must be specified: a filename argument, `--latest`, or `--backup-id`.

**Engine detection:** the restore engine is resolved in this order:

1. Explicit `--db-engine` (flag or `DB__ENGINE`) -- always wins, a warning is logged if it contradicts the registry.
2. Registry `tool.method` of the selected backup (for `--latest` and `--backup-id`): `mongodump` selects `mongodb`, anything else selects `mariadb`.
3. Default: `mariadb`.

A filename argument without `--db-engine` restores as `mariadb` (MariaDB restore is extract-only).

**Options:**

| Option                             | Env Var                             | Description                                                  | Default                                     |
| ---------------------------------- | ----------------------------------- | ------------------------------------------------------------ | ------------------------------------------- |
| **Database**                       |                                     |                                                              |                                             |
| `--db-engine`                      | `DB__ENGINE`                        | Restore engine: `mariadb` or `mongodb` (see detection above) | auto-detected                               |
| `--db-host`, `--host`              | `DB__HOST`                          | Database hostname                                            | `localhost`                                 |
| `--db-port`, `--port`              | `DB__PORT`                          | Database port (engine-dependent)                             | `3306` / `27017`                            |
| `--db-user`, `--user`              | `DB__USER`                          | Database username                                            | `root` (mariadb), empty (mongodb)           |
| `--db-password`, `--password`      | `DB__PASSWORD`                      | Database password                                            | `""`                                        |
| `--db-restore-binary`              | `DB__RESTORE_BINARY`                | Restore binary path                                          | engine default (`mongorestore` for mongodb) |
| `--db-restore-options`             | `DB__RESTORE_OPTIONS`               | Extra restore tool options (sanitized)                       | `""`                                        |
| `--mongo-uri`                      | `MONGODB__URI`                      | MongoDB connection URI (replaces host/port/user)             | `""`                                        |
| `--mongo-auth-source`              | `MONGODB__AUTH_SOURCE`              | MongoDB authentication source database                       | `admin` (mongodb)                           |
| `--mongo-tls`                      | `MONGODB__TLS`                      | Enable MongoDB TLS                                           | `false`                                     |
| `--mongo-tls-ca-file`              | `MONGODB__TLS_CA_FILE`              | MongoDB TLS CA file                                          | `""`                                        |
| `--mongo-tls-certificate-key-file` | `MONGODB__TLS_CERTIFICATE_KEY_FILE` | MongoDB TLS certificate key file                             | `""`                                        |
| **Storage / Encryption / Restore** |                                     |                                                              |                                             |
| `--storage-url`, `--storage`       | `STORAGE__URL`                      | Storage URL                                                  | **required**                                |
| `--encryption-key`                 | `ENCRYPTION__KEY`                   | Encryption key                                               | `""`                                        |
| `--target-dir`                     | `RESTORE__TARGET_DIR`               | Target directory (meaning depends on engine, see note)       | **required**                                |
| `--latest`                         |                                     | Restore latest ready backup from registry                    | `false`                                     |
| `--backup-id`                      |                                     | Restore backup by registry ID                                | `""`                                        |

**Restore latest backup:**

```sh
mariadb-backup-s3 restore \
  --storage-url="s3://bucket/path" \
  --target-dir=/var/lib/mysql \
  --latest
```

**Restore by filename or ID:**

```sh
mariadb-backup-s3 restore --storage-url="s3://bucket/path" \
  --target-dir=/tmp/restore \
  2026-07-03-12-00-00.tar.gz

mariadb-backup-s3 restore --storage-url="s3://bucket/path" \
  --target-dir=/tmp/restore \
  2026-07-03-12-00-00.tar.gz.enc  # .enc suffix for encrypted backups

mariadb-backup-s3 restore --storage-url="s3://bucket/path" \
  --target-dir=/tmp/restore \
  --backup-id="2026-07-03-12-00-00-a1b2c3d4"
```

> **Note**
> Logical backups (`mariadb-dump`) restore by extracting `.sql` files from the archive. Import them manually:
> ```sh
> mariadb-backup-s3 restore --storage-url="s3://bucket/path" --target-dir=/tmp/restore --latest
> mariadb -u root -p < /tmp/restore/mydb.sql
> ```

**Restore a MongoDB backup:**

```sh
# Engine auto-detected from the registry tool.method=mongodump entry
mariadb-backup-s3 restore \
  --storage-url="s3://my-bucket/mongodb-backups" \
  --target-dir=/tmp/mongo-restore \
  --latest \
  --db-restore-options="--drop --oplogReplay"

# Filename argument: pass the engine explicitly
mariadb-backup-s3 restore \
  --storage-url="s3://my-bucket/mongodb-backups" \
  --target-dir=/tmp/mongo-restore \
  --db-engine=mongodb \
  2026-07-03-12-00-00.tar.gz
```

`--drop` and `--oplogReplay` are passed through to `mongorestore` via `--db-restore-options` (sanitized before execution).

> **Note**
> For the `mongodb` engine, `--target-dir` is the extraction/staging directory: the archive is decrypted, decompressed, and extracted there, then `mongorestore --dir=<target-dir>` runs against it. For `mariadb`, `--target-dir` remains the destination directory (restore is extract-only).

<p align="right">(<a href="#readme-top">back to top</a>)</p>

### Retention

Apply retention policies to prune old backups.

```sh
mariadb-backup-s3 retention [options]
```

**Options:**

| Option              | Env Var                   | Description                      | Default        |
| ------------------- | ------------------------- | -------------------------------- | -------------- |
| `--storage-url`     | `STORAGE__URL`            | Storage URL                      | **required**   |
| `--retention-count` | `RETENTION__COUNT`        | Number of backups to retain      | `0`            |
| `--max-age`         | `RETENTION__MAX_AGE`      | Maximum age (e.g. `24h`, `168h`) | unlimited      |
| `--keep-daily`      | `RETENTION__KEEP_DAILY`   | Keep N per day                   | `0` (disabled) |
| `--keep-weekly`     | `RETENTION__KEEP_WEEKLY`  | Keep N per week                  | `0` (disabled) |
| `--keep-monthly`    | `RETENTION__KEEP_MONTHLY` | Keep N per month                 | `0` (disabled) |
| `--dry-run`         | `RETENTION__DRY_RUN`      | Preview without deleting         | `false`        |
| `--force`           | `RETENTION__FORCE`        | Continue despite errors          | `false`        |

**Examples:**

```sh
# Keep only the 7 most recent backups
mariadb-backup-s3 retention \
  --storage-url="s3://my-bucket/backups" \
  --retention-count=7

# Keep backups from the last 7 days
mariadb-backup-s3 retention \
  --storage-url="s3://my-bucket/backups" \
  --max-age=168h

# Keep 1 daily for 7 days, 1 weekly for 4 weeks, 1 monthly for 12 months
mariadb-backup-s3 retention \
  --storage-url="s3://my-bucket/backups" \
  --keep-daily=7 \
  --keep-weekly=4 \
  --keep-monthly=12

# Preview what would be deleted
mariadb-backup-s3 retention \
  --storage-url="s3://my-bucket/backups" \
  --retention-count=3 \
  --dry-run
```

> **Note**
> At least one retention policy must be enabled. The retention command will fail if no policies are specified.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

### Registry

View and manage the backup registry.

```sh
mariadb-backup-s3 registry list [options]
```

| Option          | Env Var        | Description                                               | Default      |
| --------------- | -------------- | --------------------------------------------------------- | ------------ |
| `--storage-url` | `STORAGE__URL` | Storage URL                                               | **required** |
| `--rebuild`     |                | Rebuild the registry from stored backups if it is missing | `false`      |

```sh
mariadb-backup-s3 registry list \
  --storage-url="s3://my-bucket/backups"
```

**Registry entry fields:**

| Field                       | Description                                                     |
| --------------------------- | --------------------------------------------------------------- |
| `id`                        | Unique identifier (timestamp + SHA256 prefix)                   |
| `filename`                  | Backup filename                                                 |
| `created_at`                | Creation timestamp                                              |
| `size_bytes`                | File size in bytes                                              |
| `sha256`                    | SHA256 hash of the backup file                                  |
| `status`                    | `ready`, `failed`, or `deleted`                                 |
| `encrypted`                 | Whether the backup is encrypted                                 |
| `encryption`                | Encryption metadata (algorithm)                                 |
| `tool.name`, `tool.version` | Tool name and version                                           |
| `tool.method`               | Backup method: `mariadb-backup`, `mariadb-dump`, or `mongodump` |

> **Note**
> Entries rebuilt with `--rebuild` use timestamp-only IDs, set `sha256` to `"unknown"`, and have an empty `tool.method`.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

### Scheduler

Built-in cron daemon that executes backup and retention jobs on a schedule without external cron or systemd timers.

```sh
mariadb-backup-s3 scheduler <command> [options]
```

**Subcommands:**

| Command  | Description                           |
| -------- | ------------------------------------- |
| `run`    | Start the scheduler daemon            |
| `status` | Show the status of scheduled jobs     |
| `check`  | Validate a scheduler YAML config file |

**Options:**

| Option         | Env Var                 | Description                         | Default      |
| -------------- | ----------------------- | ----------------------------------- | ------------ |
| `--config`     | `SCHEDULER__CONFIG`     | Path to scheduler YAML config       | **required** |
| `--state-file` | `SCHEDULER__STATE_FILE` | Path to persistent state file       | from config  |
| `--once`       |                         | Run all enabled jobs once then exit | `false`      |

**Example scheduler config:**

```yaml
# scheduler.yaml
state_file: /var/lib/mariadb-backup-s3/state.json
jobs:
  - name: nightly-full-backup
    schedule: "0 2 * * *"
    command: backup
    timeout: 4h
    storage:
      url: s3://my-bucket/backups?endpoint=https://s3.custom.com
    mariadb:
      host: localhost
      port: 3306
      user: root
      password: ${MARIADB__PASSWORD}
      backup_method: mariadb-backup
    retention:
      max_count: 7

  - name: nightly-mongodb-backup
    schedule: "30 2 * * *"
    command: backup
    timeout: 4h
    storage:
      url: s3://my-bucket/mongodb-backups
    database:
      engine: mongodb
      host: localhost
      port: 27017
      user: backup
      password: ${DB__PASSWORD}
      backup_method: mongodump
      auth_source: admin
```

The `database:` section works for both engines. Keys: `engine`, `host`, `port`, `user`, `password`, `backup_method`, `backup_binary`, `client_binary`, `restore_binary`, `backup_options`, `restore_options`, `uri`, `auth_source`, `tls`, `tls_ca_file`, `tls_certificate_key_file`.

> **Note**
> The legacy `mariadb:` section is still supported. Setting both `database:` and `mariadb:` in one job is a validation error.

```sh
# Validate the config
mariadb-backup-s3 scheduler check --config scheduler.yaml

# Run the scheduler daemon
mariadb-backup-s3 scheduler run --config scheduler.yaml

# Run all enabled jobs once and exit
mariadb-backup-s3 scheduler run --config scheduler.yaml --once

# Check job status
mariadb-backup-s3 scheduler status --config scheduler.yaml --state-file /var/lib/mariadb-backup-s3/state.json
```

> **Note**
> See [examples/scheduler](./examples/scheduler/) for a complete setup guide.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- STORAGE TYPES -->
## Storage Types

### S3 Storage

For S3-compatible storage (AWS S3, MinIO, DigitalOcean Spaces, etc.):

```dotenv
STORAGE__URL=s3://bucket-name/path?endpoint=https://s3.example.com
```

**Required environment variables:**

| Env Var                 | Description |
| ----------------------- | ----------- |
| `AWS_REGION`            | AWS region  |
| `AWS_ACCESS_KEY_ID`     | Access key  |
| `AWS_SECRET_ACCESS_KEY` | Secret key  |

**Query parameters:**

| Parameter             | Description                                                                          |
| --------------------- | ------------------------------------------------------------------------------------ |
| `endpoint`            | S3 endpoint URL                                                                      |
| `s3-force-path-style` | Set to `true` to use path-style URLs                                                 |
| `part-size`           | Multipart upload part size in bytes (default: 10485760 = 10 MB, min: 5242880 = 5 MB) |

### FTP Storage

```dotenv
STORAGE__URL=ftp://username:password@host:port/path
```

Username defaults to `anonymous`.

### Filesystem Storage

```dotenv
STORAGE__URL=file:///absolute/path/to/backup/directory
```

Examples:
- Linux/macOS: `file:///var/backups/mariadb`
- Windows: `file:///C:/backups/mariadb`
- Docker volume: `file:///data/backups`

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- ENCRYPTION -->
## Encryption

Client-side encryption using AES-256-GCM for confidentiality and integrity verification.

**Features:**
- 256-bit key strength
- Authenticated encryption with associated data (AEAD)
- Automatic nonce generation per backup
- HKDF-SHA256 key derivation

**Configuration:**

```dotenv
# base64-encoded encryption key
# Replace with a unique value generated by: openssl rand -base64 32
ENCRYPTION__KEY=REPLACE_WITH_A_UNIQUE_BASE64_KEY
```

**Key generation:**

```sh
# Generate a 32-byte (256-bit) key
openssl rand -base64 32

# Alternative
head -c 32 /dev/urandom | base64
```

**Security considerations:**

- Never commit encryption keys to version control
- Store keys in secure environment variables or dedicated secret management systems
- Implement proper access controls for key storage

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- EXAMPLES -->
## Examples

- [systemd Service](./examples/systemd-service/) -- Run as a systemd service
- [Docker Swarm CRON](./examples/docker-cron-backup/) -- Scheduled backups in Docker Swarm
- [Simple CRON](./examples/simple-cron-backup/) -- Basic cron-based scheduling
- [Advanced CRON](./examples/advanced-cron-backup/) -- Advanced cron with logging and notifications
- [Encryption](./examples/encryption-example/) -- Encrypted backup setup
- [Scheduler](./examples/scheduler/) -- Built-in scheduler daemon

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- CONTRIBUTING -->
## Contributing

Contributions are what make the open source community such an amazing place to learn, inspire, and create. Any contributions you make are **greatly appreciated**.

1. Fork the Project
2. Create your Feature Branch (`git checkout -b feature/amazing-feature`)
3. Commit your Changes (`git commit -m 'Add amazing feature'`)
4. Push to the Branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

See [CONTRIBUTORS.md](CONTRIBUTORS.md) for a list of contributors.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- LICENSE -->
## License

Distributed under the Apache 2.0 License. See [LICENSE](LICENSE) for details.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- CONTACT -->
## Contact

- [Report a bug](https://github.com/capcom6/mariadb-backup-s3/issues/new?labels=bug&template=bug-report---.md)
- [Request a feature](https://github.com/capcom6/mariadb-backup-s3/issues/new?labels=enhancement&template=feature-request---.md)
- [View all issues](https://github.com/capcom6/mariadb-backup-s3/issues)

Project Link: [https://github.com/capcom6/mariadb-backup-s3](https://github.com/capcom6/mariadb-backup-s3)

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- ACKNOWLEDGMENTS -->
## Acknowledgments

- [Choose an Open Source License](https://choosealicense.com)
- [Img Shields](https://shields.io)
- [Best-README-Template](https://github.com/othneildrew/Best-README-Template)

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- MARKDOWN LINKS & IMAGES -->
[contributors-shield]: https://img.shields.io/github/contributors/capcom6/mariadb-backup-s3.svg?style=for-the-badge
[contributors-url]: https://github.com/capcom6/mariadb-backup-s3/graphs/contributors
[forks-shield]: https://img.shields.io/github/forks/capcom6/mariadb-backup-s3.svg?style=for-the-badge
[forks-url]: https://github.com/capcom6/mariadb-backup-s3/network/members
[stars-shield]: https://img.shields.io/github/stars/capcom6/mariadb-backup-s3.svg?style=for-the-badge
[stars-url]: https://github.com/capcom6/mariadb-backup-s3/stargazers
[issues-shield]: https://img.shields.io/github/issues/capcom6/mariadb-backup-s3.svg?style=for-the-badge
[issues-url]: https://github.com/capcom6/mariadb-backup-s3/issues
[license-shield]: https://img.shields.io/github/license/capcom6/mariadb-backup-s3.svg?style=for-the-badge
[license-url]: https://github.com/capcom6/mariadb-backup-s3/blob/master/LICENSE
[go-version-shield]: https://img.shields.io/github/go-mod/go-version/capcom6/mariadb-backup-s3?style=for-the-badge
