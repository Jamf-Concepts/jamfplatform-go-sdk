default: test lint

generate:
	cd tools/generate && go run . -root $(CURDIR)

test:
	go test -v -cover -count=1 -timeout=120s ./...

testacc:
	JAMFPLATFORM_ACC=1 go test -v -cover -count=1 -tags acceptance -timeout 120m -p=1 ./...

# The generator is a separate module, so a root-only run never descends into it
# and nothing under tools/generate was ever linted. tools/ itself is skipped:
# its only file is a build-tagged blank import of copywrite (a main package),
# which typecheck rightly rejects and which exists solely to pin the dependency.
lint:
	golangci-lint run ./...
	cd tools/generate && golangci-lint run ./...

.PHONY: default generate test testacc lint
