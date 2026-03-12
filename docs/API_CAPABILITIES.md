# API Capabilities

## Overview

`cursor2api-go` is no longer just a plain text OpenAI-compatible proxy.
It now exposes three protocol surfaces on top of Cursor Web and includes multiple compatibility/stability layers borrowed from the JS implementation.

## Endpoint Matrix

| Endpoint | Protocol shape | Stream | Tools / function calling | Identity probe mock | Vision / OCR preprocess | Notes |
|---|---|---:|---:|---:|---:|---|
| `GET /v1/models` | OpenAI models list | N/A | N/A | N/A | N/A | Returns configured Claude-compatible model ids |
| `POST /v1/chat/completions` | OpenAI Chat Completions | ✅ | ✅ | ✅ | ✅ | Supports tool call parsing and OpenAI `tool_calls` streaming deltas |
| `POST /v1/messages` | Anthropic Messages API | ✅ | ✅ | ✅ | ✅ | Emits `content_block_*` / `message_*` SSE events |
| `POST /v1/messages/count_tokens` | Anthropic token counter | N/A | N/A | N/A | N/A | Lightweight estimated token counting |
| `POST /v1/responses` | OpenAI Responses API | ✅ | ✅ | ✅ | ✅ | Internally converted to Chat Completions, then into Anthropic/Cursor pipeline |
| `/chat/completions` / `/messages` / `/responses` | Alias routes | Same as above | Same as above | Same as above | Same as above | Convenience aliases without `/v1` |

## Compatibility Layers

### 1. Protocol compatibility

- OpenAI Chat Completions request/response mapping
- Anthropic Messages request/response mapping
- OpenAI Responses → Chat Completions → Anthropic → Cursor conversion
- Claude-compatible model alias mapping

### 2. Tool compatibility

- Tool definition conversion (`tools`, `input_schema`, OpenAI function schema)
- Tool call parsing from structured ```json action blocks
- JSON-tolerant parser for partially malformed tool payloads
- Embedded code-fence-safe parsing for large file writes / edits
- `tool_choice=any` fallback retry when the model fails to emit a tool call
- Streaming tool call deltas:
  - Anthropic `input_json_delta`
  - OpenAI `tool_calls[].function.arguments` chunks

### 3. Stability / anti-refusal layers

- Refusal detection and auto-retry with reframed prompt
- Historical refusal-context cleanup
- Response sanitization to scrub Cursor identity leakage
- Identity probe interception with mock Claude responses
- Short-response retry for tool mode
- Truncation detection + auto-continue for long tool outputs

### 4. Vision layers

- Local OCR mode via Tesseract + `gosseract`
- External vision API mode via OpenAI-compatible image endpoint
- OpenAI `image_url` payload conversion into Anthropic-style image blocks
- OCR / image understanding result injected back into the textual conversation before forwarding to Cursor Web

## Authentication

Supported auth inputs:

- `Authorization: Bearer <API_KEY>`
- `x-api-key: <API_KEY>`
- `anthropic-api-key: <API_KEY>`

## Test Coverage Status

Current automated coverage includes:

- Parser tests (`compat/*`)
- Config + YAML / env override tests
- Stability tests (refusal, sanitize, identity probe, truncation)
- HTTP-level handler tests for:
  - non-stream identity probe
  - stream identity probe
  - non-stream tools
  - stream tools
  - non-stream vision
  - stream vision
  - Responses API streaming tools

## Practical Notes

### What is solid now

- Protocol surface compatibility
- Tool call parsing and streaming shape conversion
- Identity / refusal handling
- OCR preprocessing path
- Startup/local OCR self-check
- Anthropic thinking block compatibility
- HTTP-level regression coverage

### What still depends on real upstream behavior

- Actual Cursor Web prompt behavior and anti-bot changes
- Real-world refusal pattern drift from Cursor backend updates
- Live upstream reliability / Cloudflare behavior

## Recommended verification commands

### Unit + handler regression

```bash
go test ./...
```

### Live smoke script (starts the real server locally)

```bash
./scripts/e2e_smoke.sh
```

This live smoke script currently focuses on:

- health
- models
- token counting
- identity-probe non-stream
- identity-probe stream

These paths are chosen because they exercise the real running server without requiring successful upstream Cursor generation for every check.
