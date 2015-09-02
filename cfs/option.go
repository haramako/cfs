package cfs

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
)

type OptionInfo struct {
	Tag        string          `json:"tag"`
	Repository string          `json:"repository"`
	Aws        S3UploderOption `json:"aws"`
}

var Option = &OptionInfo{}

func loadDefaultOptions() {
	data, err := ioutil.ReadFile(".cfsenv")
	if err != nil {
		return
	}
	Option.Parse(data)
}

func (o *OptionInfo) Parse(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	err := dec.Decode(o)
	if err != nil {
		return err
	}
	return nil
}
