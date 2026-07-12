#!/usr/bin/env bash
set -euo pipefail

SPEC_PATH="${1:?usage: scripts/generate-openapi.sh path/to/canvas.openapi.yaml}"
go run ./tools/openapi-gen -spec "$SPEC_PATH" -out internal/api/generated.go
gofmt -w internal/api/generated.go
