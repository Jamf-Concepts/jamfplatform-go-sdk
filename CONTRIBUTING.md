# Contributing

Thank you for your interest in contributing to the Jamf Platform Go SDK.

## Prerequisites

- **Go** >= 1.26 (see `go.mod` for the exact version)
- **golangci-lint** for linting
- A Jamf Platform tenant with API credentials (for acceptance tests only)

## Getting Started

```bash
# Clone the repository
git clone https://github.com/Jamf-Concepts/jamfplatform-go-sdk.git
cd jamfplatform-go-sdk

# Run tests
go test -v -count=1 -timeout=120s ./...

# Run linting
golangci-lint run ./...
```

## Development Workflow

1. Create a feature branch from `dev`.
2. Make your changes.
3. Run tests and linting before committing:

   ```bash
   go test -v -count=1 -timeout=120s ./...
   go vet ./...
   golangci-lint run ./...
   ```

4. Open a pull request against `dev`. CI will run tests, vet, lint, mod tidy, copyright headers, and vulnerability checks automatically.

## Running Acceptance Tests

Acceptance tests run against a live Jamf Platform tenant. Set the following environment variables:

```bash
export JAMFPLATFORM_BASE_URL="https://your-tenant.apigw.jamf.com"
export JAMFPLATFORM_CLIENT_ID="your-client-id"
export JAMFPLATFORM_CLIENT_SECRET="your-client-secret"
```

Then run:

```bash
make testacc
```

Or manually:

```bash
JAMFPLATFORM_ACC=1 go test -v -cover -count=1 -timeout 120m -p=1 ./...
```

## Project Structure

| Directory          | Purpose                                          |
| ------------------ | ------------------------------------------------ |
| `jamfplatform/`    | Exported SDK package — typed client methods      |
| `internal/client/` | Transport layer — HTTP, auth, pagination, errors |
| `tools/`           | Dev tool dependencies (copywrite)                |

## Adding a New Resource

1. Add the types and client methods in `jamfplatform/<resource>.go`.
2. Add unit tests in `jamfplatform/<resource>_test.go` using the `testServer` helper.
3. Ensure copyright headers are present (`copywrite headers --config .copywrite.hcl`).
4. Run tests and linting.

## Dependencies

This project uses native Go and `golang.org/x/oauth2`. Do not introduce third-party dependencies without discussion.

## Commit Messages

Use [conventional commit](https://www.conventionalcommits.org/) style messages:

- `feat: add device_group resource support`
- `fix: handle nil response in GetDevice`
- `test: add unit tests for benchmark methods`
- `refactor: extract shared pagination logic`
- `chore: update CI workflow action versions`
- `docs: update README with new usage examples`

## Pull Requests

- Keep PRs focused — one feature or fix per PR.
- Include unit tests for new resources.
- CI must pass before merge.

## Reporting Issues

Open an issue on GitHub with:

- SDK version (Go module version or commit SHA).
- Relevant code snippet (redact credentials).
- Expected vs actual behaviour.
- Any error messages or logs.
