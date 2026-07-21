default: test lint

# Refresh the private testing/ source specs from a Jamf Platform APIs GitOps
# archive, then regenerate as usual: `make ingest ZIP=path/to/archive.zip`.
# App Installer specs are a manual exception and are left untouched.
ingest:
	cd tools/generate && go run ./ingest -root "$(CURDIR)" -zip "$(ZIP)"

# Reconcile config.json with the Pro spec after Jamf publishes a new endpoint
# version: synthesize any missing spec version by cloning its closest config
# sibling. Idempotent; re-run `make generate` afterwards.
backfill:
	cd tools/generate && go run ./backfill -root "$(CURDIR)"

generate:
	cd tools/generate && go run . -root $(CURDIR)

test:
	go test -v -cover -count=1 -timeout=120s ./...

testacc:
	JAMFPLATFORM_ACC=1 go test -v -cover -count=1 -tags acceptance -timeout 120m -p=1 ./...

# Each tool is a separate module, so a root-only run never descends into them.
# tools/ is entered only at acctargets: the module root also holds tools.go, a
# build-tagged blank import of copywrite (a main package) that typecheck rejects
# by design and which exists solely to pin the dependency.
lint:
	golangci-lint run ./...
	cd tools/generate && golangci-lint run ./...
	cd tools && golangci-lint run ./acctargets/...

.PHONY: default ingest backfill generate test testacc lint
