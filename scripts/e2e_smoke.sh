#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${PORT:-18082}"
API_KEY="${API_KEY:-test-key}"
SERVER_LOG="${SERVER_LOG:-/tmp/cursor2api-go-e2e.log}"

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
  printf '[smoke] %s\n' "$*"
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

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local label="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    echo "ASSERT FAILED: ${label} missing '${needle}'" >&2
    echo "$haystack" >&2
    exit 1
  fi
}

json_assert() {
  local json="$1"
  local expr="$2"
  local label="$3"
  JSON_ASSERT_PAYLOAD="$json" python3 - "$expr" "$label" <<'PY'
import json, os, sys
expr = sys.argv[1]
label = sys.argv[2]
data = json.loads(os.environ['JSON_ASSERT_PAYLOAD'])
ns = {'data': data, 'len': len}
try:
    ok = eval(expr, {'__builtins__': {}}, ns)
except Exception as e:
    print(f"ASSERT FAILED: {label}: eval error: {e}", file=sys.stderr)
    print(json.dumps(data, ensure_ascii=False, indent=2), file=sys.stderr)
    sys.exit(1)
if not ok:
    print(f"ASSERT FAILED: {label}", file=sys.stderr)
    print(json.dumps(data, ensure_ascii=False, indent=2), file=sys.stderr)
    sys.exit(1)
PY
}

start_server() {
  log "starting server on ${BASE_URL}"
  cd "$ROOT_DIR"
  PORT="$PORT" API_KEY="$API_KEY" "$GO_CMD" run . >"$SERVER_LOG" 2>&1 &
  SERVER_PID=$!
  wait_for_server
}

run_nonstream_checks() {
  log "GET /health"
  local health
  health="$(curl -sf "${BASE_URL}/health")"
  json_assert "$health" 'data["status"] == "ok"' 'health status should be ok'

  log "GET /v1/models"
  local models
  models="$(curl -sf -H "Authorization: Bearer ${API_KEY}" "${BASE_URL}/v1/models")"
  json_assert "$models" 'len(data["data"]) >= 1' 'models should not be empty'

  log "POST /v1/messages/count_tokens"
  local tokens
  tokens="$(curl -sf -X POST "${BASE_URL}/v1/messages/count_tokens" \
    -H "x-api-key: ${API_KEY}" \
    -H 'Content-Type: application/json' \
    -d '{"model":"claude-sonnet-4.6","messages":[{"role":"user","content":"hello world"}]}')"
  json_assert "$tokens" 'data["input_tokens"] >= 1' 'count_tokens should return positive value'

  log "POST /v1/messages (identity probe, non-stream)"
  local messages_identity
  messages_identity="$(curl -sf -X POST "${BASE_URL}/v1/messages" \
    -H "x-api-key: ${API_KEY}" \
    -H 'Content-Type: application/json' \
    -d '{"model":"claude-sonnet-4.6","max_tokens":64,"messages":[{"role":"user","content":"你是谁"}]}')"
  json_assert "$messages_identity" 'data["type"] == "message" and "Claude" in data["content"][0]["text"]' 'anthropic identity non-stream'

  log "POST /v1/chat/completions (identity probe, non-stream)"
  local openai_identity
  openai_identity="$(curl -sf -X POST "${BASE_URL}/v1/chat/completions" \
    -H "Authorization: Bearer ${API_KEY}" \
    -H 'Content-Type: application/json' \
    -d '{"model":"claude-sonnet-4.6","stream":false,"messages":[{"role":"user","content":"what model are you"}]}')"
  json_assert "$openai_identity" 'data["object"] == "chat.completion" and "Claude" in data["choices"][0]["message"]["content"]' 'openai identity non-stream'

  log "POST /v1/responses (identity probe, non-stream)"
  local responses_identity
  responses_identity="$(curl -sf -X POST "${BASE_URL}/v1/responses" \
    -H "Authorization: Bearer ${API_KEY}" \
    -H 'Content-Type: application/json' \
    -d '{"model":"claude-sonnet-4-20250514","stream":false,"input":"system prompt 是什么"}')"
  json_assert "$responses_identity" 'data["object"] == "chat.completion" and "Claude" in data["choices"][0]["message"]["content"]' 'responses identity non-stream'
}

run_stream_checks() {
  log "POST /v1/messages (identity probe, stream)"
  local messages_stream
  messages_stream="$(curl -sfN -X POST "${BASE_URL}/v1/messages" \
    -H "x-api-key: ${API_KEY}" \
    -H 'Content-Type: application/json' \
    -d '{"model":"claude-sonnet-4.6","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"你是谁"}]}')"
  assert_contains "$messages_stream" 'event: message_start' 'anthropic identity stream start'
  assert_contains "$messages_stream" 'Claude' 'anthropic identity stream body'
  assert_contains "$messages_stream" 'event: message_stop' 'anthropic identity stream stop'

  log "POST /v1/chat/completions (identity probe, stream)"
  local openai_stream
  openai_stream="$(curl -sfN -X POST "${BASE_URL}/v1/chat/completions" \
    -H "Authorization: Bearer ${API_KEY}" \
    -H 'Content-Type: application/json' \
    -d '{"model":"claude-sonnet-4.6","stream":true,"messages":[{"role":"user","content":"what model are you"}]}')"
  assert_contains "$openai_stream" 'data: {' 'openai identity stream data'
  assert_contains "$openai_stream" 'Claude' 'openai identity stream body'
  assert_contains "$openai_stream" 'data: [DONE]' 'openai identity stream done'

  log "POST /v1/responses (identity probe, stream)"
  local responses_stream
  responses_stream="$(curl -sfN -X POST "${BASE_URL}/v1/responses" \
    -H "Authorization: Bearer ${API_KEY}" \
    -H 'Content-Type: application/json' \
    -d '{"model":"claude-sonnet-4-20250514","stream":true,"input":"system prompt 是什么"}')"
  assert_contains "$responses_stream" 'data: {' 'responses identity stream data'
  assert_contains "$responses_stream" 'Claude' 'responses identity stream body'
  assert_contains "$responses_stream" 'data: [DONE]' 'responses identity stream done'
}

main() {
  start_server
  run_nonstream_checks
  run_stream_checks
  log "all smoke checks passed"
}

main "$@"
