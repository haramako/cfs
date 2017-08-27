all: darwin linux windows

.PHONY: all darwin linux windows test test-file test-gcs

darwin:
	cd cfssv; GOOS=darwin GOARCH=amd64 go build -o ../bin/darwin/cfssv
	cd cfsctl; GOOS=darwin GOARCH=amd64 go build -o ../bin/darwin/cfsctl

linux:
	cd cfssv; GOOS=linux GOARCH=amd64 go build -o ../bin/linux/cfssv
	cd cfsctl; GOOS=linux GOARCH=amd64 go build -o ../bin/linux/cfsctl

windows:
	cd cfssv; GOOS=windows GOARCH=amd64 go build -o ../bin/windows/cfssv.exe
	cd cfsctl; GOOS=windows GOARCH=amd64 go build -o ../bin/windows/cfsctl.exe

test: test-file test-s3 test-gcs

test-file:
	go test

test-cfs:
	CFS_TEST_STORAGE=cfs go test

test-gcs:
	CFS_TEST_STORAGE=gcs go test

test-s3:
	CFS_TEST_STORAGE=s3 go test
