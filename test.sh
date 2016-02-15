#!/bin/sh

cd cfs

echo testing file uploader
CFS_TEST_UPLOADER=file go test

# echo testing sftp uploader
# CFS_TEST_UPLOADER=sftp go test

if [ $ALL ] ; then
	echo testing s3 uploader
	CFS_TEST_UPLOADER=s3 go test
fi
