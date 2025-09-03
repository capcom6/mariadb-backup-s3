# CRON Backup Example (Minimal)

This example demonstrates a minimal setup for automated MariaDB backups using system CRON with the mariadb-backup-s3 tool.

## 📁 Files Structure

```
examples/simple-cron-backup/
├── README.md
├── .env.example
└── crontab.example
```

## 📋 Prerequisites

To ensure successful backups:

- At least 2x the actual database size in free space available

## 🚀 Setup Instructions

### 1. Create a Dedicated User

Create a system user for backups:

```bash
sudo adduser --system --group --home /var/backups backup
```

### 2. Configure Environment

Copy the environment template to the backup user’s home directory:

```bash
sudo install -m 600 -o backup -g backup .env.example /var/backups/.env
sudo nano /var/backups/.env
```

### 3. Install CRON Job

Add the CRON job:

```bash
sudo -u backup crontab -e
```

Insert the line from `crontab.example`:

```
0 2 * * * mariadb-backup-s3
```

## 📋 Best Practices

- Keep your `.env` file secure with proper permissions (e.g., chmod 600)
- Test the backup command manually before relying on CRON
- Monitor backup logs for failures
