#!/usr/bin/env bash
set -euo pipefail

DOCS_URL="${CANVAS_DOCS_URL:-https://documentation.instructure.com/doc/api/all_resources.html}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

curl --fail --location --silent --show-error "$DOCS_URL" --output "$TMP_DIR/all_resources.html"
go run ./tools/canvas-docs-gen \
  -html "$TMP_DIR/all_resources.html" \
  -out "$TMP_DIR/canvas.openapi.yaml" \
  -source-url "$DOCS_URL"
go run ./tools/openapi-gen \
  -spec "$TMP_DIR/canvas.openapi.yaml" \
  -out internal/api/generated.go
gofmt -w internal/api/generated.go

echo "Updated internal/api/generated.go from $DOCS_URL"
