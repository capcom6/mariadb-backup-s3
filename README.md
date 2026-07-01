# 🗄️ MariaDB Backup to S3

![Go Version](https://img.shields.io/github/go-mod/go-version/capcom6/mariadb-backup-s3)
![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)

🔁 Automated MariaDB database backups with S3-compatible storage integration

## Table of Contents
- [🗄️ MariaDB Backup to S3](#️-mariadb-backup-to-s3)
  - [Table of Contents](#table-of-contents)
  - [🚀 Quick Start](#-quick-start)
  - [✨ Features](#-features)
  - [🛠️ How It Works](#️-how-it-works)
    - [Temporary Working Directory](#temporary-working-directory)
    - [Backup Registry](#backup-registry)
  - [📋 Prerequisites](#-prerequisites)
  - [📦 Installation](#-installation)
    - [Binary Installation (Recommended)](#binary-installation-recommended)
    - [Using Go Install](#using-go-install)
    - [Docker](#docker)
    - [From Source (Advanced)](#from-source-advanced)
  - [⚙️ Configuration](#️-configuration)
    - [Logging Configuration](#logging-configuration)
  - [🚀 Usage](#-usage)
    - [Backup](#backup)
    - [Restore](#restore)
    - [Retention](#retention)
    - [Registry](#registry)
    - [Scheduler](#scheduler)
  - [📂 Storage Types](#-storage-types)
    - [S3 Storage](#s3-storage)
    - [FTP Storage](#ftp-storage)
    - [Filesystem Storage](#filesystem-storage)
  - [🔐 Encryption](#-encryption)
    - [Security Considerations](#security-considerations)
      - [Key Management](#key-management)
      - [Key Generation](#key-generation)
  - [📝 Examples](#-examples)
  - [🤝 Contributing](#-contributing)
  - [👥 Contributors](#-contributors)
  - [📄 License](#-license)


## 🚀 Quick Start

```shell
# Using pre-built binary (check Releases page for latest version)
curl -LO https://github.com/capcom6/mariadb-backup-s3/releases/latest/download/mariadb-backup-s3_Linux_x86_64.tar.gz
tar -xzf mariadb-backup-s3_Linux_x86_64.tar.gz
chmod +x mariadb-backup-s3
./mariadb-backup-s3 --help

# Or via go install
go install github.com/capcom6/mariadb-backup-s3@latest

# Configure & run
cp .env.example .env
nano .env  # Edit with your credentials
./mariadb-backup-s3
```

## ✨ Features

- 🛡️ Full database backups using `mariabackup`
- 🗜️ Compression to `.tar.gz` format
- 🔑 Optional encryption using AES-256-GCM
- ☁️ Multiple storage backends (S3-compatible, FTP, filesystem)
- 🔌 Pluggable storage interface for extensibility
- 🔄 Automatic backup rotation with configurable retention policies
- 📋 Backup registry for tracking and managing backups
- 🐳 Docker container support
- ⏰ Built-in scheduler — run scheduled backups as a long-running daemon, no external cron needed

## 🛠️ How It Works

The backup process follows these steps:

1. 📂 Create temporary working directory
2. 💾 Perform MariaDB backup using `mariabackup --backup`
3. 🔧 Prepare backup for consistency using `mariabackup --prepare`
4. 🗜️ Compress backup to `.tar.gz` archive
5. 🔑 Encrypt archive using AES-256-GCM
6. 🚀 Upload archive to configured storage backend
7. 📋 Update backup registry with new backup metadata
8. 🧹 Clean up old backups based on retention policy (unless `--skip-retention` is set)

The restore process follows these steps:

1. 📥 Download the backup file from the specified storage backend
2. 🔑 Decrypt the backup using AES-256-GCM
3. 🗜️ Decompress the `.tar.gz` archive
4. 🔄 Restore the database files to specified directory

### Temporary Working Directory

The tool uses a temporary working directory to store intermediate files. By default, it uses the system's default temporary directory (e.g., `/tmp`). If for some reason you need to use a different directory, you can specify it via the `TMPDIR` environment variable.

```bash
export TMPDIR=/mnt/data/backup
./mariadb-backup-s3 backup
```

### Backup Registry

The tool maintains a backup registry (`.backup-registry.json`) in the storage backend to track all backups. This registry enables:

- **Backup tracking**: Each backup is recorded with metadata including ID, filename, creation time, size, SHA256 hash, encryption status, and tool version
- **Retention management**: The registry is used to apply retention policies and clean up old backups
- **Backup listing**: View all available backups with their status and metadata

The registry is automatically updated when backups are created or deleted. Each backup entry includes:

| Field        | Description                                   |
| ------------ | --------------------------------------------- |
| `id`         | Unique identifier (timestamp + SHA256 prefix) |
| `filename`   | Backup filename                               |
| `created_at` | Creation timestamp                            |
| `size_bytes` | File size in bytes                            |
| `sha256`     | SHA256 hash of the backup file                |
| `status`     | Status: `ready`, `failed`, or `deleted`       |
| `encrypted`  | Whether the backup is encrypted               |
| `encryption` | Encryption metadata (algorithm)               |
| `tool`       | Tool name and version                         |

## 📋 Prerequisites

- Go 1.23+ (for building from source)
- MariaDB server
- At least 2x the actual database size in free space (for successful backup)
- Storage backend credentials (depending on chosen storage type)

## 📦 Installation

### Binary Installation (Recommended)
1. Visit the [Releases page](https://github.com/capcom6/mariadb-backup-s3/releases/latest)
2. Download the appropriate binary for your OS
3. Make executable: `chmod +x mariadb-backup-s3`
4. Move to PATH: `sudo mv mariadb-backup-s3 /usr/local/bin/`

### Using Go Install
```shell
go install github.com/capcom6/mariadb-backup-s3@latest
```

### Docker
```shell
docker pull ghcr.io/capcom6/mariadb-backup-s3:latest
```

> **Note**
> The Docker image uses MariaDB's `lts` version. For specific versions:
> 1. Clone the repository
> 2. Modify `Dockerfile` base image
> 3. Build custom image: `docker build -t custom-backup-image .`

### From Source (Advanced)
```shell
git clone https://github.com/capcom6/mariadb-backup-s3.git
cd mariadb-backup-s3
go build -o mariadb-backup-s3
```

## ⚙️ Configuration

The tool supports loading configuration from multiple sources:

1. `.env` file in the current directory
2. Environment variables
3. Command-line flags

The priority order is: `.env` > Environment variables > Command-line flags

### Logging Configuration

The logging system can be configured via environment variables:

- `LOG_LEVEL`: Set log level (debug, info, warn, error, fatal). Default: info
- `LOG_FORMAT`: Set format (human, json). Default: human
- `LOG_OUTPUT`: Set output destination:
  - `stdout` (default): Standard output
  - `stderr`: Standard error
  - Any file path: Write logs to the specified file
- `NO_COLOR`: When set (any non-empty value), disables colored output for human format

## 🚀 Usage

```bash
mariadb-backup-s3 [global options] command [command options] [arguments...]
```

The tool offers the following commands:

| Command              | Description                                       |
| -------------------- | ------------------------------------------------- |
| `backup`             | Perform a backup of the MariaDB database          |
| `restore`            | Restore the database files to specified directory |
| `retention`          | Apply retention policies to backups               |
| `registry`           | Manage backup registry                            |
| `scheduler`, `sched` | Run the backup scheduler daemon                   |

### Backup

```bash
mariadb-backup-s3 backup [options]
```

**Options:**

| Option                        | Env Var                   | Description                                       | Default value    |
| ----------------------------- | ------------------------- | ------------------------------------------------- | ---------------- |
| **Database**                  |                           |                                                   |                  |
| `--db-host`, `--host`         | `MARIADB__HOST`           | MariaDB hostname                                  | `localhost`      |
| `--db-port`, `--port`         | `MARIADB__PORT`           | MariaDB port                                      | `3306`           |
| `--db-user`, `--user`         | `MARIADB__USER`           | MariaDB username                                  | `root`           |
| `--db-password`, `--password` | `MARIADB__PASSWORD`       | MariaDB password                                  | `""`             |
| **Storage**                   |                           |                                                   |                  |
| `--storage`, `--storage-url`  | `STORAGE__URL`            | Storage URL, see [Storage Types](#-storage-types) | **required**     |
| **Encryption**                |                           |                                                   |                  |
| `--encryption-key`            | `ENCRYPTION__KEY`         | Encryption key                                    | `""`             |
| **mariadb-backup**            |                           |                                                   |                  |
| `--db-backup-binary`          | `MARIADB__BACKUP_BINARY`  | MariaDB backup binary path                        | `mariadb-backup` |
| `--db-backup-options`         | `MARIADB__BACKUP_OPTIONS` | MariaDB backup options                            | `""`             |
| **Retention**                 |                           |                                                   |                  |
| `--retention-count`           | `RETENTION__COUNT`        | Number of backups to retain, 0 = unlimited        | `0`              |
| `--max-age`                   | `RETENTION__MAX_AGE`      | Maximum age of backups to keep (e.g. 24h, 168h)   | unlimited        |
| `--keep-daily`                | `RETENTION__KEEP_DAILY`   | Number of daily backups to keep                   | unlimited        |
| `--keep-weekly`               | `RETENTION__KEEP_WEEKLY`  | Number of weekly backups to keep                  | unlimited        |
| `--keep-monthly`              | `RETENTION__KEEP_MONTHLY` | Number of monthly backups to keep                 | unlimited        |
| `--skip-retention`            | `BACKUP__SKIP_RETENTION`  | Skip retention policy after backup                | `false`          |

**Example:**

```shell
./mariadb-backup-s3 backup \
  --db-host=mariadb.example.com \
  --db-user=backup \
  --storage-url="file:///var/backups/mariadb"
```

### Restore

```bash
mariadb-backup-s3 restore [options] [backup_name.tar.gz | --latest | --backup-id=<id>]
```

**Options:**

| Option                       | Env Var               | Description                                       | Default value |
| ---------------------------- | --------------------- | ------------------------------------------------- | ------------- |
| **Storage**                  |                       |                                                   |               |
| `--storage`, `--storage-url` | `STORAGE__URL`        | Storage URL, see [Storage Types](#-storage-types) | **required**  |
| **Encryption**               |                       |                                                   |               |
| `--encryption-key`           | `ENCRYPTION__KEY`     | Encryption key                                    | `""`          |
| **Restore**                  |                       |                                                   |               |
| `--target-dir`               | `RESTORE__TARGET_DIR` | Target directory to restore files to              | **required**  |
| `--latest`                   |                       | Restore latest ready backup from registry         | `false`       |
| `--backup-id`                |                       | Restore backup by registry backup ID              | `""`          |

> **Note**
> Exactly one backup selector must be specified: either a filename argument, `--latest`, or `--backup-id`.

**Arguments:**

| Argument   | Description      |
| ---------- | ---------------- |
| `filename` | Backup file name |

**Example:**

```shell
./mariadb-backup-s3 restore \
  --storage-url="file:///var/backups/mariadb" \
  --target-dir=/var/lib/mariadb \
  backup_name.tar.gz
```

### Retention

The `retention` command applies retention policies to backups stored in the configured storage backend. This allows you to clean up old backups based on various criteria.

```bash
mariadb-backup-s3 retention [options]
```

**Options:**

| Option              | Env Var                   | Description                                          | Default value |
| ------------------- | ------------------------- | ---------------------------------------------------- | ------------- |
| **Storage**         |                           |                                                      |               |
| `--storage-url`     | `STORAGE__URL`            | Storage URL, see [Storage Types](#-storage-types)    | **required**  |
| **Retention**       |                           |                                                      |               |
| `--retention-count` | `RETENTION__COUNT`        | Number of backups to retain, 0 = unlimited           | `0`           |
| `--max-age`         | `RETENTION__MAX_AGE`      | Maximum age of backups to keep (e.g. 24h, 168h)      | unlimited     |
| `--keep-daily`      | `RETENTION__KEEP_DAILY`   | Number of daily backups to keep                      | unlimited     |
| `--keep-weekly`     | `RETENTION__KEEP_WEEKLY`  | Number of weekly backups to keep                     | unlimited     |
| `--keep-monthly`    | `RETENTION__KEEP_MONTHLY` | Number of monthly backups to keep                    | unlimited     |
| **Options**         |                           |                                                      |               |
| `--dry-run`         | `RETENTION__DRY_RUN`      | Show what would be deleted without actually deleting | `false`       |
| `--force`           | `RETENTION__FORCE`        | Continue even if errors occur                        | `false`       |

**Retention Policy Examples:**

```shell
# Keep only the 7 most recent backups
./mariadb-backup-s3 retention \
  --storage-url="s3://my-bucket/backups" \
  --retention-count=7

# Keep backups from the last 7 days
./mariadb-backup-s3 retention \
  --storage-url="s3://my-bucket/backups" \
  --max-age=168h

# Keep 1 daily backup for 7 days, 1 weekly for 4 weeks, 1 monthly for 12 months
./mariadb-backup-s3 retention \
  --storage-url="s3://my-bucket/backups" \
  --keep-daily=7 \
  --keep-weekly=4 \
  --keep-monthly=12

# Preview what would be deleted without actually deleting
./mariadb-backup-s3 retention \
  --storage-url="s3://my-bucket/backups" \
  --retention-count=3 \
  --dry-run
```

> **Note**
> At least one retention policy must be enabled. The retention command will fail if no policies are specified.

### Registry

The `registry` command allows you to manage and view the backup registry.

```bash
mariadb-backup-s3 registry [command] [options]
```

**Subcommands:**

| Command | Alias | Description                |
| ------- | ----- | -------------------------- |
| `list`  | `ls`  | List backups from registry |

**List Command Options:**

| Option          | Env Var        | Description                                       | Default value |
| --------------- | -------------- | ------------------------------------------------- | ------------- |
| `--storage-url` | `STORAGE__URL` | Storage URL, see [Storage Types](#-storage-types) | **required**  |

**Example:**

```shell
./mariadb-backup-s3 registry list \
  --storage-url="s3://my-bucket/backups"
```

**Output:**

The list command displays all backups in the registry with the following information:
- **ID**: Unique backup identifier
- **Created At**: Backup creation timestamp
- **Status**: Backup status (`ready`, `failed`, or `deleted`)
- **Encrypted**: Whether the backup is encrypted
- **Size**: Backup file size in bytes
- **Filename**: Backup filename

### Scheduler

The `scheduler` command runs a built-in cron daemon that executes backup and retention jobs on a schedule without relying on external cron or systemd timers.

```bash
mariadb-backup-s3 scheduler [command] [options]
```

**Subcommands:**

| Command  | Description                           |
| -------- | ------------------------------------- |
| `run`    | Start the scheduler daemon            |
| `status` | Show the status of scheduled jobs     |
| `check`  | Validate a scheduler YAML config file |

**Common Options:**

| Option     | Env Var             | Description                        | Default value |
| ---------- | ------------------- | ---------------------------------- | ------------- |
| `--config` | `SCHEDULER__CONFIG` | Path to scheduler YAML config file | **required**  |

**Scheduler Run Options:**

| Option         | Env Var                 | Description                     | Default value |
| -------------- | ----------------------- | ------------------------------- | ------------- |
| `--state-file` | `SCHEDULER__STATE_FILE` | Path to persistent state file   | from config   |
| `--once`       |                         | Run all due jobs once then exit | `false`       |

**Scheduler Status Options:**

| Option         | Env Var                 | Description                   | Default value            |
| -------------- | ----------------------- | ----------------------------- | ------------------------ |
| `--state-file` | `SCHEDULER__STATE_FILE` | Path to persistent state file | `/tmp/mariadb-scheduler-state.json` |

**Example:**

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
    retention:
      max_count: 7
```

```bash
# Validate the config
mariadb-backup-s3 scheduler check --config scheduler.yaml

# Run the scheduler daemon
mariadb-backup-s3 scheduler run --config scheduler.yaml

# Run all due jobs once and exit
mariadb-backup-s3 scheduler run --config scheduler.yaml --once

# Check job status
mariadb-backup-s3 scheduler status --state-file /var/lib/mariadb-backup-s3/state.json
```

> **Note**
> The scheduler replaces the need for external cron, systemd timers, or shell scripts. See [examples/scheduler](./examples/scheduler/) for a complete setup guide.

## 📂 Storage Types

### S3 Storage
For S3-compatible storage (including AWS S3, MinIO, DigitalOcean Spaces, etc.):

```dotenv
STORAGE__URL=s3://bucket-name/path?endpoint=https://s3.example.com
```

**Required for S3:**
- `AWS_ACCESS_KEY`: Your access key
- `AWS_SECRET_KEY`: Your secret key
- `AWS_REGION`: AWS region (or any region for non-AWS S3)

**Query Parameters:**
- `endpoint`: S3 endpoint URL
- `s3-force-path-style`: Set to "true" to use path-style URLs
- `part-size`: Multipart upload part size in bytes (default: 10485760 = 10 MB, minimum: 5242880 = 5 MB)

### FTP Storage
For FTP servers:

```dotenv
STORAGE__URL=ftp://username:password@host:port/path
```

**Required for FTP:**
- `username`: FTP username (defaults to "anonymous" if not provided)
- `password`: FTP password (optional for anonymous)
- `host`: FTP server hostname
- `port`: FTP port (defaults to 21)

### Filesystem Storage
For local or mounted filesystem storage:

```dotenv
STORAGE__URL=file:///absolute/path/to/backup/directory
```

**Examples:**
- Linux/macOS: `file:///var/backups/mariadb`
- Windows: `file://C:/backups/mariadb`
- Docker volume: `file:///data/backups`

## 🔐 Encryption

The backup system supports client-side encryption using AES-256 in Galois/Counter Mode (GCM) to ensure your database backups remain confidential and secure. This method provides both confidentiality and integrity verification.

**Features:**
- 256-bit key strength
- Authenticated encryption with additional data (AEAD)
- Automatic nonce generation for each backup

**Configuration:**
```dotenv
# base64-encoded encryption key
ENCRYPTION__KEY=Av2cfWJ3enCHTyzPdzowfAXshvJtbEsvwgPjV46wnjc=
```

### Security Considerations

#### Key Management
- **Never commit encryption keys to version control**
- Store keys in secure environment variables or dedicated secret management systems
- Implement proper access controls for key storage

#### Key Generation
Generate secure encryption keys using cryptographically secure methods:

```bash
# Generate a 32-byte (256-bit) key for AES256-GCM
openssl rand -base64 32

# Alternative method using /dev/urandom
head -c 32 /dev/urandom | base64
```

## 📝 Examples

- **systemd Service Example**: [examples/systemd-service](./examples/systemd-service/)
- **Docker Swarm CRON Example**: [examples/docker-cron-backup](./examples/docker-cron-backup/)
- **Simple CRON Example**: [examples/simple-cron-backup](./examples/simple-cron-backup/)
- **Advanced CRON Example**: [examples/advanced-cron-backup](./examples/advanced-cron-backup/)
- **Encryption Example**: [examples/encryption-example](./examples/encryption-example/)
- **Scheduler Example**: [examples/scheduler](./examples/scheduler/)

## 🤝 Contributing

We welcome contributions! Please follow these steps:

1. 🍴 Fork the repository
2. 🌿 Create a feature branch: `git checkout -b feat/amazing-feature`
3. 💾 Commit changes: `git commit -m 'Add amazing feature'`
4. 🚀 Push to branch: `git push origin feat/amazing-feature`
5. 🔀 Create a Pull Request

## 👥 Contributors

A big thank you to everyone who has contributed to this project!

- [gslongo](https://github.com/gslongo)

See the full [CONTRIBUTORS.md](CONTRIBUTORS.md) file for more information.

## 📄 License

Apache 2.0 - See [LICENSE](LICENSE) for details.

---

💡 **Need Help?** Open an [issue](https://github.com/capcom6/mariadb-backup-s3/issues) for support.

