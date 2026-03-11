#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${PORT:-18083}"
API_KEY="${API_KEY:-test-key}"
SERVER_LOG="${SERVER_LOG:-/tmp/cursor2api-go-upstream.log}"
CURL_MAX_TIME="${CURL_MAX_TIME:-180}"
MODE="${MODE:-full}"   # quick|full

PASS_COUNT=0
WARN_COUNT=0
FAIL_COUNT=0

cleanup() {
  if [[ -n "${SERVER_PID:-}" ]] && kill -0 "$SERVER_PID" >/dev/null 2>&1; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

pick_go() {
  if [[ -n "${GO_BIN:-}" ]]; then
    echo "$GO_BIN"
    return
  fi
  if command -v go1.24.0 >/dev/null 2>&1; then
    echo "go1.24.0"
    return
  fi
  if [[ -x "$HOME/go/bin/go1.24.0" ]]; then
    echo "$HOME/go/bin/go1.24.0"
    return
  fi
  echo "go"
}

GO_CMD="$(pick_go)"
BASE_URL="http://127.0.0.1:${PORT}"

log() {
  printf '[upstream] %s\n' "$*"
}

pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  printf 'PASS  %-28s %s\n' "$1" "$2"
}

warn() {
  WARN_COUNT=$((WARN_COUNT + 1))
  printf 'WARN  %-28s %s\n' "$1" "$2"
}

fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf 'FAIL  %-28s %s\n' "$1" "$2"
}

wait_for_server() {
  for _ in $(seq 1 60); do
    if curl -sf "${BASE_URL}/health" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "server failed to start; log follows:" >&2
  cat "$SERVER_LOG" >&2 || true
  return 1
}

start_server() {
  log "starting server on ${BASE_URL}"
  cd "$ROOT_DIR"
  PORT="$PORT" API_KEY="$API_KEY" "$GO_CMD" run . >"$SERVER_LOG" 2>&1 &
  SERVER_PID=$!
  wait_for_server
}

perform_request() {
  local method="$1"; shift
  local url="$1"; shift
  local body="${1:-}"
  shift || true
  local response_file
  response_file="$(mktemp)"
  local status
  if [[ -n "$body" ]]; then
    status="$(curl -sS -m "$CURL_MAX_TIME" -o "$response_file" -w '%{http_code}' -X "$method" "$url" "$@" --data "$body")"
  else
    status="$(curl -sS -m "$CURL_MAX_TIME" -o "$response_file" -w '%{http_code}' -X "$method" "$url" "$@")"
  fi
  local response
  response="$(cat "$response_file")"
  rm -f "$response_file"
  printf '%s\n%s' "$status" "$response"
}

extract_body() {
  printf '%s' "$1" | tail -n +2
}

extract_status() {
  printf '%s' "$1" | head -n1
}

json_has_text() {
  JSON_PAYLOAD="$1" python3 - <<'PY'
import json, os, sys
try:
    data = json.loads(os.environ['JSON_PAYLOAD'])
except Exception:
    sys.exit(2)
blob = json.dumps(data, ensure_ascii=False)
sys.exit(0 if blob.strip() else 1)
PY
}

mk_png_data_url() {
  python3 - <<'PY'
from PIL import Image, ImageDraw
import base64, io
img = Image.new('RGB', (260, 90), 'white')
d = ImageDraw.Draw(img)
d.text((10, 25), 'UPSTREAM VISION 42', fill='black')
buf = io.BytesIO()
img.save(buf, format='PNG')
print('data:image/png;base64,' + base64.b64encode(buf.getvalue()).decode())
PY
}

check_infra_basics() {
  local health_resp status body
  health_resp="$(perform_request GET "${BASE_URL}/health" "")"
  status="$(extract_status "$health_resp")"
  body="$(extract_body "$health_resp")"
  if [[ "$status" == "200" && "$body" == *'"status":"ok"'* ]]; then
    pass health "service started"
  else
    fail health "unexpected response: ${status} ${body}"
  fi

  local models_resp
  models_resp="$(perform_request GET "${BASE_URL}/v1/models" "" -H "Authorization: Bearer ${API_KEY}")"
  status="$(extract_status "$models_resp")"
  body="$(extract_body "$models_resp")"
  if [[ "$status" == "200" && "$body" == *'claude-sonnet'* ]]; then
    pass models "model list reachable"
  else
    fail models "unexpected response: ${status} ${body}"
  fi
}

run_messages_plain_nonstream() {
  local name="messages-plain-nonstream"
  local payload='{"model":"claude-sonnet-4.6","max_tokens":128,"messages":[{"role":"user","content":"Reply with exactly OK_UPSTREAM_PLAIN and nothing else."}]}'
  local resp status body
  resp="$(perform_request POST "${BASE_URL}/v1/messages" "$payload" -H "x-api-key: ${API_KEY}" -H 'Content-Type: application/json')"
  status="$(extract_status "$resp")"
  body="$(extract_body "$resp")"
  if [[ "$status" != "200" ]]; then
    fail "$name" "HTTP ${status}"
    return
  fi
  if ! json_has_text "$body"; then
    fail "$name" "body is not valid json"
    return
  fi
  if [[ "$body" == *'OK_UPSTREAM_PLAIN'* ]]; then
    pass "$name" "exact marker returned"
  else
    warn "$name" "request succeeded but exact marker not found"
  fi
}

run_messages_plain_stream() {
  local name="messages-plain-stream"
  local payload='{"model":"claude-sonnet-4.6","max_tokens":128,"stream":true,"messages":[{"role":"user","content":"Reply with exactly OK_UPSTREAM_STREAM and nothing else."}]}'
  local resp status body
  resp="$(perform_request POST "${BASE_URL}/v1/messages" "$payload" -H "x-api-key: ${API_KEY}" -H 'Content-Type: application/json')"
  status="$(extract_status "$resp")"
  body="$(extract_body "$resp")"
  if [[ "$status" != "200" ]]; then
    fail "$name" "HTTP ${status}"
    return
  fi
  if [[ "$body" != *'event: message_start'* || "$body" != *'event: message_stop'* ]]; then
    fail "$name" "missing anthropic SSE framing"
    return
  fi
  if [[ "$body" == *'OK_UPSTREAM_STREAM'* ]]; then
    pass "$name" "stream marker observed"
  else
    warn "$name" "SSE worked but exact marker not found"
  fi
}

run_chat_plain_nonstream() {
  local name="chat-plain-nonstream"
  local payload='{"model":"claude-sonnet-4.6","stream":false,"messages":[{"role":"user","content":"Reply with exactly OK_UPSTREAM_CHAT and nothing else."}]}'
  local resp status body
  resp="$(perform_request POST "${BASE_URL}/v1/chat/completions" "$payload" -H "Authorization: Bearer ${API_KEY}" -H 'Content-Type: application/json')"
  status="$(extract_status "$resp")"
  body="$(extract_body "$resp")"
  if [[ "$status" != "200" ]]; then
    fail "$name" "HTTP ${status}"
    return
  fi
  if ! json_has_text "$body"; then
    fail "$name" "body is not valid json"
    return
  fi
  if [[ "$body" == *'OK_UPSTREAM_CHAT'* ]]; then
    pass "$name" "exact marker returned"
  else
    warn "$name" "request succeeded but exact marker not found"
  fi
}

run_chat_plain_stream() {
  local name="chat-plain-stream"
  local payload='{"model":"claude-sonnet-4.6","stream":true,"messages":[{"role":"user","content":"Reply with exactly OK_UPSTREAM_CHAT_STREAM and nothing else."}]}'
  local resp status body
  resp="$(perform_request POST "${BASE_URL}/v1/chat/completions" "$payload" -H "Authorization: Bearer ${API_KEY}" -H 'Content-Type: application/json')"
  status="$(extract_status "$resp")"
  body="$(extract_body "$resp")"
  if [[ "$status" != "200" ]]; then
    fail "$name" "HTTP ${status}"
    return
  fi
  if [[ "$body" != *'data: {'* || "$body" != *'data: [DONE]'* ]]; then
    fail "$name" "missing openai SSE framing"
    return
  fi
  if [[ "$body" == *'OK_UPSTREAM_CHAT_STREAM'* ]]; then
    pass "$name" "stream marker observed"
  else
    warn "$name" "SSE worked but exact marker not found"
  fi
}

run_messages_tools_nonstream() {
  local name="messages-tools-nonstream"
  local payload='{"model":"claude-sonnet-4.6","max_tokens":256,"messages":[{"role":"user","content":"Use the RespondWith action and set text to OK_TOOL_MESSAGE."}],"tools":[{"name":"RespondWith","description":"Return a marker string","input_schema":{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}}]}'
  local resp status body
  resp="$(perform_request POST "${BASE_URL}/v1/messages" "$payload" -H "x-api-key: ${API_KEY}" -H 'Content-Type: application/json')"
  status="$(extract_status "$resp")"
  body="$(extract_body "$resp")"
  if [[ "$status" != "200" ]]; then
    fail "$name" "HTTP ${status}"
    return
  fi
  if [[ "$body" == *'"type":"tool_use"'* ]]; then
    pass "$name" "tool_use returned"
  else
    warn "$name" "request succeeded but no tool_use detected"
  fi
}

run_messages_tools_stream() {
  local name="messages-tools-stream"
  local payload='{"model":"claude-sonnet-4.6","max_tokens":256,"stream":true,"messages":[{"role":"user","content":"Use the RespondWith action and set text to OK_TOOL_MESSAGE_STREAM."}],"tools":[{"name":"RespondWith","description":"Return a marker string","input_schema":{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}}]}'
  local resp status body
  resp="$(perform_request POST "${BASE_URL}/v1/messages" "$payload" -H "x-api-key: ${API_KEY}" -H 'Content-Type: application/json')"
  status="$(extract_status "$resp")"
  body="$(extract_body "$resp")"
  if [[ "$status" != "200" ]]; then
    fail "$name" "HTTP ${status}"
    return
  fi
  if [[ "$body" == *'"type":"tool_use"'* && "$body" == *'"partial_json"'* ]]; then
    pass "$name" "stream tool_use detected"
  else
    warn "$name" "request succeeded but no stream tool_use detected"
  fi
}

run_responses_tools_stream() {
  local name="responses-tools-stream"
  local payload='{"model":"claude-sonnet-4-20250514","stream":true,"input":"Use the RespondWith action and set text to OK_RESPONSES_TOOL.","tools":[{"type":"function","name":"RespondWith","description":"Return a marker string","parameters":{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}}]}'
  local resp status body
  resp="$(perform_request POST "${BASE_URL}/v1/responses" "$payload" -H "Authorization: Bearer ${API_KEY}" -H 'Content-Type: application/json')"
  status="$(extract_status "$resp")"
  body="$(extract_body "$resp")"
  if [[ "$status" != "200" ]]; then
    fail "$name" "HTTP ${status}"
    return
  fi
  if [[ "$body" == *'"tool_calls"'* && "$body" == *'data: [DONE]'* ]]; then
    pass "$name" "responses stream tool_calls detected"
  else
    warn "$name" "request succeeded but no responses stream tool_calls detected"
  fi
}

run_optional_vision_probe() {
  if [[ "${VISION_ENABLED:-false}" != "true" ]]; then
    warn vision-probe "skipped (VISION_ENABLED!=true)"
    return
  fi
  local name="vision-probe"
  local data_url payload resp status body
  data_url="$(mk_png_data_url)"
  payload="$(python3 - <<'PY'
import json, os
print(json.dumps({
  'model': 'claude-sonnet-4.6',
  'max_tokens': 256,
  'messages': [{
    'role': 'user',
    'content': [
      {'type': 'text', 'text': 'Describe the attached screenshot briefly.'},
      {'type': 'image', 'source': {'type': 'base64', 'media_type': 'image/png', 'data': os.environ['DATA_URL'].split(',',1)[1]}}
    ]
  }]
}))
PY
)"
  resp="$(DATA_URL="$data_url" perform_request POST "${BASE_URL}/v1/messages" "$payload" -H "x-api-key: ${API_KEY}" -H 'Content-Type: application/json')"
  status="$(extract_status "$resp")"
  body="$(extract_body "$resp")"
  if [[ "$status" != "200" ]]; then
    fail "$name" "HTTP ${status}"
    return
  fi
  if ! json_has_text "$body"; then
    fail "$name" "body is not valid json"
    return
  fi
  pass "$name" "vision-enabled request path returned 200"
}

summary() {
  echo
  printf 'Summary: PASS=%d WARN=%d FAIL=%d\n' "$PASS_COUNT" "$WARN_COUNT" "$FAIL_COUNT"
  if (( FAIL_COUNT > 0 )); then
    echo "Server log: ${SERVER_LOG}" >&2
    return 1
  fi
  return 0
}

main() {
  start_server
  check_infra_basics
  run_messages_plain_nonstream
  run_messages_plain_stream
  run_chat_plain_nonstream
  if [[ "$MODE" == "full" ]]; then
    run_chat_plain_stream
    run_messages_tools_nonstream
    run_messages_tools_stream
    run_responses_tools_stream
    run_optional_vision_probe
  fi
  summary
}

main "$@"
