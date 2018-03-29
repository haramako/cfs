all: darwin linux windows

.PHONY: all darwin linux windows test test-file test-gcs

darwin:
	cd cmd/cfssv; GOOS=darwin GOARCH=amd64 go build -o ../../bin/darwin/cfssv
	cd cmd/cfsctl; GOOS=darwin GOARCH=amd64 go build -o ../../bin/darwin/cfsctl

linux:
	cd cmd/cfssv; GOOS=linux GOARCH=amd64 go build -o ../../bin/linux/cfssv
	cd cmd/cfsctl; GOOS=linux GOARCH=amd64 go build -o ../../bin/linux/cfsctl

windows:
	cd cmd/cfssv; GOOS=windows GOARCH=amd64 go build -o ../../bin/windows/cfssv.exe
	cd cmd/cfsctl; GOOS=windows GOARCH=amd64 go build -o ../../bin/windows/cfsctl.exe

test: test-file test-s3 test-gcs

test-file:
	go test

test-cfs:
	CFS_TEST_STORAGE=cfs go test

test-gcs:
	CFS_TEST_STORAGE=gcs go test

test-s3:
	CFS_TEST_STORAGE=s3 go test
