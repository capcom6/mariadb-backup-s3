# Encryption Example

This example demonstrates how to set up encrypted MariaDB backups using the mariadb-backup-s3 tool with AES256-GCM encryption.

## 📁 Files Structure

```
examples/encryption-example/
├── README.md              # This file
├── .env.example           # Environment variables template
└── backup.sh              # Backup script wrapper
```

## 📋 Prerequisites

To ensure successful encrypted backups:

- At least 2x the actual database size in free space available
- MariaDB backup tools installed (`mariadb-backup`)
- Encryption key generated and stored securely

## 🚀 Setup Instructions

### 1. Generate Encryption Key

Generate a secure encryption key using one of these methods:

#### Method A: Using OpenSSL (Recommended)
```bash
# Generate a cryptographically secure 32-byte key
openssl rand -base64 32

# Alternative: Generate key and save directly to environment file
ENCRYPTION_KEY=$(openssl rand -base64 32)
echo "ENCRYPTION__KEY=$ENCRYPTION_KEY" >> .env
```

#### Method B: Using /dev/urandom
```bash
# Generate key from /dev/urandom
head -c 32 /dev/urandom | base64

# Alternative: Generate key and save directly to environment file
ENCRYPTION_KEY=$(head -c 32 /dev/urandom | base64)
echo "ENCRYPTION__KEY=$ENCRYPTION_KEY" >> .env
```

#### Method C: Using Python (for cross-platform compatibility)
```bash
# Generate key using Python
python3 -c "import base64, os; print(base64.b64encode(os.urandom(32)).decode())"

# Alternative: Generate key and save directly to environment file
ENCRYPTION_KEY=$(python3 -c "import base64, os; print(base64.b64encode(os.urandom(32)).decode())")
echo "ENCRYPTION__KEY=$ENCRYPTION_KEY" >> .env
```

#### Key Validation
After generating your key, validate it:

```bash
# Verify key is properly base64 encoded
echo "$ENCRYPTION__KEY" | base64 -d > /dev/null && echo "Key format is valid" || echo "Invalid key format"
```

### 2. Create Environment File

Copy the environment template and customize it:

```bash
cp .env.example .env
nano .env
```

Configure the following variables:
- Database connection settings
- Storage configuration
- **Encryption key** (required for encrypted backups)

### 3. Make Backup Script Executable

```bash
chmod +x backup.sh
```

### 4. Test Backup Manually

Run the backup script to test your configuration:

```bash
./backup.sh
```

### 5. Schedule Automated Backups

Add the backup script to your crontab:

```bash
# Edit crontab
crontab -e

# Add daily backup at 2 AM
0 2 * * * /path/to/encryption-example/backup.sh
```

## 🔐 Encryption Configuration

### Environment Variables

The encryption is configured through environment variables in `.env`:

```bash
# Encryption key (required)
ENCRYPTION__KEY=BASE64_ENCODED_KEY
```

### Key Security

- **Never commit encryption keys to version control**
- Store keys in secure environment variables or secret management systems
- Use cryptographically secure random generation methods
- Consider rotating keys quarterly for production environments
- Store backup keys separately from production keys

## 📧 Logging and Error Handling

The backup script includes comprehensive logging:

- Main log: `/var/log/mariadb-backup-encryption.log`
- Error log: `/var/log/mariadb-backup-encryption-error.log`

To enable email notifications on failure, set `NOTIFY_EMAIL` in your environment file:

```bash
NOTIFY_EMAIL="admin@example.com"
```

## 🔧 How Encryption Integrates with Pipeline

The encryption process works as follows:

1. **Backup Creation**: MariaDB backup tools create an unencrypted backup
2. **Encryption**: The backup is encrypted using AES256-GCM with your provided key
3. **Upload**: Encrypted backup is uploaded to S3 storage
4. **Cleanup**: Old backups are automatically removed based on retention settings

### Security Features

- **AES256-GCM**: Industry-standard encryption with authenticated encryption
- **Key Management**: Full control over encryption keys (no key escrow)
- **Secure Storage**: Encrypted backups are stored securely in S3
- **Backup Integrity**: GCM mode provides both confidentiality and integrity verification

## 🔍 Troubleshooting

### Common Issues

#### Encryption Key Problems

**Invalid Key Format**
```bash
# Test if key is valid base64
echo "$ENCRYPTION__KEY" | base64 -d > /dev/null && echo "Key format is valid" || echo "Invalid key format"

# Regenerate a new key if needed
openssl rand -base64 32 > new-key.txt
echo "New key generated: $(cat new-key.txt)"
```

**Missing Key**
```bash
# Check if key is set in environment
if [ -z "${ENCRYPTION__KEY:-}" ]; then
    echo "Error: ENCRYPTION__KEY is not set in environment file"
    echo "Please add a valid encryption key to your .env file"
fi
```

#### Backup Failures

**Check Logs**
```bash
# View main backup log
tail -f /var/log/mariadb-backup-encryption.log

# View error log
tail -f /var/log/mariadb-backup-encryption-error.log

# Check log permissions
ls -la /var/log/mariadb-backup-encryption*
```

**Test Backup Manually**
```bash
# Run backup with verbose output
./backup.sh 2>&1 | tee /tmp/backup-test.log

# Check exit code
echo "Exit code: $?"

# View test log
cat /tmp/backup-test.log
```

**Environment Variables Check**
```bash
# Test if environment file is loaded correctly
source .env && echo "ENCRYPTION__KEY: ${ENCRYPTION__KEY:0:8}..."

# Check if all required variables are set
source .env && env | grep -E '^(MARIADB__|STORAGE__|ENCRYPTION__|BACKUP__)'
```

#### Encryption-Specific Issues

**Encryption Performance Problems**
```bash
# Check system resources during encryption
top -b -n 1 | head -20

# Check available disk space
df -h

# Monitor memory usage
free -h
```

### Verify Encrypted Backups

**Basic Verification**
```bash
# Attempt to view (should show garbled text)
head -c 100 /tmp/encrypted-backup.xz.enc

# Check file size and permissions
ls -la /tmp/encrypted-backup.xz.enc
```

### Advanced Troubleshooting

**Debug Mode**
```bash
# Enable debug logging in the backup script
DEBUG=1 ./backup.sh

# Check for debug information in logs
grep "DEBUG" /var/log/mariadb-backup-encryption.log
```

**System Resource Check**
```bash
# Check available disk space for temporary files
df -h /tmp

# Check available memory
free -h

# Check CPU usage
top -b -n 1 | grep "Cpu(s)"
```

**Network Connectivity**
```bash
# Test S3 connectivity
aws s3 ls s3://your-bucket

# Test network connectivity
ping your-bucket.s3.amazonaws.com
```

### Common Error Messages and Solutions

**"Encryption key not configured"**
- **Cause**: ENCRYPTION__KEY is not set in environment file
- **Solution**: Add a valid base64-encoded key to your .env file

**"Invalid encryption key format"**
- **Cause**: Key is not properly base64-encoded
- **Solution**: Regenerate key using `openssl rand -base64 32`

**"Backup binary not found"**
- **Cause**: mariadb-backup-s3 binary is not installed or not in PATH
- **Solution**: Install the binary or update BACKUP_BINARY path in script

**"Permission denied" for log files**
- **Cause**: Script doesn't have write permission to log directory
- **Solution**: Fix permissions: `sudo mkdir -p /var/log && sudo chmod 755 /var/log`

**"S3 upload failed"**
- **Cause**: Network issues, invalid credentials, or bucket permissions
- **Solution**: Test S3 connectivity with `aws s3 ls` and verify credentials

## 📋 Best Practices

### Key Management
1. **Separate Storage**: Store encryption keys separately from backup configuration files
2. **Access Control**: Restrict access to encryption keys using file permissions (chmod 600)
3. **Secure Generation**: Always use cryptographically secure random number generators
4. **Key Rotation**: Rotate encryption keys every 3-6 months for production environments
5. **Backup Keys**: Store backup copies of encryption keys in secure, offline locations
6. **Key Validation**: Regularly validate encryption keys before backup runs

### Storage Security
1. **Secure Storage**: Use secure key management systems (AWS KMS, HashiCorp Vault, Azure Key Vault)
2. **Environment Variables**: Use secure environment variables instead of plaintext files when possible
3. **Secret Management**: Consider using secret management tools like Docker secrets, Kubernetes secrets, or HashiCorp Vault
4. **File Permissions**: Set restrictive permissions on environment files (chmod 600)

### Backup Security
1. **Testing**: Regularly test backup restoration with encrypted backups
2. **Verification**: Verify backup integrity both before and after encryption
3. **Access Control**: Restrict access to backup files using appropriate IAM policies or file permissions
4. **Audit Logging**: Enable audit logging for access to encrypted backups and encryption keys

### Monitoring and Alerting
1. **Backup Failures**: Set up alerts for backup failures and encryption issues
2. **Key Usage**: Monitor encryption key usage and detect unusual patterns
3. **Storage Monitoring**: Monitor storage capacity and encryption performance
4. **Regular Audits**: Conduct regular security audits of backup and encryption processes

### Documentation
1. **Key Rotation**: Document key rotation procedures and recovery processes
2. **Emergency Procedures**: Document emergency procedures for key loss or corruption
3. **Recovery Testing**: Document backup restoration testing procedures and results
4. **Security Policies**: Document security policies and compliance requirements

## 🔄 Backup Rotation

The mariadb-backup-s3 tool automatically handles backup rotation based on the `RETENTION__COUNT` setting. Older encrypted backups are automatically removed when this limit is exceeded.

## 📊 Monitoring

Consider setting up monitoring for encrypted backups:

```bash
# Check if backup ran successfully in the last 24 hours
if [ -f "/var/log/mariadb-backup-encryption.log" ] && [ $(find /var/log/mariadb-backup-encryption.log -mtime -1 | wc -l) -eq 0 ]; then
    echo "Encrypted backup may have failed" | mail -s "Backup Alert" admin@example.com
fi
```

## 🔒 Security Notes

- Encrypted backups are only as secure as your encryption key
- Implement proper access controls for backup files
- Consider using AWS KMS or similar services for key management
- Regularly audit backup access and encryption key usage
- Test backup restoration procedures regularly
