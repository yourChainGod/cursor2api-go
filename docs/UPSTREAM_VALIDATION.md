# Upstream Validation

This document explains the difference between the two runtime verification scripts shipped with `cursor2api-go`.

## 1. `scripts/e2e_smoke.sh`

Purpose: verify that the local server starts correctly and that the most stable, deterministic compatibility paths still work.

What it checks:

- `/health`
- `/v1/models`
- `/v1/messages/count_tokens`
- identity-probe non-stream paths
- identity-probe stream paths

Why it is strict:

These checks do **not** depend on upstream Cursor Web successfully following a complex prompt. They are meant to be reliable CI / operator sanity checks.

## 2. `scripts/e2e_upstream_matrix.sh`

Purpose: verify the **real upstream behavior** of the running service against Cursor Web.

What it checks:

- plain text non-stream requests
- plain text stream requests
- tools non-stream requests
- tools stream requests
- Responses API stream tool calls
- optional vision request path (only when `VISION_ENABLED=true`)

## Result Semantics

The upstream matrix intentionally distinguishes between three outcomes:

- `PASS` — request succeeded and the expected behavior was observed
- `WARN` — request succeeded, but upstream behavior was weaker / different than expected
- `FAIL` — server startup, HTTP transport, or protocol framing failed

This distinction matters because upstream Cursor behavior can drift even when the local proxy implementation is correct.

## Usage

### Quick mode

```bash
MODE=quick ./scripts/e2e_upstream_matrix.sh
```

Quick mode runs a smaller set of plain-text probes.

### Full mode

```bash
./scripts/e2e_upstream_matrix.sh
```

### Via Makefile

```bash
make smoke
make upstream-check
```

## Optional vision validation

If you want the upstream matrix to hit the vision path too, enable vision before running it:

```bash
VISION_ENABLED=true \
VISION_MODE=ocr \
VISION_LANGUAGES=eng,chi_sim \
make upstream-check
```

or:

```bash
VISION_ENABLED=true \
VISION_MODE=api \
VISION_BASE_URL=https://api.openai.com/v1/chat/completions \
VISION_API_KEY=your-key \
VISION_MODEL=gpt-4o-mini \
make upstream-check
```

## Recommended operator workflow

1. `make test`
2. `make smoke`
3. `make upstream-check`

That order gives you:

- unit / handler regression confidence
- local runtime confidence
- real upstream confidence
