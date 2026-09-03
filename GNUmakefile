default: test lint

generate:
	cd tools/generate && go run . -root $(CURDIR)

test:
	go test -v -cover -count=1 -timeout=120s ./...
	cd tools/generate && go test -count=1 -timeout=120s ./...
	cd tools && go test -count=1 -timeout=120s ./acctargets/... ./acclanes/...

testacc:
	go test -v -cover -count=1 -tags acceptance -timeout 120m -p=1 ./...

# Each tool is a separate module, so a root-only run never descends into them.
# tools/ is entered only at its command subpackages (acctargets, acclanes): the
# module root also holds tools.go, a build-tagged blank import of copywrite (a
# main package) that typecheck rejects by design and which exists solely to pin
# the dependency.
#
# These lists must match ci.yml's test job and lint matrix. They drifted once
# already — acclanes landed in CI and not here, so `make lint` before pushing
# linted less than CI did and `make test` never ran the tests that guard the
# acceptance matrix.
lint:
	golangci-lint run ./...
	cd tools/generate && golangci-lint run ./...
	cd tools && golangci-lint run ./acctargets/... ./acclanes/...

.PHONY: default generate test testacc lint
