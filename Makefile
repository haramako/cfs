revision=$(shell git rev-parse --short HEAD)

all: darwin linux windows

.PHONY: all darwin linux windows test test-file test-gcs

darwin:
	cd cmd/cfs; GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.revision=$(revision)" -o ../../bin/darwin/cfs

linux:
	cd cmd/cfs; GOOS=linux GOARCH=amd64 go build -ldflags "-X main.revision=$(revision)" -o ../../bin/linux/cfs

windows:
	cd cmd/cfs; GOOS=windows GOARCH=amd64 go build -ldflags "-X main.revision=$(revision)" -o ../../bin/windows/cfs.exe

test: test-file test-s3 test-gcs

test-file:
	go test
	
test-cfs:
	CFS_TEST_STORAGE=cfs go test

test-gcs:
	CFS_TEST_STORAGE=gcs go test

test-s3:
	CFS_TEST_STORAGE=s3 go test
