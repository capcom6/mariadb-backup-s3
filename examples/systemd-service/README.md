# MariaDB Backup to S3 with systemd

This example shows how to run the `mariadb-backup-s3` tool as a scheduled service using systemd.

## Files

- `mariadb-backup-s3.service`: a systemd service unit file
- `mariadb-backup-s3.timer`: a systemd timer for scheduling

## Setup Instructions

### 1. Install the binary
Place the compiled `mariadb-backup-s3` binary in `/usr/local/bin/`:

```bash
sudo install -m 0755 mariadb-backup-s3 /usr/local/bin/
```

### 2. Create configuration
Create environment file at `/etc/default/mariadb-backup-s3` with your configuration:

```ini
# Database configuration
MARIADB__HOST=localhost
MARIADB__PORT=3306
MARIADB__USER=backup
MARIADB__PASSWORD=your_secure_password
MARIADB__BACKUP_OPTIONS=--skip-ssl --parallel=4

# Storage configuration (S3)
STORAGE__URL=s3://your-bucket-name/backup-path?endpoint=https://s3.example.com
AWS_ACCESS_KEY_ID=your_access_key
AWS_SECRET_ACCESS_KEY=your_secret_key
AWS_REGION=us-east-1

# Backup retention
RETENTION__COUNT=30

# Scheduler configuration (only needed for the scheduler daemon)
SCHEDULER__CONFIG=/etc/mariadb-backup-s3/scheduler.yaml
SCHEDULER__STATE_FILE=/var/lib/mariadb-backup-s3/state.json
```

Set appropriate permissions:
```bash
sudo chmod 600 /etc/default/mariadb-backup-s3
sudo chown root:root /etc/default/mariadb-backup-s3
```

### 3. Create backup user
Create a dedicated user for backups:

```bash
sudo useradd -r -s /usr/sbin/nologin backup-user
sudo mkdir -p /var/backups/mariadb
sudo chown backup-user:backup-user /var/backups/mariadb
```

### 4. Install service files
Copy the service and timer files to the systemd unit directory:

```bash
sudo install -m 0644 mariadb-backup-s3.service /etc/systemd/system/
sudo install -m 0644 mariadb-backup-s3.timer /etc/systemd/system/
```

### 5. Enable and start the timer
Reload systemd and enable the timer:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now mariadb-backup-s3.timer
```

## Verification

Check timer status:
```bash
systemctl list-timers mariadb-backup-s3.timer
```

View service logs:
```bash
journalctl -u mariadb-backup-s3.service
```

## Customization

To change the backup schedule, edit the timer file:
```ini
[Timer]
OnCalendar=*-*-* 02:30:00
```

Valid time formats:
- `*-*-* 02:30:00` - Daily at 2:30 AM
- `Mon *-*-* 03:00:00` - Every Monday at 3 AM
- `00:00` - Daily at midnight

After changing the timer, reload systemd:
```bash
sudo systemctl daemon-reload
sudo systemctl restart mariadb-backup-s3.timer
```

## Alternative: Built-in Scheduler Daemon

The tool includes a built-in scheduler that runs as a long-lived daemon, which can replace the oneshot+timer pattern entirely:

```bash
# Create scheduler config (see examples/scheduler/) first
# Then run
mariadb-backup-s3 scheduler run --config /etc/mariadb-backup-s3/scheduler.yaml
```

A systemd service for the scheduler daemon is available at `mariadb-backup-s3-scheduler.service`:

```bash
sudo install -m 0644 mariadb-backup-s3-scheduler.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now mariadb-backup-s3-scheduler.service
```

The daemon manages both backups and retention automatically based on the YAML configuration. See [examples/scheduler](../scheduler/) for a complete guide.

## Security Notes
- The service runs with restricted privileges using systemd's security features
- The configuration file should be accessible only to root
