#!/bin/bash
# Seed script — recreates initial templates after a DB reset.
# Usage: ./seed.sh [base_url] [admin_token]
#
# Defaults assume local dev (.env values).

BASE_URL="${1:-http://localhost:8080}"
ADMIN_TOKEN="${2:-agent-manager-local-dev}"

api() {
  local method="$1" path="$2" body="$3"
  local resp
  resp=$(curl -s -w "\n%{http_code}" -X "$method" "${BASE_URL}${path}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    ${body:+-d "$body"})
  local code=$(echo "$resp" | tail -1)
  local body_out=$(echo "$resp" | sed '$d')
  if [ "$code" -ge 400 ] 2>/dev/null; then
    echo "FAILED (HTTP $code): $body_out"
    return 1
  fi
  echo "$body_out"
}

# Wait for server to be ready
echo "Waiting for ${BASE_URL} ..."
for i in $(seq 1 15); do
  if curl -s "${BASE_URL}/health" > /dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo "Seeding ${BASE_URL} ..."

# --- Templates ---

echo -n "  ratchet template... "
result=$(api POST /agent-templates '{
  "name": "ratchet",
  "description": "SRE monitoring agent",
  "labels": {"env": "prod"},
  "allowlist": ["*.googleapis.com"],
  "maxRunners": 1
}')
if [ $? -eq 0 ]; then
  echo "done"
else
  echo "$result"
fi

# Add more templates here as needed:
# echo -n "  another-template... "
# api POST /agent-templates '{ ... }'
# echo "done"

echo ""
echo "Template tokens:"
api GET /agent-templates | python3 -c "
import sys, json
for t in json.load(sys.stdin):
    print(f\"  {t['name']}: {t['token']}\")
" 2>/dev/null || echo "  (could not list templates)"
