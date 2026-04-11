default: test lint

generate:
	cd tools/generate && go run . -root $(CURDIR)

test:
	go test -v -cover -count=1 -timeout=120s ./...

testacc:
	JAMFPLATFORM_ACC=1 go test -v -cover -count=1 -tags acceptance -timeout 120m -p=1 ./...

lint:
	golangci-lint run ./...

.PHONY: default generate test testacc lint
