#!/bin/sh
set -e

for config in "$OPENCLAW_CONFIG" "/root/.picoclaw/config.json"; do
  if [ -n "$config" ] && [ -f "$config" ]; then
    if [ -n "$CLOUDFLARE_ACCOUNT_ID" ]; then
      sed -i "s#__CLOUDFLARE_ACCOUNT_ID__#${CLOUDFLARE_ACCOUNT_ID}#g" "$config"
    fi
    if [ -n "$CLOUDFLARE_API_KEY" ]; then
      sed -i "s#__CLOUDFLARE_API_KEY__#${CLOUDFLARE_API_KEY}#g" "$config"
    fi
    if [ -n "$CLOUDFLARE_API_TOKEN" ]; then
      sed -i "s#__CLOUDFLARE_API_TOKEN__#${CLOUDFLARE_API_TOKEN}#g" "$config"
    fi
    if [ -n "$WORKSPACE_PATH" ]; then
      sed -i "s#__WORKSPACE_PATH__#${WORKSPACE_PATH}#g" "$config"
    fi
  fi
done

case "${1:-}" in
  ""|-*) set -- clutch "$@" ;;
esac

exec "$@"
