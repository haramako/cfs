package cfs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	Url     string // ダウンロード元。空ならキャビネットから直接読む

	// S3 のリージョン。空ならバケットから自動で判定する
	S3Region string
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
// Cabinet と Url に手元の相対パスが書かれていたら、設定ファイルのあるフォルダを基準にする
func LoadDefaultOptions(configFile string) error {
	if configFile == "" {
		configFile = ".cfsenv"
	}
	data, err := os.ReadFile(configFile)
	if err == nil {
		err = Option.Parse(data)
		if err != nil {
			fmt.Printf("cannot parse %s, %s\n", configFile, err)
		}
		base, err := filepath.Abs(filepath.Dir(configFile))
		if err != nil {
			return err
		}
		if Option.Cabinet, err = ResolveLocation(Option.Cabinet, base); err != nil {
			return fmt.Errorf("invalid Cabinet in %s: %w", configFile, err)
		}
		if Option.Url, err = ResolveLocation(Option.Url, base); err != nil {
			return fmt.Errorf("invalid Url in %s: %w", configFile, err)
		}
	}

	// 暗号鍵は、リポジトリに置く .cfsenv ではなく環境変数から渡せる（環境変数が優先）
	if key := os.Getenv("CFS_ENCRYPT_KEY"); key != "" {
		Option.EncryptKey = key
	}
	if iv := os.Getenv("CFS_ENCRYPT_IV"); iv != "" {
		Option.EncryptIv = iv
	}

	if Verbose {
		fmt.Printf("option loaded as %s\n", Option.maskedJSON())
	}
	return nil
}

// maskedJSON は、暗号鍵を伏せた設定の JSON を返す（ログ用）
func (o *OptionInfo) maskedJSON() []byte {
	masked := *o
	if masked.EncryptKey != "" {
		masked.EncryptKey = "***"
	}
	if masked.EncryptIv != "" {
		masked.EncryptIv = "***"
	}
	data, _ := json.Marshal(masked)
	return data
}

func (o *OptionInfo) Parse(data []byte) error {
	return json.Unmarshal(data, o)
}
