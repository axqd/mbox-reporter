BINARY  := mbox-reporter
GOFLAGS := -trimpath

.PHONY: all build test vet fix lint clean

all: fix vet lint test build

build:
	go build $(GOFLAGS) -o $(BINARY) ./cmd/mbox-reporter

test:
	go test ./...

vet:
	go vet ./...

fix:
	go fix ./...

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY)
