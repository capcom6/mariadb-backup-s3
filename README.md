# 🗄️ MariaDB Backup to S3

![Go Version](https://img.shields.io/github/go-mod/go-version/capcom6/mariadb-backup-s3)
![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)

🔁 Automated MariaDB database backups with S3-compatible storage integration

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
mariadb-backup-s3
```

## ✨ Features

- 🛡️ Full database backups using `mariabackup`
- 🗜️ Compression to `.tar.gz` format
- ☁️ Multiple storage backends (S3-compatible, filesystem)
- 🔌 Pluggable storage interface for extensibility
- 🔄 Automatic backup rotation
- 🐳 Docker container support

## 🛠️ How It Works

The backup process follows these steps:

1. 📂 Create temporary working directory
2. 💾 Perform MariaDB backup using `mariabackup --backup`
3. 🔧 Prepare backup for consistency using `mariabackup --prepare`
4. 🗜️ Compress backup to `.tar.gz` archive
5. 🚀 Upload archive to S3-compatible storage
6. 🧹 Clean up old backups based on retention policy

## 📋 Prerequisites

- Go 1.22+ (for building from source)
- MariaDB server
- AWS credentials with S3 access
- S3-compatible storage bucket

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

### From Source (Advanced)
```shell
git clone https://github.com/capcom6/mariadb-backup-s3.git
cd mariadb-backup-s3
go build -o mariadb-backup-s3
```

## ⚙️ Configuration

### Environment Variables
Create `.env` file with these variables:

```dotenv
# MariaDB Configuration
MARIADB__USER=root
MARIADB__PASSWORD=your_strong_password
MARIADB__HOST=localhost
MARIADB__PORT=3306

# Storage Configuration
STORAGE__URL=s3://your-bucket/backups?endpoint=https://s3.endpoint

# S3 Configuration (when STORAGE__TYPE=s3)
AWS_ACCESS_KEY=your_access_key
AWS_SECRET_KEY=your_secret_key
AWS_REGION=us-east-1

# Backup Settings
BACKUP__LIMITS__MAX_COUNT=30  # Keep last 30 backups
```

| Variable                    | Default      | Description                          |
| --------------------------- | ------------ | ------------------------------------ |
| `MARIADB__HOST`             | localhost    | Database host address                |
| `MARIADB__PORT`             | 3306         | Database port                        |
| `MARIADB__USER`             | root         | Database user                        |
| `MARIADB__PASSWORD`         | -            | Database password                    |
| `MARIADB__BACKUP_OPTIONS`   | -            | Extra `mariabackup` options          |
| `STORAGE__URL`              | **Required** | Storage URL (format depends on type) |
| `BACKUP__LIMITS__MAX_COUNT` | 30           | Maximum backups to retain            |

### Storage Types

#### S3 Storage
For S3-compatible storage (including AWS S3, MinIO, DigitalOcean Spaces, etc.):

```dotenv
STORAGE__URL=s3://bucket-name/path?endpoint=https://s3.example.com&region=us-east-1
```

**Required for S3:**
- `AWS_ACCESS_KEY`: Your access key
- `AWS_SECRET_KEY`: Your secret key
- `AWS_REGION`: AWS region (or any region for non-AWS S3)

#### Filesystem Storage
For local or mounted filesystem storage:

```dotenv
STORAGE__URL=file:///absolute/path/to/backup/directory
```

**Examples:**
- Linux/macOS: `file:///var/backups/mariadb`
- Windows: `file://C:/backups/mariadb`
- Docker volume: `file:///data/backups`

### Command-Line Flags
Override any configuration with flags:

```shell
./mariadb-backup-s3 \
  --db-host=mariadb.example.com \
  --db-password=secret \
  --storage-url="file:///var/backups/mariadb"
```

| Flag                  | Description                         |
| --------------------- | ----------------------------------- |
| `--db-host`           | Override database host              |
| `--db-port`           | Override database port              |
| `--db-user`           | Specify database user               |
| `--db-password`       | Set database password               |
| `--db-backup-options` | Additional `mariabackup` parameters |
| `--storage-url`       | Custom storage URL                  |

## 🐳 Docker Usage

### Basic Example
```shell
docker run --rm \
  -v /var/lib/mysql:/var/lib/mysql \
  -v /var/backups:/backups \
  --env-file .env \
  ghcr.io/capcom6/mariadb-backup-s3
```

### Filesystem Storage Example
```shell
# Create .env file for filesystem storage
cat > .env << EOF
MARIADB__USER=root
MARIADB__PASSWORD=secret
STORAGE__URL=file:///backups/mariadb
BACKUP__LIMITS__MAX_COUNT=7
EOF

# Run with filesystem storage
docker run --rm \
  -v /var/lib/mysql:/var/lib/mysql \
  -v /var/backups:/backups \
  --env-file .env \
  ghcr.io/capcom6/mariadb-backup-s3
```

### Docker Swarm Example

The example can be found in [examples/docker-cron-backup](./examples/docker-cron-backup/compose.yml)

> **Note**
> The Docker image uses MariaDB's `lts` version. For specific versions:
> 1. Clone the repository
> 2. Modify `Dockerfile` base image
> 3. Build custom image: `docker build -t custom-backup-image .`

## 🤝 Contributing

We welcome contributions! Please follow these steps:

1. 🍴 Fork the repository
2. 🌿 Create a feature branch: `git checkout -b feat/amazing-feature`
3. 💾 Commit changes: `git commit -m 'Add amazing feature'`
4. 🚀 Push to branch: `git push origin feat/amazing-feature`
5. 🔀 Create a Pull Request

## 📄 License

Apache 2.0 - See [LICENSE](LICENSE) for details.

---

💡 **Need Help?** Open an [issue](https://github.com/capcom6/mariadb-backup-s3/issues) for support.
