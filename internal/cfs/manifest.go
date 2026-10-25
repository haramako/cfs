package cfs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ExcludePatterns は除外するファイルのパターンを表す
var ExcludePatterns = []string{".*", "*.vdat", "cfs", "*.meta", "*.tmx"}

// Verbose 詳細なログを表示するかどうか
var Verbose = false

// マニフェストの形式（形式 v2）
//
//	#cfs-manifest 2
//	<ハッシュ>\t<パス>\t<サイズ>\t<更新日時>\t<元のハッシュ>\t<元のサイズ>\t<属性>   … 1 ファイル 1 行、パス順
//	#signature ed25519 <鍵 ID> <署名>   … 署名する場合だけ。それより前の全バイトへの署名
const (
	manifestHeader          = "#cfs-manifest 2"
	manifestSignaturePrefix = "#signature ed25519 "
)

// Manifest マニフェスト（1 回のアップロードで記録されるファイル一覧）
type Manifest struct {
	Tag         string
	Path        string // 手元のキャッシュのファイル（前回のアップロードのマニフェスト）
	Hash        string
	Contents    map[string]Content
	UploadCount int
}

// ContentAttribute はコンテンツの圧縮や暗号の状態を表すenum
type ContentAttribute int

const (
	// NoContentAttribute は圧縮も暗号化もしないことを示す
	NoContentAttribute ContentAttribute = 0
	// Compressed は圧縮をするかどうかを示す
	Compressed = 1
	// Crypted は暗号化をするかどうかを示す
	Crypted = 2
)

// Content は一つのファイルの内容を表すstruct
type Content struct {
	Path     string
	Hash     string // キャビネットに置いたデータ（圧縮・暗号化後）のハッシュ
	Time     time.Time
	Size     int
	OrigHash string // 元のファイルのハッシュ
	OrigSize int
	Attr     ContentAttribute
	Touched  bool
}

// DefaultContentAttribute デフォルトのContentAttributeを取得する
func DefaultContentAttribute() ContentAttribute {
	var result ContentAttribute
	if Option.Compress {
		result |= Compressed
	}
	if Option.EncryptKey != "" {
		result |= Crypted
	}
	return ContentAttribute(result)
}

// Compressed 圧縮するかどうかを返す
func (c ContentAttribute) Compressed() bool {
	return (int(c) & Compressed) != 0
}

func (c ContentAttribute) Crypted() bool {
	return (int(c) & Crypted) != 0
}

func NewManifest() *Manifest {
	return &Manifest{
		Contents: make(map[string]Content),
	}
}

// ManifestFromFile は、手元のキャッシュからマニフェストを読む。
// ファイルがない、または読めない（古い形式など）場合は、空のマニフェストを返す
func ManifestFromFile(path string) (*Manifest, error) {
	m := NewManifest()
	m.Path = path
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		return m, nil
	}
	if Verbose {
		fmt.Printf("read manifest from '%s'\n", m.Path)
	}
	if err := m.Parse(data, nil); err != nil {
		if Verbose {
			fmt.Printf("ignore local manifest cache '%s': %v\n", m.Path, err)
		}
		m = NewManifest()
		m.Path = path
		return m, nil
	}
	hash, err := os.ReadFile(filepath.FromSlash(path) + ".hash")
	if err == nil && isHash(strings.TrimSpace(string(hash))) {
		m.Hash = strings.TrimSpace(string(hash))
	}
	return m, nil
}

// Parse は、形式 v2 のマニフェストを読む。verifier に鍵が設定されていれば、署名を確かめる
func (m *Manifest) Parse(data []byte, verifier *Verifier) error {
	body := data
	var keyID, signature string
	if i := bytes.Index(data, []byte("\n"+manifestSignaturePrefix)); i >= 0 {
		body = data[:i+1]
		fields := strings.Fields(strings.TrimPrefix(string(data[i+1:]), manifestSignaturePrefix))
		if len(fields) != 2 {
			return fmt.Errorf("invalid manifest signature line")
		}
		keyID, signature = fields[0], fields[1]
	}

	if verifier.Enabled() {
		if signature == "" {
			return fmt.Errorf("manifest is not signed (VerifyKeys is set)")
		}
		if err := verifier.Verify(body, keyID, signature); err != nil {
			return err
		}
	}

	lines := strings.Split(string(body), "\n")
	if strings.TrimRight(lines[0], "\r") != manifestHeader {
		return fmt.Errorf("unsupported manifest format (header %q)", lines[0])
	}

	for n, line := range lines[1:] {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		col := strings.Split(line, "\t")
		if len(col) != 7 {
			return fmt.Errorf("invalid manifest line %d: %q", n+2, line)
		}
		size, err := strconv.Atoi(col[2])
		if err != nil {
			return err
		}
		time, err := time.Parse(time.RFC3339, col[3])
		if err != nil {
			return err
		}
		origSize, err := strconv.Atoi(col[5])
		if err != nil {
			return err
		}
		attr, err := strconv.Atoi(col[6])
		if err != nil {
			return err
		}
		if !isHash(col[0]) || !isHash(col[4]) {
			return fmt.Errorf("invalid hash in manifest line %d", n+2)
		}
		m.Contents[col[1]] = Content{
			Hash:     col[0],
			Path:     col[1],
			Size:     size,
			Time:     time,
			OrigHash: col[4],
			OrigSize: origSize,
			Attr:     ContentAttribute(attr),
		}
	}
	return nil
}

// Merge bにfromの内容をマージする
// 両方に存在する場合は、fromの内容で上書きされる
func (m *Manifest) Merge(from *Manifest) {
	for _, c := range from.Contents {
		m.Contents[c.Path] = c
	}
}

func (m *Manifest) RemoveUntouched() {
	newContents := make(map[string]Content)
	for _, c := range m.Contents {
		if c.Touched {
			newContents[c.Path] = c
		} else {
			if Verbose {
				fmt.Printf("removed: %s\n", c.Path)
			}
		}
	}
	m.Contents = newContents
}

// Dump は、Manifestの内容を形式 v2 で書き出す。signer があれば署名を付ける
func (m *Manifest) Dump(signer *SigningKey) []byte {
	keys := []string{}
	for k := range m.Contents {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteString(manifestHeader + "\n")
	for _, k := range keys {
		c := m.Contents[k]
		buf.WriteString(strings.Join(
			[]string{
				c.Hash,
				c.Path,
				strconv.Itoa(c.Size),
				c.Time.Format(time.RFC3339),
				c.OrigHash,
				strconv.Itoa(c.OrigSize),
				strconv.Itoa(int(c.Attr)),
			},
			"\t") + "\n")
	}

	if signer != nil {
		sig := signer.Sign(buf.Bytes())
		buf.WriteString(manifestSignaturePrefix + signer.KeyID() + " " + sig + "\n")
	}
	return buf.Bytes()
}

// DataHashes は、マニフェストが参照するデータのハッシュの一覧を返す（0 バイトのものは置かれないので除く）
func (m *Manifest) DataHashes() []string {
	hashes := make([]string, 0, len(m.Contents))
	for _, c := range m.Contents {
		if c.Size > 0 {
			hashes = append(hashes, c.Hash)
		}
	}
	return hashes
}

func (m *Manifest) GetAttribute(path string) ContentAttribute {
	var attr = DefaultContentAttribute()
	// TODO: とりあえずフィルタを固定している
	if filepath.Ext(path) == ".ab" || filepath.Ext(path) == ".raw" || filepath.Ext(path) == ".pbx" || filepath.Ext(path) == ".mp4" {
		attr = ContentAttribute(0)
	}
	return attr
}
