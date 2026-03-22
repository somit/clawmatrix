#!/bin/sh
set -e

# Bootstrap admin user on first run (idempotent — silently skips if already exists)
if [ -n "$ADMIN_USERNAME" ] && [ -n "$ADMIN_PASSWORD" ]; then
  control-plane createadmin --username "$ADMIN_USERNAME" --password "$ADMIN_PASSWORD" 2>/dev/null || true
fi

exec control-plane "$@"
