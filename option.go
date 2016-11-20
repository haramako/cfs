package cfs

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
)

type OptionInfo struct {
	Tag        string
	Repository string
	Recursive  bool
	Flatten    bool
	Compress   bool
	EncryptKey string
	EncryptIv  string

	// common setting
	Cabinet string

	// Google Cloud Storage setting

	// Server settings
	AdminUser string
	AdminPass string
}

var Option = &OptionInfo{
	Recursive:  true,
	Compress:   true,
	Flatten:    false,
	EncryptKey: "",
	EncryptIv:  "",
	Cabinet:    "http://localhost:8086",
}

func LoadDefaultOptions(configFile string) {
	if configFile == "" {
		configFile = ".cfsenv"
	}
	data, err := ioutil.ReadFile(configFile)
	if err == nil {
		err = Option.Parse(data)
		if err != nil {
			fmt.Printf("cannot parse %s, %s\n", configFile, err)
		}
		if Verbose {
			option_json, _ := json.Marshal(Option)
			fmt.Printf("option loaded as %s\n", option_json)
		}
	}
}

func (o *OptionInfo) Parse(data []byte) error {
	return json.Unmarshal(data, o)
}
