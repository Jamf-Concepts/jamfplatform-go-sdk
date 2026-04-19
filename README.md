# jamfplatform-go-sdk

Go client library for the [Jamf Platform API](https://developer.jamf.com/platform-api).

All types, methods, and unit tests are generated from OpenAPI spec files. Published API specs are available in the [`api/`](api/) directory.

## Installation

```bash
go get github.com/Jamf-Concepts/jamfplatform-go-sdk
```

## Usage

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/devices"
)

func main() {
	client := jamfplatform.NewClient(
		"https://your-tenant.apigw.jamf.com",
		os.Getenv("JAMFPLATFORM_CLIENT_ID"),
		os.Getenv("JAMFPLATFORM_CLIENT_SECRET"),
		jamfplatform.WithTenantID(os.Getenv("JAMFPLATFORM_TENANT_ID")),
	)

	ctx := context.Background()

	// List all devices
	ds, err := devices.New(client).ListDevices(ctx, nil, "")
	if err != nil {
		log.Fatal(err)
	}
	for _, d := range ds {
		fmt.Printf("%s  %s  %s\n", d.ID, d.Name, d.SerialNumber)
	}
}
```

### Authentication

The client uses OAuth 2.0 client credentials. Create API credentials in your Jamf Platform tenant and provide the client ID and secret.

Token refresh is handled automatically.

### Client options

```go
client := jamfplatform.NewClient(baseURL, clientID, clientSecret,
	jamfplatform.WithTenantID(tenantID),
	jamfplatform.WithUserAgent("my-app/1.0"),
	jamfplatform.WithHTTPClient(customHTTPClient),
	jamfplatform.WithLogger(myLogger),
	jamfplatform.WithFileTokenCache("/tmp/tokens"),
)
```

### Error handling

```go
import (
	"errors"

	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform"
	"github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/devices"
)

device, err := devices.New(client).GetDevice(ctx, id)
if errors.Is(err, jamfplatform.ErrNotFound) {
	// handle not found
}

// Or inspect the full API error
var apiErr *jamfplatform.APIResponseError
if errors.As(err, &apiErr) {
	fmt.Println(apiErr.StatusCode, apiErr.TraceID)
}
```

### RSQL filters

Build filters for list endpoints:

```go
filter := jamfplatform.BuildRSQLExpression([]jamfplatform.RSQLClause{
	{Selector: "name", Operator: "==", Argument: "MacBook*"},
	{Selector: "operatingSystemVersion", Operator: "=gt=", Argument: "15.0"},
})
ds, err := devices.New(client).ListDevices(ctx, nil, filter)
```

### Async polling

For async operations (e.g. benchmark sync), use the `PollUntil` helper:

```go
ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
defer cancel()

cb := compliancebenchmarks.New(client)
err := jamfplatform.PollUntil(ctx, 5*time.Second, func(ctx context.Context) (bool, error) {
	bm, err := cb.GetBenchmark(ctx, id)
	if err != nil {
		return false, err
	}
	return bm.SyncState == "SYNCED", nil
})
```

## API coverage

Each API family lives in its own sub-package under `jamfplatform/`. Construct a service client with `<pkg>.New(rootClient)`.

| Sub-package | API |
|---|---|
| `jamfplatform/devices` | Platform device inventory |
| `jamfplatform/devicegroups` | Platform device groups |
| `jamfplatform/deviceactions` | Platform MDM commands (erase, restart, shutdown, unmanage, check-in) |
| `jamfplatform/blueprints` | Platform blueprints + components |
| `jamfplatform/ddmreport` | Platform declaration reporting |
| `jamfplatform/compliancebenchmarks` | Platform compliance benchmarks |
| `jamfplatform/pro` | Jamf Pro JSON API (buildings, packages, policies, MDM, enrollment, settings, PKI, etc.) |
| `jamfplatform/proclassic` | Jamf Classic XML API (computers, mobile devices, groups, profiles, policies, etc.) |

All list methods handle pagination automatically. Pro's versioned endpoints emit version-suffixed Go methods (`ListBuildingsV1`, `GetCheckInSettingsV3`) so consumers pin to a specific API version. Exact method lists are generated from the OpenAPI specs under `testing/` — see the published specs in [`api/`](api/) for the current surface.

### Classic (XML) example

```go
import "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/proclassic"

classic := proclassic.New(client)
computer, err := classic.GetComputerByID(ctx, "42")
if err != nil {
    log.Fatal(err)
}
fmt.Println(computer.General.Name, computer.Hardware.ModelIdentifier)
```

Classic is fully typed — the generator hoists nested XML sections (`general`, `hardware`, `purchasing`, etc.) into named structs and emits every field as a pointer so three-state null/value semantics round-trip cleanly (required for the upcoming Terraform provider).

### Pro example

```go
import "github.com/Jamf-Concepts/jamfplatform-go-sdk/jamfplatform/pro"

p := pro.New(client)
pkgs, err := p.ListPackagesV1(ctx, nil, "")
if err != nil {
    log.Fatal(err)
}
for _, pkg := range pkgs {
    fmt.Println(pkg.ID, pkg.PackageName, pkg.FileName)
}

// Multipart .pkg upload
f, _ := os.Open("my-app.pkg")
defer f.Close()
created, _ := p.CreatePackageV1(ctx, &pro.Package{
    PackageName: "my-app",
    FileName:    "my-app.pkg",
    CategoryID:  "-1",
})
_, err = p.UploadPackageV1(ctx, created.ID, "my-app.pkg", f)
```

## Code generation

All SDK types, methods, and unit tests are generated from OpenAPI spec files using a custom generator built on [kin-openapi](https://github.com/getkin/kin-openapi). The generator also publishes filtered API specs to `api/` containing only the public SDK surface.

```bash
make generate    # regenerate Go code, tests, and published API specs
make test        # run unit tests
make testacc     # run acceptance tests (requires API credentials)
make lint        # run golangci-lint
```

The only handwritten source file is `jamfplatform/client.go`. Everything else is generated from the specs in `testing/` via the config in `tools/generate/config.json`.

To add a new API endpoint:

1. Ensure the endpoint is defined in the OpenAPI spec file under `testing/`
2. Add an operation entry to `tools/generate/config.json`
3. Run `make generate`

CI enforces that generated output is current on every pull request.

## License

[MIT](LICENSE)
