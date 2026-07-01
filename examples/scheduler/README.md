# Scheduler Example

This example demonstrates how to use the built-in scheduler daemon for automated MariaDB backups. The scheduler runs cron-scheduled jobs without requiring external cron or systemd timers.

## 📁 Files Structure

```text
examples/scheduler/
├── README.md
└── config.yaml
```

## 📋 Prerequisites

To ensure successful scheduled backups:

- At least 2x the actual database size in free space available
- Valid scheduler configuration (see `config.yaml`)
- Proper file permissions for the state file location

## 🚀 Quick Start

### 1. Copy and validate scheduler config

```bash
# Copy example config to your system location
sudo install -m 600 config.yaml /etc/mariadb-backup-s3/scheduler.yaml

# Edit with your credentials
sudo nano /etc/mariadb-backup-s3/scheduler.yaml

# Validate the configuration
mariadb-backup-s3 scheduler check --config /etc/mariadb-backup-s3/scheduler.yaml
```

### 2. Run the scheduler daemon

```bash
# Start the scheduler in the foreground
mariadb-backup-s3 scheduler run --config /etc/mariadb-backup-s3/scheduler.yaml

# Or run as a background process (examples for common init systems)
```

### 3. Check scheduler status

```bash
# Show job status and execution history
mariadb-backup-s3 scheduler status --state-file /var/lib/mariadb-backup-s3/state.json
```

## 📚 Scheduler Commands

### `scheduler run`

Start the scheduler daemon that executes jobs based on their schedule.

```bash
mariadb-backup-s3 scheduler run --config scheduler.yaml [options]
```

**Options:**

| Option         | Env Var                 | Description                        | Default value |
| -------------- | ----------------------- | ---------------------------------- | ------------- |
| `--config`     | `SCHEDULER__CONFIG`     | Path to scheduler YAML config file | **required**  |
| `--state-file` | `SCHEDULER__STATE_FILE` | Path to persistent state file      | from config   |
| `--once`       |                         | Run all due jobs once then exit    | `false`       |

**Example:**

```bash
# Start the daemon
mariadb-backup-s3 scheduler run --config /etc/mariadb-backup-s3/scheduler.yaml

# Run all due jobs once and exit (useful for testing)
mariadb-backup-s3 scheduler run --config /etc/mariadb-backup-s3/scheduler.yaml --once
```

### `scheduler status`

Show the current status of scheduled jobs and their execution history.

```bash
mariadb-backup-s3 scheduler status --state-file [file] [options]
```

**Options:**

| Option         | Env Var                 | Description                   | Default value                       |
| -------------- | ----------------------- | ----------------------------- | ----------------------------------- |
| `--state-file` | `SCHEDULER__STATE_FILE` | Path to persistent state file | `/tmp/mariadb-scheduler-state.json` |

**Example:**

```bash
# Show status with default state file
mariadb-backup-s3 scheduler status

# Show status with custom state file
mariadb-backup-s3 scheduler status --state-file /var/lib/mariadb-backup-s3/state.json
```

**Output:**

The status command displays:
- **Enabled jobs**: Scheduled jobs that are active
- **Job name**: The job identifier from the configuration
- **Next run**: Scheduled time for the next execution
- **Last run**: When the job was last executed
- **Last status**: Success, failure, or skipped
- **Execution count**: Number of times the job has run

### `scheduler check`

Validate a scheduler YAML configuration file without running the scheduler.

```bash
mariadb-backup-s3 scheduler check --config scheduler.yaml
```

**Options:**

| Option     | Env Var             | Description                        | Default value |
| ---------- | ------------------- | ---------------------------------- | ------------- |
| `--config` | `SCHEDULER__CONFIG` | Path to scheduler YAML config file | **required**  |

**Example:**

```bash
# Validate the configuration
mariadb-backup-s3 scheduler check --config /etc/mariadb-backup-s3/scheduler.yaml
```

## ⚙️ Configuration

A scheduler YAML file defines one or more jobs with their schedules, commands, and configuration. See [config.yaml](./config.yaml) for a complete example.

### Configuration Structure

```yaml
# Optional: global webhook URL for notifications
webhook:
  url: https://hooks.slack.com/services/...
  timeout: 30s

# Required: path to persistent state file
state_file: /var/lib/mariadb-backup-s3/state.json

# Required: list of scheduled jobs
jobs:
  - name: job-name
    schedule: "0 2 * * *"  # cron expression
    command: backup or retention
    timeout: 4h
    storage:
      url: s3://bucket/path
    mariadb:
      host: localhost
      port: 3306
      user: root
      password: ${MARIADB__PASSWORD}
    encrypt:
      key: ${ENCRYPTION__KEY}
    retention:
      max_count: 7
      max_age: 168h
      keep_daily: 7
      keep_weekly: 4
      keep_monthly: 12
```

### Cron Schedule Format

Jobs use standard cron syntax (5 fields):

```text
┌───────────── minute (0 - 59)
│ ┌───────────── hour (0 - 23)
│ │ ┌───────────── day of month (1 - 31)
│ │ │ ┌───────────── month (1 - 12)
│ │ │ │ ┌───────────── day of week (0 - 6, Sunday = 0)
│ │ │ │ │
* * * * *
```

**Examples:**

```yaml
schedule: "0 2 * * *"          # Daily at 2:00 AM
schedule: "0 3 * * 0"          # Every Sunday at 3:00 AM
schedule: "0 */6 * * *"        # Every 6 hours
schedule: "30 8 1 * *"         # First of month at 8:30 AM
```

### Job Commands

| Command     | Description                              |
| ----------- | ---------------------------------------- |
| `backup`    | Perform a backup of the MariaDB database |
| `retention` | Apply retention policies to backups      |

### Storage Configuration

The scheduler reuses your existing storage configuration:

```yaml
storage:
  url: s3://bucket/path?endpoint=https://s3.custom.com
```

See [Storage Types](../README.md#-storage-types) for all supported storage backends.

### Database Configuration

```yaml
mariadb:
  host: localhost
  port: 3306
  user: root
  password: ${MARIADB__PASSWORD}
  backup_binary: mariadb-backup
  backup_options: ""
```

**Options:**

| Option           | Description                                 | Default          |
| ---------------- | ------------------------------------------- | ---------------- |
| `host`           | MariaDB hostname                            | `localhost`      |
| `port`           | MariaDB port                                | `3306`           |
| `user`           | MariaDB username                            | `root`           |
| `password`       | MariaDB password                            | **required**     |
| `backup_binary`  | Path to mariabackup binary                  | `mariadb-backup` |
| `backup_options` | Additional mariabackup command-line options | empty            |

See the [backup command documentation](../README.md#backup) for all options.

### Encryption Configuration

```yaml
encrypt:
  key: ${ENCRYPTION__KEY}
```

See [Encryption](../README.md#-encryption) for details.

### Retention Policy

```yaml
retention:
  max_count: 7           # Keep 7 most recent backups
  max_age: 168h          # Or 7 days old
  keep_daily: 7          # Daily backups to retain
  keep_weekly: 4         # Weekly backups to retain
  keep_monthly: 12       # Monthly backups to retain
```

**Options:**

| Option         | Description                         | Default   |
| -------------- | ----------------------------------- | --------- |
| `max_count`    | Maximum number of backups to keep   | unlimited |
| `max_age`      | Maximum age of backups (e.g., 168h) | unlimited |
| `keep_daily`   | Number of daily backups to keep     | unlimited |
| `keep_weekly`  | Number of weekly backups to keep    | unlimited |
| `keep_monthly` | Number of monthly backups to keep   | unlimited |

See [Retention](../README.md#retention) for details.

> **Note:** At least one retention option must be specified. When multiple options are set, they are evaluated in combination: `max_count` and `max_age` prune eligible backups first, then `keep_daily`, `keep_weekly`, and `keep_monthly` preserve the specified number of recent backups within each period.

## ⏱️ Job Timeouts

Each job has a configurable timeout period. If a job exceeds its timeout, it will be terminated and marked as failed:

- **Backup jobs**: Default timeout is `4 hours`
- **Retention jobs**: Default timeout is `30 minutes`
- **Custom timeout**: Can be set per job using the `timeout` field

> **Warning:** If a job fails (including timeout), it will be retried on the next scheduled execution. Check logs for failed jobs.

### Timeout Configuration Example

```yaml
jobs:
  - name: large-database-backup
    schedule: "0 3 * * *"
    command: backup
    timeout: 6h  # Longer timeout for large databases
    storage:
      url: s3://bucket/path
    mariadb:
      host: localhost
      password: ${MARIADB__PASSWORD}
    retention:
      max_count: 30
```

## 🔧 Job Execution Features

The scheduler includes several safety features:

- **SkipIfStillRunning**: Prevents overlapping job executions. If a job is still running from a previous execution, it won't start again until the previous run completes.
- **Panic Recovery**: Any panic during job execution is caught and logged. Failed jobs can be retried on the next scheduled run.
- **Timeout**: Each job has a configurable timeout. If a job exceeds the timeout, it's terminated and marked as failed.
- **State Persistence**: Job execution state and history is saved to a JSON file, allowing the scheduler to recover from crashes.

## 🎯 Example Scenarios

### Scenario 1: Nightly full backup with retention

```yaml
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
    encrypt:
      key: ${ENCRYPTION__KEY}
    retention:
      max_count: 7
```

### Scenario 2: Weekly retention cleanup

```yaml
jobs:
  - name: weekly-retention
    schedule: "0 3 * * 0"
    command: retention
    timeout: 30m
    storage:
      url: s3://my-bucket/backups?endpoint=https://s3.custom.com
    retention:
      max_age: 720h  # 30 days
```

### Scenario 3: Daily backup with daily/weekly/monthly retention

```yaml
jobs:
  - name: daily-backup
    schedule: "0 */6 * * *"  # Every 6 hours
    command: backup
    timeout: 4h
    storage:
      url: s3://my-bucket/backups?endpoint=https://s3.custom.com
    mariadb:
      host: localhost
      password: ${MARIADB__PASSWORD}
    retention:
      keep_daily: 7
      keep_weekly: 4
      keep_monthly: 12
```

## 🚀 Running as a System Service

### Using systemd

Create a systemd service file for the scheduler:

```bash
sudo install -m 0644 mariadb-backup-s3-scheduler.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now mariadb-backup-s3-scheduler.service
```

See [systemd service example](../systemd-service/) for complete integration.

### Using a process manager

```bash
# Using nohup
nohup mariadb-backup-s3 scheduler run --config /etc/mariadb-backup-s3/scheduler.yaml > /dev/null 2>&1 &

# Using supervisor
[program:mariadb-backup-s3-scheduler]
command=/usr/local/bin/mariadb-backup-s3 scheduler run --config /etc/mariadb-backup-s3/scheduler.yaml
autostart=true
autorestart=true
```

## 📊 Monitoring

### Check scheduler status

```bash
mariadb-backup-s3 scheduler status --state-file /var/lib/mariadb-backup-s3/state.json
```

### Check job logs

```bash
# View scheduler logs
journalctl -u mariadb-backup-s3-scheduler -f

# Check recent backup operations
tail -f /var/log/mariadb-backup.log
```

### Set up alerts

The scheduler can send [CloudEvents v1.0](https://github.com/cloudevents/spec/blob/v1.0/spec.md) formatted notifications when jobs complete. Configure a webhook URL in your scheduler config:

```yaml
webhook:
  url: https://your-webhook-endpoint.com
  timeout: 30s
```

#### CloudEvent Attributes

Each notification includes the following CloudEvents attributes:

| Attribute     | Type      | Description                       | Example                                              |
| ------------- | --------- | --------------------------------- | ---------------------------------------------------- |
| `specversion` | string    | CloudEvents specification version | `1.0`                                                |
| `id`          | string    | Event unique identifier           | `job.nightly-full-backup.succeeded`                  |
| `source`      | string    | Source of the event               | `com.capcom6.mariadb-backup-s3.scheduler`            |
| `type`        | string    | Event type                        | `com.capcom6.mariadb-backup-s3.scheduler.job.backup` |
| `time`        | timestamp | When the event occurred           | `2024-06-23T02:00:00Z`                               |

**Event type values:**
- `com.capcom6.mariadb-backup-s3.scheduler.job.backup` — Backup command completed
- `com.capcom6.mariadb-backup-s3.scheduler.job.retention` — Retention command completed

#### Extension Attributes

Additional metadata attached to each event:

| Extension     | Type    | Description                             |
| ------------- | ------- | --------------------------------------- |
| `job_name`    | string  | Name of the scheduled job               |
| `command`     | string  | Command type (`backup` or `retention`)  |
| `schedule`    | string  | Cron schedule expression                |
| `duration_ms` | integer | Job execution duration in milliseconds  |
| `error`       | string  | Error message (present only on failure) |

#### Data Payload

The event data contains a JSON object with job execution details:

**Success event:**
```json
{
  "job_name": "nightly-full-backup",
  "command": "backup",
  "status": "succeeded",
  "duration": "2h30m0s"
}
```

**Failure event:**
```json
{
  "job_name": "nightly-full-backup",
  "command": "backup",
  "status": "failed",
  "error": "connection timeout",
  "duration": "30s"
}
```

#### Full Example

**Complete CloudEvent payload:**
```json
{
  "specversion": "1.0",
  "id": "job.nightly-full-backup.succeeded",
  "source": "com.capcom6.mariadb-backup-s3.scheduler",
  "type": "com.capcom6.mariadb-backup-s3.scheduler.job.backup",
  "time": "2024-06-23T02:00:00Z",
  "datacontenttype": "application/json",
  "subject": "nightly-full-backup",
  "extensions": {
    "job_name": "nightly-full-backup",
    "command": "backup",
    "schedule": "0 2 * * *",
    "duration_ms": 9000000
  },
  "data": {
    "job_name": "nightly-full-backup",
    "command": "backup",
    "status": "succeeded",
    "duration": "2h30m0s"
  }
}
```

#### Receiver Implementation

Most HTTP webhook receivers understand CloudEvents automatically. Most cloud providers (Slack, Datadog, PagerDuty, etc.) accept CloudEvents-formatted webhooks.

**Example using Node.js/Express:**
```javascript
const express = require('express');
const app = express();

app.use(express.json());

app.post('/', (req, res) => {
  const event = req.body;
  console.log('CloudEvent received:', {
    id: event.id,
    type: event.type,
    source: event.source,
    subject: event.subject,
    time: event.time,
    data: event.data
  });
  res.status(200).send('OK');
});

app.listen(3000);
```

**Example using Python/Flask:**
```python
from flask import Flask, request, jsonify

app = Flask(__name__)

@app.route('/', methods=['POST'])
def webhook():
    event = request.json
    print(f"CloudEvent received: {event}")
    return jsonify({'status': 'received'}), 200

if __name__ == '__main__':
    app.run(port=3000)
```

#### Testing Webhooks

Test webhook delivery with a local receiver:

```bash
# Start a simple webhook receiver
python -m http.server 3000

# Generate a sample CloudEvent and send it
curl -X POST http://localhost:3000 \
  -H "Content-Type: application/cloudevents+json" \
  -d '{
    "specversion": "1.0",
    "id": "test.job.123.succeeded",
    "source": "com.capcom6.mariadb-backup-s3.scheduler",
    "type": "com.capcom6.mariadb-backup-s3.scheduler.job.backup",
    "time": "2024-06-23T02:00:00Z",
    "datacontenttype": "application/json",
    "job_name": "test-job",
    "command": "backup",
    "schedule": "0 2 * * *",
    "duration_ms": 5000,
    "data": {
      "job_name": "test-job",
      "command": "backup",
      "status": "succeeded",
      "duration": "5s"
    }
  }'

# Run scheduler and verify webhook delivery
./mariadb-backup-s3 scheduler run --config scheduler.yaml
```

## 🔍 Troubleshooting

### Check configuration

```bash
mariadb-backup-s3 scheduler check --config /etc/mariadb-backup-s3/scheduler.yaml
```

### Verify scheduler is running

```bash
# Check systemd service
systemctl status mariadb-backup-s3-scheduler

# Check logs
journalctl -u mariadb-backup-s3-scheduler

# Check job status
mariadb-backup-s3 scheduler status --state-file /var/lib/mariadb-backup-s3/state.json
```

### Common issues

**Job not running**: Check cron schedule syntax and ensure the scheduler is running. Verify the state file permissions are correct.

**Backup fails**: Check credentials in environment variables (`MARIADB__PASSWORD`, `ENCRYPTION__KEY`), verify storage connection, check disk space.

**Retention not working**: Ensure retention configuration is valid. The retention command applies policies to ALL backups in storage, not just recent ones.

**State file permissions error**: Ensure the state file location is writable by the user running the scheduler.

**Job timeout**: Check if your job takes longer than the configured timeout. Increase timeout or optimize the backup job.

## 📋 Best Practices

1. **Validate before running**: Always use `scheduler check` to validate your configuration before starting the daemon.
2. **Monitor job status**: Regularly check `scheduler status` to ensure jobs are running successfully.
3. **Test with `--once`**: Test individual jobs with the `--once` flag before enabling them in production.
4. **Set appropriate timeouts**: Each job should have a timeout that allows it to complete under normal conditions.
5. **Use environment variables**: Store sensitive credentials in environment variables and reference them in the config with `${VAR}` syntax.
6. **Review logs**: Regularly review scheduler and backup logs for failures or anomalies.
7. **Back up state file**: The state file contains job execution history. Consider backing it up alongside your backups.
8. **Use retention wisely**: Carefully plan retention policies to balance storage costs with compliance requirements.

## 🔄 Comparison with other methods

| Method                  | Pros                                                                        | Cons                                                    |
| ----------------------- | --------------------------------------------------------------------------- | ------------------------------------------------------- |
| **Built-in Scheduler**  | Single binary, no external cron, persistent state, built-in safety features | Requires YAML configuration, runs as daemon             |
| **Systemd Timer**       | Standard Linux feature, well-tested                                         | Requires separate service and timer files, more complex |
| **Cron + Shell Script** | Simple, familiar                                                            | Scripts can diverge from tool behavior, less robust     |
| **Docker + Cron**       | Containerized, isolated                                                     | Additional complexity, overhead                         |

The built-in scheduler is recommended for new deployments as it provides the best balance of simplicity, reliability, and feature set.