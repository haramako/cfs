package cfs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type OptionInfo struct {
	Tag        string
	Repository string
	Recursive  bool
	Flatten    bool
	Compress   bool
	NoCache    bool

	// 暗号化の鍵（任意の長さの文字列。16 文字以上）。環境変数 CFS_ENCRYPT_KEY が優先
	EncryptKey string
	// 鍵を入れ替えた後も、古い鍵で暗号化したデータを読むための鍵。環境変数 CFS_OLD_ENCRYPT_KEYS（カンマ区切り）が優先
	OldEncryptKeys []string

	// マニフェストの署名を確かめる公開鍵（base64）。設定されていれば、署名のないマニフェストは読まない
	VerifyKeys []string
	// マニフェストに署名する秘密鍵（base64）。秘密なので環境変数 CFS_SIGNING_KEY からだけ読む
	SigningKey string `json:"-"`

	// common setting
	Cabinet string // アップロード先のURL
	Url     string // ダウンロード元。空ならキャビネットから直接読む

	// S3 のリージョン。空ならバケットから自動で判定する
	S3Region string
}

var Option = &OptionInfo{
	Recursive: true,
	Compress:  true,
	Flatten:   false,
	Cabinet:   "file:///var/cfs",
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

	// 鍵は、リポジトリに置く .cfsenv ではなく環境変数から渡せる（環境変数が優先）
	if key := os.Getenv("CFS_ENCRYPT_KEY"); key != "" {
		Option.EncryptKey = key
	}
	if keys := os.Getenv("CFS_OLD_ENCRYPT_KEYS"); keys != "" {
		Option.OldEncryptKeys = strings.Split(keys, ",")
	}
	if key := os.Getenv("CFS_SIGNING_KEY"); key != "" {
		Option.SigningKey = key
	}

	if Verbose {
		fmt.Printf("option loaded as %s\n", Option.maskedJSON())
	}
	return nil
}

// maskedJSON は、鍵を伏せた設定の JSON を返す（ログ用）
func (o *OptionInfo) maskedJSON() []byte {
	masked := *o
	if masked.EncryptKey != "" {
		masked.EncryptKey = "***"
	}
	if len(masked.OldEncryptKeys) > 0 {
		masked.OldEncryptKeys = []string{"***"}
	}
	data, _ := json.Marshal(masked)
	return data
}

func (o *OptionInfo) Parse(data []byte) error {
	return json.Unmarshal(data, o)
}

// KeyRing は、設定から暗号化の鍵の組を作る
func (o *OptionInfo) KeyRing() (*KeyRing, error) {
	return NewKeyRing(o.EncryptKey, o.OldEncryptKeys)
}

// Signer は、設定からマニフェストに署名する鍵を作る。設定されていなければ nil
func (o *OptionInfo) Signer() (*SigningKey, error) {
	if o.SigningKey == "" {
		return nil, nil
	}
	return ParseSigningKey(o.SigningKey)
}

// Verifier は、設定からマニフェストの署名を確かめる鍵の組を作る
func (o *OptionInfo) Verifier() (*Verifier, error) {
	return NewVerifier(o.VerifyKeys)
}
