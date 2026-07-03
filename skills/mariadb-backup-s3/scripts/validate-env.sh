#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 <backup|restore|retention> [path/to/.env]"
  echo "Validates that required env vars are present for the given command."
  exit 1
}

cmd="${1:-}"
env_file="${2:-.env}"

[ -z "$cmd" ] && usage
if [ -f "$env_file" ]; then
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue
    [[ "$line" =~ ^([[:alnum:]_]+)=(.*)$ ]] && export "${BASH_REMATCH[1]}=${BASH_REMATCH[2]}"
  done < "$env_file"
fi

errors=0

check() {
  local var="$1"
  if [ -z "${!var:-}" ]; then
    echo "MISSING: $var"
    errors=$((errors + 1))
  fi
}

case "$cmd" in
  backup)
    check MARIADB__HOST
    check MARIADB__USER
    check STORAGE__URL
    ;;
  restore)
    check STORAGE__URL
    check RESTORE__TARGET_DIR
    ;;
  retention)
    check STORAGE__URL
    ;;
  *)
    echo "Unknown command: $cmd"
    usage
    ;;
esac

if [ "$errors" -eq 0 ]; then
  echo "OK: all required vars present for '$cmd'"
else
  echo "FAIL: $errors required variable(s) missing for '$cmd'"
  exit 1
fi
