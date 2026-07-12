# Contributing

Thanks for helping improve Canvas CLI.

## Development setup

Canvas CLI requires the Go version declared in `go.mod`.

```bash
git clone https://github.com/hhe48203-ctrl/canvas-cli.git
cd canvas-cli
go test ./...
go vet ./...
go build -o canvas .
```

Tests use local `httptest` servers and do not contact a real Canvas instance.

## Before opening a pull request

```bash
gofmt -w .
go test ./...
go vet ./...
go build -o canvas .
```

Keep changes focused and include tests for behavior changes. Never commit Canvas access tokens, `.env` files, real course data, or student data.

## Updating the API catalog

The checked-in catalog is generated from Instructure's official Canvas REST API documentation:

```bash
scripts/update-api-catalog.sh
go test ./...
```

Alternatively, generate it from a Canvas OpenAPI specification:

```bash
scripts/generate-openapi.sh path/to/canvas.openapi.yaml
go test ./...
```

Generated catalog changes should be committed together with the generator change or upstream documentation update that produced them.
