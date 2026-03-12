#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OUT_DIR="$SCRIPT_DIR"
NPM_DIR="$PROJECT_ROOT/npm"

echo "Building WASM from Go protocol package..."

cd "$PROJECT_ROOT/go"

tinygo build \
  -o "$OUT_DIR/hotwatermat.wasm" \
  -target wasm \
  -no-debug \
  ../wasm/main.go

echo "Built: $OUT_DIR/hotwatermat.wasm"

# Copy to npm package if it exists
if [ -d "$NPM_DIR/src" ]; then
  cp "$OUT_DIR/hotwatermat.wasm" "$NPM_DIR/src/"
  echo "Copied to: $NPM_DIR/src/hotwatermat.wasm"
fi

# Copy wasm_exec.js from TinyGo
WASM_EXEC="$(tinygo env TINYGOROOT)/targets/wasm_exec.js"
if [ -f "$WASM_EXEC" ]; then
  cp "$WASM_EXEC" "$OUT_DIR/wasm_exec.js"
  if [ -d "$NPM_DIR/src" ]; then
    cp "$WASM_EXEC" "$NPM_DIR/src/wasm_exec.js"
  fi
  echo "Copied wasm_exec.js"
fi

echo "Done."
