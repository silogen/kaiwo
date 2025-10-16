#!/usr/bin/env bash
set -euo pipefail

# ---- hard-coded bits you gave ----
NS="kgateway-system"
SVC="kserve-ingress-gateway"
SVC_PORT="80"
BASE="/integration/test/v1"  # contains /models and /chat/completions
TIMEOUT=10

# Optional: if your gateway expects Authorization
AUTH_HEADER=()
if [[ -n "${OPENAI_API_KEY:-}" ]]; then
  AUTH_HEADER=( -H "Authorization: Bearer ${OPENAI_API_KEY}" )
fi

need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing: $1" >&2; exit 2; }; }
need kubectl; need curl; need jq

start_proxy() {
  for p in 8001 8002 8003 8004 8005; do
    if ! lsof -iTCP:"$p" -sTCP:LISTEN -P -n >/dev/null 2>&1; then
      kubectl proxy --port="$p" >/dev/null 2>&1 &
      PROXY_PID=$!
      sleep 1
      if kill -0 "$PROXY_PID" 2>/dev/null; then
        PROXY_PORT="$p"
        trap 'kill "$PROXY_PID" 2>/dev/null || true' EXIT
        return 0
      fi
    fi
  done
  echo "ERROR: could not start kubectl proxy (8001-8005 busy?)" >&2
  exit 1
}

echo "Starting kubectl proxy…"
start_proxy
echo "kubectl proxy on 127.0.0.1:${PROXY_PORT}"

MODELS_URL="http://127.0.0.1:${PROXY_PORT}/api/v1/namespaces/${NS}/services/${SVC}:${SVC_PORT}/proxy${BASE}/models"
echo "GET $MODELS_URL"

RESP="$(curl -sS -w '\n%{http_code}' --max-time "$TIMEOUT" "${AUTH_HEADER[@]}" "$MODELS_URL")"
BODY="$(echo "$RESP" | head -n -1)"
CODE="$(echo "$RESP" | tail -n 1)"

echo "HTTP: $CODE"
if [[ "$CODE" != "200" ]]; then
  echo "ERROR: expected 200 from /models, got $CODE"
  echo "$BODY" | head -c 600; echo
  exit 1
fi
echo "$BODY" | jq empty >/dev/null 2>&1 || { echo "ERROR: /models body is not valid JSON"; exit 1; }
echo "✅ /models returned 200 and valid JSON"

MODEL_ID="$(echo "$BODY" | jq -r '.data[0].id // empty')"
[[ -z "$MODEL_ID" || "$MODEL_ID" == "null" ]] && { echo "ERROR: no model id found in /models response"; exit 1; }
echo "Using model: $MODEL_ID"

# ---- OpenAI-compatible Chat Completions request ----
CHAT_URL="http://127.0.0.1:${PROXY_PORT}/api/v1/namespaces/${NS}/services/${SVC}:${SVC_PORT}/proxy${BASE}/chat/completions"
echo "POST $CHAT_URL"

PAYLOAD="$(jq -n --arg model "$MODEL_ID" '
{
  model: $model,
  messages: [
    {role:"user", content:"Hello there!"}
  ],
  max_tokens: 16,
  temperature: 0
}')"

RESP2="$(curl -sS -w '\n%{http_code}' --max-time "$TIMEOUT" \
  -H 'Content-Type: application/json' "${AUTH_HEADER[@]}" \
  -d "$PAYLOAD" "$CHAT_URL")"

BODY2="$(echo "$RESP2" | head -n -1)"
CODE2="$(echo "$RESP2" | tail -n 1)"

echo "HTTP: $CODE2"
if [[ "$CODE2" != "200" ]]; then
  echo "ERROR: expected 200 from /chat/completions, got $CODE2"
  echo "$BODY2" | head -c 800; echo
  exit 1
fi

# Validate general OpenAI chat schema surface
jq -e '
  .object and
  (.choices|type=="array" and length>0) and
  (.choices[0].message.content|type=="string") and
  (.model|type=="string")
' >/dev/null 2>&1 <<<"$BODY2" || {
  echo "ERROR: /chat/completions body does not look like a valid OpenAI response"
  echo "$BODY2" | head -c 800; echo
  exit 1
}

echo "✅ /chat/completions returned 200 and looks valid"
echo
echo "Assistant said:"
echo "$BODY2" | jq -r '.choices[0].message.content'
echo
echo "All checks passed ✅"
