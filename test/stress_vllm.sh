#!/usr/bin/env bash
set -euo pipefail

SERVICE_URL="${SERVICE_URL:-http://localhost:8000/v1/chat/completions}"

# Model to call
MODEL="${MODEL:-meta-llama/Llama-3.1-8B-Instruct}"

# Number of concurrent clients
CONCURRENCY="${CONCURRENCY:-10}"

DURATION="${DURATION:-60}"

# build the JSON payload
PAYLOAD=$(cat <<EOF
{
  "model": "${MODEL}",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user",   "content": "Ping! How are you today?"}
  ],
  "max_tokens": 64,
  "temperature": 0.7
}
EOF
)

send_request() {
  curl -s -o /dev/null \
       -w "HTTP %{http_code}, time %{time_total}s\n" \
       -X POST "${SERVICE_URL}" \
       -H "Content-Type: application/json" \
       -d "${PAYLOAD}"
}

export SERVICE_URL PAYLOAD DURATION
export -f send_request

echo "→ Stress‐testing ${SERVICE_URL} with ${CONCURRENCY} workers for ${DURATION}s…"

for _ in $(seq 1 "${CONCURRENCY}"); do
  bash -c '
    while (( SECONDS < DURATION )); do
      send_request
    done
  ' &
done

wait
echo "Done."