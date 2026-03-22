#!/bin/bash
# Seed script — recreates initial profiles after a DB reset.
# Usage: ./seed.sh [base_url] [username] [password]
#
# Defaults assume local dev.

BASE_URL="${1:-http://localhost:8080}"
USERNAME="${2:-admin}"
PASSWORD="${3:-admin}"

# Login to get JWT token
echo "Logging in as ${USERNAME}..."
login_resp=$(curl -s -X POST "${BASE_URL}/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"${USERNAME}\",\"password\":\"${PASSWORD}\"}")
TOKEN=$(echo "$login_resp" | python3 -c "import sys,json; print(json.load(sys.stdin).get('token',''))" 2>/dev/null)
if [ -z "$TOKEN" ]; then
  echo "Login failed: $login_resp"
  exit 1
fi
echo "Logged in."

api() {
  local method="$1" path="$2" body="$3"
  local resp
  resp=$(curl -s -w "\n%{http_code}" -X "$method" "${BASE_URL}${path}" \
    -H "Authorization: Bearer ${TOKEN}" \
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

# --- Agent Profiles ---

echo -n "  marketing-agent profile... "
result=$(api POST /agent-profiles '{
  "name": "marketing-agent",
  "description": "Marketing agent profile",
  "maxCount": 1,
  "ttlMinutes": -1
}')
if [ $? -eq 0 ]; then
  echo "done"
else
  echo "$result"
fi

# Add more profiles here as needed:
# echo -n "  another-profile... "
# api POST /agent-profiles '{ ... }'
# echo "done"

echo ""
echo "Profiles:"
api GET /agent-profiles | python3 -c "
import sys, json
for p in json.load(sys.stdin):
    print(f\"  {p['name']}: {p.get('description','')}\")
" 2>/dev/null || echo "  (could not list profiles)"
