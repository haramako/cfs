package cfs

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
)

type OptionInfo struct {
	Tag        string          `json:"tag"`
	Repository string          `json:"repository"`
	File       FileOption      `json:"file"`
	Aws        S3UploderOption `json:"aws"`
	Sftp       SftpOption      `json:"sftp"`
	Recursive  bool            `json:"recursive"`
	Flatten    bool            `json:"flatten"`
	Compress   bool            `json:"compress"`
	EncryptKey string          `json:"encryptKey"`
	EncryptIv  string          `json:"encryptIv"`
}

var Option = &OptionInfo{
	Recursive:  true,
	Compress:   true,
	Flatten:    true,
	EncryptKey: "aiRue7ooouNee0IeooneeN2eel9Aifie",
	EncryptIv:  "Yee9zoogoow3Geiz",
}

func LoadDefaultOptions() {
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
