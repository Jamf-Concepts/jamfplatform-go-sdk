default: test lint

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

.PHONY: default generate test testacc lint
