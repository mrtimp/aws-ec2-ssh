VERSION := $(shell git rev-parse --short HEAD)
BUILDTIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

GOLDFLAGS += -s -w
GOLDFLAGS += -X main.Version=$(VERSION)
GOLDFLAGS += -X main.Buildtime=$(BUILDTIME)
GOFLAGS = -ldflags "$(GOLDFLAGS)"

dep:
	go mod download

build:
	goreleaser build --clean

# Optionally, you can pass extra GoReleaser build flags via env var:
# make build GORELEASER_FLAGS="--single-target --id ec2-ssh --os darwin --arch arm64"
build-custom:
	goreleaser build --clean $(GORELEASER_FLAGS)

optimize:
	if [ -x /usr/bin/upx ] || [ -x /usr/local/bin/upx ]; then upx --brute ${BINARY_NAME}-*; fi

release:
	goreleaser release --clean

test:
	go test -v ./...

clean:
	go clean
	rm -rf dist bin

