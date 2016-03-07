package cfs

import (
	//"fmt"
	"testing"
)

func TestParse(t *testing.T) {
	opt := &OptionInfo{}
	err := opt.Parse([]byte(`{"tag":"hoge", "aws":{"bucketName":"buk"}}`))
	if err != nil {
		t.Errorf("cannot parse")
	}
}
