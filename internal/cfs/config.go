package cfs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config は、cfs の動作の設定。.cfsenv と環境変数から読み、Client や Downloader などに渡す
type Config struct {
	Recursive bool // フォルダの中も上げる
	Flatten   bool // パスをファイル名だけにする
	Compress  bool // 圧縮する

	// アップロードの対象から外すファイルのパターン（.gitignore と似た書き方。exclude.go を参照）。
	// DefaultExcludes（".*"、"#*"、"*~"）の後に続けて使う
	Exclude []string
	// 暗号化しないファイルのパターン（書き方は Exclude と同じ）。EncryptKey があっても暗号化しない
	NoEncrypt []string
	// 圧縮しないファイルのパターン（書き方は Exclude と同じ）。Compress が true でも圧縮しない
	NoCompress []string

	// 暗号化の鍵（任意の長さの文字列。16 文字以上）。環境変数 CFS_ENCRYPT_KEY が優先
	EncryptKey string
	// 鍵を入れ替えた後も、古い鍵で暗号化したデータを読むための鍵。環境変数 CFS_OLD_ENCRYPT_KEYS（カンマ区切り）が優先
	OldEncryptKeys []string

	// マニフェストの署名を確かめる公開鍵（base64）。設定されていれば、署名のないマニフェストは読まない
	VerifyKeys []string
	// マニフェストに署名する秘密鍵（base64）。秘密なので環境変数 CFS_SIGNING_KEY からだけ読む
	SigningKey string `json:"-"`

	Cabinet string // アップロード先（URL か手元のパス）
	Url     string // ダウンロード元。空ならキャビネットから直接読む

	// S3 のリージョン。空ならバケットから自動で判定する
	S3Region string

	// 手元のキャッシュのフォルダ。環境変数 CFS_CACHE_DIR、なければ OS のキャッシュフォルダの下の cfs
	CacheDir string `json:"-"`

	// 詳しいログを表示する
	Verbose bool `json:"-"`
}

// DefaultConfig は、既定の設定を返す
func DefaultConfig() *Config {
	return &Config{
		Recursive: true,
		Compress:  true,
	}
}

// LoadConfig は、既定の設定に、設定ファイル（.cfsenv）と環境変数を重ねて読む。設定ファイルがなければ使わない。
// Cabinet と Url に手元の相対パスが書かれていたら、設定ファイルのあるフォルダを基準にする
func LoadConfig(configFile string) (*Config, error) {
	cfg := DefaultConfig()
	if configFile == "" {
		configFile = ".cfsenv"
	}
	data, err := os.ReadFile(configFile)
	if err == nil {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("cannot parse %s: %w", configFile, err)
		}
		base, err := filepath.Abs(filepath.Dir(configFile))
		if err != nil {
			return nil, err
		}
		if cfg.Cabinet, err = ResolveLocation(cfg.Cabinet, base); err != nil {
			return nil, fmt.Errorf("invalid Cabinet in %s: %w", configFile, err)
		}
		if cfg.Url, err = ResolveLocation(cfg.Url, base); err != nil {
			return nil, fmt.Errorf("invalid Url in %s: %w", configFile, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	// 鍵は、リポジトリに置く .cfsenv ではなく環境変数から渡せる（環境変数が優先）
	if key := os.Getenv("CFS_ENCRYPT_KEY"); key != "" {
		cfg.EncryptKey = key
	}
	if keys := os.Getenv("CFS_OLD_ENCRYPT_KEYS"); keys != "" {
		cfg.OldEncryptKeys = strings.Split(keys, ",")
	}
	if key := os.Getenv("CFS_SIGNING_KEY"); key != "" {
		cfg.SigningKey = key
	}
	cfg.CacheDir = os.Getenv("CFS_CACHE_DIR")
	return cfg, nil
}

// maskedJSON は、鍵を伏せた設定の JSON を返す（ログ用）
func (c *Config) maskedJSON() []byte {
	masked := *c
	if masked.EncryptKey != "" {
		masked.EncryptKey = "***"
	}
	if len(masked.OldEncryptKeys) > 0 {
		masked.OldEncryptKeys = []string{"***"}
	}
	data, _ := json.Marshal(masked)
	return data
}

// String は、鍵を伏せた設定を返す（ログ用）
func (c *Config) String() string {
	return string(c.maskedJSON())
}

// KeyRing は、設定から暗号化の鍵の組を作る
func (c *Config) KeyRing() (*KeyRing, error) {
	return NewKeyRing(c.EncryptKey, c.OldEncryptKeys)
}

// Signer は、設定からマニフェストに署名する鍵を作る。設定されていなければ nil
func (c *Config) Signer() (*SigningKey, error) {
	if c.SigningKey == "" {
		return nil, nil
	}
	return ParseSigningKey(c.SigningKey)
}

// Verifier は、設定からマニフェストの署名を確かめる鍵の組を作る
func (c *Config) Verifier() (*Verifier, error) {
	return NewVerifier(c.VerifyKeys)
}

// DefaultAttribute は、設定に従った、ファイルの圧縮・暗号化の既定の属性を返す
func (c *Config) DefaultAttribute() ContentAttribute {
	var result ContentAttribute
	if c.Compress {
		result |= Compressed
	}
	if c.EncryptKey != "" {
		result |= Crypted
	}
	return result
}

// DownloadLocation は、ダウンロード元を返す。Url がなければ、キャビネット（手元のフォルダや S3）から直接読む
func (c *Config) DownloadLocation() string {
	if c.Url != "" {
		return c.Url
	}
	return c.Cabinet
}

// Logger は、Verbose なら標準出力に、そうでなければ何も出さない Logger を返す
func (c *Config) Logger() Logger {
	if c == nil || !c.Verbose {
		return nil
	}
	return func(format string, args ...any) { fmt.Printf(format, args...) }
}

// ManifestCacheDir は、前回アップロードしたマニフェストを置く手元のフォルダを返す（なければ作る）
func (c *Config) ManifestCacheDir() (string, error) {
	return c.cacheDir("manifest")
}

// DataCacheDir は、ダウンロードしたデータを置く手元のフォルダを返す（なければ作る）
func (c *Config) DataCacheDir() (string, error) {
	return c.cacheDir("data")
}

func (c *Config) cacheDir(name string) (string, error) {
	base := c.CacheDir
	if base == "" {
		userCache, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("cannot find the cache folder (set CFS_CACHE_DIR): %w", err)
		}
		base = filepath.Join(userCache, "cfs")
	}
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return "", err
	}
	return dir, nil
}

// orDefault は、nil なら既定の設定を返す
func (c *Config) orDefault() *Config {
	if c == nil {
		return DefaultConfig()
	}
	return c
}

// Logger は、詳細なログの出し先。nil なら何も出さない
type Logger func(format string, args ...any)

// Printf は、ログを出す（nil なら何もしない）
func (l Logger) Printf(format string, args ...any) {
	if l != nil {
		l(format, args...)
	}
}
