# CRON Backup Example

This example demonstrates how to set up automated MariaDB backups using system CRON with the mariadb-backup-s3 tool.

> **Recommendation**
> The built-in scheduler (`mariadb-backup-s3 scheduler run --config scheduler.yaml`) provides a better alternative with persistent state, built-in safety features, and no need for external cron configuration. See [examples/scheduler](../scheduler/) for details.

## 📁 Files Structure

```
examples/cron-backup/
├── README.md              # This file
├── backup.sh             # Backup script wrapper
├── crontab.example       # Example crontab configuration
└── backup.env.example    # Environment variables template
```

## 📋 Prerequisites

To ensure successful backups:

- At least 2x the actual database size in free space available

## 🚀 Setup Instructions

### 1. Create Environment File

Copy the environment template and customize it:

```bash
cp backup.env.example backup.env
nano backup.env
```

### 2. Make Backup Script Executable

```bash
chmod +x backup.sh
```

### 3. Install CRON Job

Choose one of the following methods:

#### Method A: Install to System Crontab

```bash
# Edit system crontab
sudo crontab -e

# Add the following line (adjust paths as needed):
0 2 * * * /path/to/mariadb-backup-s3/examples/cron-backup/backup.sh
```

#### Method B: Install to User Crontab

```bash
# Edit user crontab
crontab -e

# Add the following line (adjust paths as needed):
0 2 * * * /path/to/mariadb-backup-s3/examples/cron-backup/backup.sh
```

#### Method C: Use the Example Crontab

```bash
# Copy and install the example crontab
cp crontab.example /tmp/my-crontab
crontab /tmp/my-crontab
rm /tmp/my-crontab
```

## ⏰ CRON Schedule Examples

The `crontab.example` file includes several common backup schedules:

```bash
# Daily backup at 2 AM
0 2 * * * /path/to/backup.sh

# Weekly backup on Sunday at 3 AM
0 3 * * 0 /path/to/backup.sh

# Monthly backup on the 1st at 4 AM
0 4 1 * * /path/to/backup.sh

# Every 6 hours
0 */6 * * * /path/to/backup.sh
```

## 📧 Logging and Notifications

The backup script includes logging functionality. By default, logs are written to:

- `/var/log/mariadb-backup.log` - Main backup log
- `/var/log/mariadb-backup-error.log` - Error log

To enable email notifications on failure, ensure your system has a working `mail` command and set `NOTIFY_EMAIL` in `backup.env`. Example:

```bash
NOTIFY_EMAIL="admin@example.com"
```

## 🔧 Customization

### Environment Variables

Edit `backup.env` to match your database and storage configuration:

```bash
# MariaDB Configuration
MARIADB__USER=root
MARIADB__PASSWORD=your_strong_password
MARIADB__HOST=localhost
MARIADB__PORT=3306

# Storage Configuration
STORAGE__URL=s3://your-bucket/backups?endpoint=https://s3.endpoint

# S3 Configuration (when using S3 storage)
AWS_ACCESS_KEY_ID=your_access_key
AWS_SECRET_ACCESS_KEY=your_secret_key
AWS_REGION=us-east-1

# Backup Settings
RETENTION__COUNT=30  # Keep last 30 backups
```

### Backup Script Options

The `backup.sh` script can be customized:

```bash
# Path to the mariadb-backup-s3 binary
BACKUP_BINARY="/usr/local/bin/mariadb-backup-s3"

# Environment file path
ENV_FILE="/path/to/backup.env"

# Log file paths
LOG_FILE="/var/log/mariadb-backup.log"
ERROR_LOG="/var/log/mariadb-backup-error.log"

# Email for notifications (optional)
NOTIFY_EMAIL="admin@example.com"
```

## 🔍 Troubleshooting

### Check CRON Service Status

```bash
# Systemd-based systems
sudo systemctl status cron

# SysV-init systems
sudo service cron status
```

### Check CRON Logs

```bash
# Systemd journal
sudo journalctl -u cron -f

# Traditional syslog
tail -f /var/log/syslog | grep CRON
```

### Test Backup Script Manually

```bash
# Run the backup script manually to test
./backup.sh

# Check exit code
echo $?
```

### Check Backup Logs

```bash
# View main log
tail -f /var/log/mariadb-backup.log

# View error log
tail -f /var/log/mariadb-backup-error.log
```

## 📋 Best Practices

1. **Backup Retention**: Configure `RETENTION__COUNT` to keep an appropriate number of backups
2. **Monitoring**: Set up log monitoring to alert on backup failures
3. **Testing**: Regularly test backup restoration process
4. **Security**: Restrict access to backup files and environment variables
5. **Documentation**: Document your backup and restore procedures

## 🔄 Rotation and Cleanup

The mariadb-backup-s3 tool automatically handles backup rotation based on the `RETENTION__COUNT` setting. Older backups are automatically removed when this limit is exceeded.

## 📊 Monitoring

Consider setting up monitoring for your backups:

```bash
# Add to crontab to check if backup ran successfully in the last 24 hours
30 8 * * * if [ -f "/var/log/mariadb-backup.log" ] && [ $(find /var/log/mariadb-backup.log -mtime -1 | wc -l) -eq 0 ]; then echo "Backup may have failed" | mail -s "Backup Alert" admin@example.com; fi
