BINARY_NAME=subnet-sentinel

build:
	go build -o bin/$(BINARY_NAME) ./cmd/subnet-sentinel

build-linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/subnet-sentinel

build-linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/subnet-sentinel

build-all: build-linux-amd64 build-linux-arm64

test:
	go test ./...
