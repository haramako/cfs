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
	NoCache    bool

	// common setting
	Cabinet string // アップロード先のURL
	Url     string // ダウンロード先のURL

	// Google Cloud Storage setting

}

var Option = &OptionInfo{
	Recursive:  true,
	Compress:   true,
	Flatten:    false,
	EncryptKey: "",
	EncryptIv:  "",
	Cabinet:    "file:///var/cfs",
}

// cfsを使うときの設定ファイルを読み込む
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
			optionJson, _ := json.Marshal(Option)
			fmt.Printf("option loaded as %s\n", optionJson)
		}
	}
}

func (o *OptionInfo) Parse(data []byte) error {
	return json.Unmarshal(data, o)
}
