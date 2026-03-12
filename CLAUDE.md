# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Go SDK for the Jamf Platform REST API (`github.com/Jamf-Concepts/jamfplatform-go-sdk`). Provides typed methods for blueprints, devices, device groups, device actions, compliance benchmarks (CBEngine), and RSQL filter building. OAuth2 client credentials auth via `golang.org/x/oauth2`.

## Commands

```bash
make test        # Run unit tests: go test -v -cover -count=1 -timeout=120s ./...
make testacc     # Run acceptance tests (requires JAMFPLATFORM_ACC=1): real API calls, -timeout 120m -p=1
make lint        # golangci-lint run ./...
go test -v -run TestFunctionName ./jamfplatform/  # Run a single test
```

## Architecture

Two-layer design, intentionally thin:

- **`jamfplatform/`** — Exported package. All resource types, API methods, and the public `Client`. Methods call `c.transport.Do`/`DoExpect`/`DoWithContentType` directly. One file per API domain (`blueprint.go`, `device.go`, `device_group.go`, `device_action.go`, `benchmark.go`).
- **`internal/client/`** — HTTP transport only. Handles OAuth2 auth, request/response marshaling, error handling, logging, and the generic `ListAllPages[T]` paginator. No resource-specific types belong here.

### Key transport methods

- `Do(ctx, method, path, body, result)` — expects 200 OK
- `DoExpect(ctx, method, path, body, expectedStatus, result)` — expects specific status
- `DoWithContentType(ctx, method, path, body, contentType, expectedStatus, result)` — overrides Content-Type (PATCH defaults to `application/merge-patch+json`)

### Pagination

`ListAllPages[T]` is a generic helper that takes a `fetchPage(ctx, page, pageSize) ([]T, bool, error)` callback. Go infers the type argument — do not use explicit type parameters (triggers `infertypeargs` lint).

### RSQL filters

`RSQLClause`, `BuildRSQLExpression`, `FormatArgument` live in `internal/client/rsql.go` and are re-exported from `jamfplatform/rsql.go`.

### Error handling

`ErrAuthentication` and `ErrNotFound` are sentinel errors. `APIResponseError` has `HasStatus(code)` for status inspection. All re-exported from `jamfplatform/errors.go`.

### API versioning

Types and endpoints use explicit version suffixes (`V1`, `V2`). Benchmark CRUD uses V2 endpoints; delete and baselines/rules use V1.

## Conventions

- MIT license. Copyright headers managed by HashiCorp `copywrite` (uses `--plan` flag, not `--check`).
- Options pattern for client configuration: `WithUserAgent`, `WithHTTPClient`, `WithLogger`.
- Pointer fields (`*string`, `*bool`) for optional/nullable JSON. `NullableString` type for fields that need explicit `null` vs omitted.
- `url.PathEscape` for path parameters, `url.QueryEscape` for query parameters.
- Error wrapping: `fmt.Errorf("MethodName(%s): %w", id, err)`.
