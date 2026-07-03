#!/usr/bin/env bash
set -euo pipefail

# Generates a base64-encoded 32-byte AES-256 key for mariadb-backup-s3
key=$(openssl rand -base64 32)
echo "$key"
