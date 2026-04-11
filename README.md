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
	devices, err := client.ListDevices(ctx, nil, "")
	if err != nil {
		log.Fatal(err)
	}
	for _, d := range devices {
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
import "errors"

device, err := client.GetDevice(ctx, id)
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
devices, err := client.ListDevices(ctx, nil, filter)
```

### Async polling

For async operations (e.g. benchmark sync), use the `PollUntil` helper:

```go
ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
defer cancel()

err := jamfplatform.PollUntil(ctx, 5*time.Second, func(ctx context.Context) (bool, error) {
	bm, err := client.GetBenchmark(ctx, id)
	if err != nil {
		return false, err
	}
	return bm.SyncState == "SYNCED", nil
})
```

## API coverage

| Domain | Methods |
|--------|---------|
| Devices | ListDevices, GetDevice, UpdateDevice, DeleteDevice, ListDeviceApplications, ListDevicesForUser |
| Device Groups | ListDeviceGroups, GetDeviceGroup, CreateDeviceGroup, UpdateDeviceGroup, DeleteDeviceGroup, ListDeviceGroupMembers, UpdateDeviceGroupMembers, ListDeviceGroupsForDevice |
| Device Actions | CheckInDevice, EraseDevice, RestartDevice, ShutdownDevice, UnmanageDevice |
| Blueprints | ListBlueprints, GetBlueprint, CreateBlueprint, UpdateBlueprint, DeleteBlueprint, DeployBlueprint, UndeployBlueprint, GetBlueprintReport, ListBlueprintComponents, GetBlueprintComponent |
| Compliance Benchmarks | ListBaselines, GetBaselineRules, ListBenchmarks, GetBenchmark, CreateBenchmark, DeleteBenchmark |
| Benchmark Reporting | ListBenchmarkRulesStats, ListBenchmarkRuleDevices, GetBenchmarkCompliancePercentage |
| DDM Declarations | GetDeviceDeclarationReport, ListDeclarationReportClients |

All list methods handle pagination automatically.

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
