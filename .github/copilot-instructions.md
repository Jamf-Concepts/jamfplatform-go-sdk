# Copilot Instructions

Go SDK for the Jamf Platform REST API. All types, methods, and unit tests are **generated** from OpenAPI spec files — the only handwritten source file is `jamfplatform/client.go`.

## Commands

```bash
make generate                                     # Regenerate Go code, tests, and published API specs
make test                                         # Unit tests: go test -v -cover -count=1 -timeout=120s ./...
make testacc                                      # Acceptance tests (requires credentials, -tags acceptance)
make lint                                         # golangci-lint run ./...
go test -v -run TestFunctionName ./jamfplatform/  # Run a single test
```

Before every commit, run `go fmt ./...` and `go fix ./...`.

## Architecture

### Two-layer design

- **`jamfplatform/`** — Exported package. Generated types and methods, plus the handwritten `Client` (options pattern: `WithTenantID`, `WithUserAgent`, `WithHTTPClient`, etc.). Sub-packages per API family: `devices`, `devicegroups`, `blueprints`, `pro`, `proclassic`, etc.
- **`internal/client/`** — HTTP transport. OAuth2 auth, request/response marshaling, error handling, logging, `ListAllPages[T]` paginator, `PollUntil` async poller, RSQL filter building.

Transport methods: `Do` (expects 200), `DoExpect` (expects specific status), `DoWithContentType` (overrides Content-Type).

### Code generation

The generator (`tools/generate/`) uses [kin-openapi](https://github.com/getkin/kin-openapi) to parse OpenAPI specs from `testing/` and produces Go code based on the whitelist in `tools/generate/config.json`.

**To add a new endpoint:** add an operation entry to `config.json` and run `make generate`. Compact config syntax:

```json
{"op": "GET /v1/devices/{id}", "name": "GetDevice"}
{"op": "GET /v1/devices", "name": "ListDevices", "pagination": "hasNext", "params": ["sort:[]string", "filter"]}
```

CI enforces generated output is current on every PR (`git diff --exit-code`).

### API formats

- **JSON** (default): Platform + Pro APIs. Structs use `json:"..."` tags.
- **XML**: Classic API (`proclassic.yaml`). Transport detects `/proclassic/` in the URL path, switches to `encoding/xml`. Structs use `xml:"..."` tags.

### Error handling

One error type: `*APIResponseError`. Accessors: `HasStatus(code)`, `Details()`, `FieldErrors()`, `Summary()`, `AsAPIError(err)`. No sentinel errors.

### Name→ID resolvers

Generator-emitted `Resolve<X>IDByName` and `Resolve<X>ByName` methods. Three modes: `filtered` (RSQL), `clientFilter` (in-memory match), `direct` (Classic by-name endpoints). Configured via `"resolver": {...}` in `config.json`.

### Pagination

`ListAllPages[T]` generic helper with three styles: `hasNext`, `sizeCheck`, `totalCount` — configured per operation in `config.json`.

## Key conventions

- **Never hand-edit generated files.** Every `.go` file in sub-packages (`pro/`, `proclassic/`, `devices/`, etc.) is generated. Modify the generator config or the generator itself instead.
- **Never modify OpenAPI specs under `testing/`.** They mirror Jamf's published specs. Fix spec quirks via generator-level config overrides or post-processing passes.
- Supplemental types (e.g. `BigInt`, `NotificationValue`) are emitted by the generator as static files — they live in `tools/generate/`, not hand-added to output packages.
- Each spec in config sets `"splitByTag": true` — methods are bucketed by OpenAPI tag into `<tag>.go` + `<tag>_test.go`; types pool into `types.go`.
- Each spec targets a sub-package via `"package": "<name>"` to avoid name collisions across API families.
- XML spec fields are always pointer + `,omitempty` (for Terraform plugin framework three-state semantics). JSON specs use the standard nullable/required heuristic.
- URL parameters: `url.PathEscape` for path, `url.QueryEscape` for query.
- Error wrapping: `fmt.Errorf("MethodName(%s): %w", id, err)`.
- Conventional commits: `feat:`, `fix:`, `test:`, `refactor:`, `chore:`, `docs:`.
- Copyright headers managed by HashiCorp `copywrite` (uses `--plan` flag, not `--check`).
- PRs target `dev` branch.

## Acceptance tests

Written in `jamfplatform/acc_<pkg>_test.go` (external `jamfplatform_test` package, `//go:build acceptance`). Require env vars: `JAMFPLATFORM_BASE_URL`, `JAMFPLATFORM_CLIENT_ID`, `JAMFPLATFORM_CLIENT_SECRET`.

- Read-only endpoints: call directly, log shape.
- Mutating endpoints: CRUD lifecycle with `t.Cleanup` deferring delete.
- Destructive endpoints: never run against shared state — create test resources within the test or `t.Skip()` with a comment.
- Never silently tolerate errors to make tests pass — understand and surface them.
