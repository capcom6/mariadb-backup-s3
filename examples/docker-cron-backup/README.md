# Docker Swarm CRON Backup Example 📋

This example demonstrates how to set up automated MariaDB backups using a Docker Swarm cron job with the mariadb-backup-s3 tool. Uses swarm-cronjob mechanism for scheduling via Docker service labels.

## 📋 Purpose
This example shows how to automate MariaDB backups using Docker Swarm. It uses the mariadb-backup-s3 tool for backups and swarm-cronjob for scheduling. The backup process is automated and reliable.

## ⚙️ Prerequisites
* Docker Swarm setup
* At least 2x the actual database size in free space for successful backup

## ⚙️ Environment Setup
Create a `.env` file with:
```
AWS_REGION=your-region
AWS_ACCESS_KEY_ID=your-access-key
AWS_SECRET_ACCESS_KEY=your-secret-key
MARIADB_ROOT_PASSWORD=your-db-password
DB_BACKUP__OPTIONS="--skip-ssl"
DB_BACKUP__SCHEDULE="0 2 * * *"
TIMEZONE="America/New_York"
```

> **Note**
> For MongoDB, use image `ghcr.io/capcom6/mongodb-backup-s3:latest` and add `DB__ENGINE=mongodb` to the backup service environment (adjust `MARIADB__*` connection variables to `DB__HOST`/`DB__PORT=27017`/`DB__USER` or `MONGODB__URI`). The default image `ghcr.io/capcom6/mariadb-backup-s3:latest` ships MariaDB tools only.

## 🚀 Installation
You can deploy this example using either `docker stack deploy` or `docker service create`.

### Using docker stack deploy
```bash
docker stack deploy -c examples/docker-cron-backup/compose.yml mariadb-backup
```

## 🔍 Verification
To verify backups and view logs: check service logs for `db-backup` (Logs reference service name `mariadb-backup-s3` but the container runs as `db-backup`) and verify backup files in your S3 bucket.
