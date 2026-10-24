revision=$(shell git rev-parse --short HEAD)
LDFLAGS=-ldflags "-X main.revision=$(revision)"

all: darwin linux windows

.PHONY: all darwin linux windows test test-file test-gcs test-s3 vet

darwin:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/darwin/cfs ./cmd/cfs

linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/linux/cfs ./cmd/cfs

windows:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/windows/cfs.exe ./cmd/cfs

test: test-file test-s3 test-gcs

test-file:
	go test ./...

test-gcs:
	CFS_TEST_STORAGE=gcs go test ./internal/cfs

test-s3:
	CFS_TEST_STORAGE=s3 go test ./internal/cfs

vet:
	go vet ./...
