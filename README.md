# jamfplatform-go-sdk

Go client library for the [Jamf Platform API](https://developer.jamf.com/platform-api).

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
	jamfplatform.WithUserAgent("my-app/1.0"),
	jamfplatform.WithHTTPClient(customHTTPClient),
	jamfplatform.WithLogger(myLogger),
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

## API coverage

| Domain | Methods |
|--------|---------|
| Devices | ListDevices, GetDevice, GetDeviceBySerialNumber, UpdateDevice, DeleteDevice, ListDeviceApplications, ListDevicesForUser |
| Device Groups | ListDeviceGroups, GetDeviceGroup, CreateDeviceGroup, UpdateDeviceGroup, DeleteDeviceGroup, ListDeviceGroupMembers, UpdateDeviceGroupMembers, ListDeviceGroupsForDevice |
| Device Actions | EraseDevice, RestartDevice, ShutdownDevice, UnmanageDevice |
| Blueprints | ListBlueprints, GetBlueprint, GetBlueprintByName, CreateBlueprint, UpdateBlueprint, DeleteBlueprint, DeployBlueprint, UndeployBlueprint, ListBlueprintComponents, GetBlueprintComponent |
| Compliance Benchmarks | ListBaselines, GetBaselineRules, ListBenchmarks, GetBenchmark, GetBenchmarkByTitle, CreateBenchmark, DeleteBenchmark |

All list methods handle pagination automatically.

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

## License

[MIT](LICENSE)
