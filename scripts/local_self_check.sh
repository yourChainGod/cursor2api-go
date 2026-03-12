#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_BIN="${GO_BIN:-${HOME}/go/bin/go1.24.0}"
if [[ ! -x "$GO_BIN" ]]; then
  GO_BIN="${GO_BIN:-go}"
fi
VISION_LANGUAGES="${VISION_LANGUAGES:-eng,chi_sim}"

echo "[self-check] repo: $ROOT_DIR"
cd "$ROOT_DIR"

echo "[self-check] checking tesseract binary"
command -v tesseract >/dev/null 2>&1 || { echo "tesseract not found in PATH" >&2; exit 1; }
tesseract --version | head -n 1

echo "[self-check] checking OCR language packs"
langs_output="$(tesseract --list-langs 2>/dev/null || true)"
for lang in ${VISION_LANGUAGES//,/ }; do
  if ! grep -qx "$lang" <<<"$langs_output"; then
    echo "missing tesseract language pack: $lang" >&2
    exit 1
  fi
done

echo "[self-check] running focused local OCR test"
VISION_ENABLED=true VISION_MODE=ocr VISION_LANGUAGES="$VISION_LANGUAGES" "$GO_BIN" test ./compat -run TestProcessWithLocalOCR -v

echo "[self-check] building project"
"$GO_BIN" build ./...

echo "[self-check] PASS"
