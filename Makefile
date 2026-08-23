.PHONY: test verify build

test:
	go test ./...
verify:
	./scripts/verify.sh
build:
	./scripts/build.sh
