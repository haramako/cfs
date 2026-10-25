package cfs

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

func isWindows() bool {
	return os.PathSeparator == '\\' && os.PathListSeparator == ';'
}

// HashLength は、ハッシュ（SHA-256 の 16 進表記）の文字数
const HashLength = sha256.Size * 2

// SumHash は、データのハッシュ（SHA-256 の 16 進表記）を返す
func SumHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func isHash(str string) bool {
	if len(str) != HashLength {
		return false
	}
	for _, c := range str {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func hashPath(hash string) string {
	if !isHash(hash) {
		panic("invalid hash " + hash)
	}
	return hash[0:2] + "/" + hash[2:]
}

// dataKey は、中身のハッシュからキャビネット上の場所を返す
func dataKey(hash string) string {
	return "data/" + hashPath(hash)
}

// manifestKey は、マニフェストのハッシュからキャビネット上の場所を返す
func manifestKey(hash string) string {
	return "manifest/" + hashPath(hash)
}

// tagKey は、タグのキャビネット上の場所を返す
func tagKey(tag string) string {
	return "tag/" + tag
}

// encode は、属性に従って圧縮・暗号化したデータを返す（圧縮 → 暗号化の順）
func encode(origData []byte, keys *KeyRing, attr ContentAttribute) ([]byte, error) {
	data := origData

	if attr.Compressed() {
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		w.Write(origData)
		w.Close()
		data = buf.Bytes()
	}

	if attr.Crypted() {
		key, err := keys.Current()
		if err != nil {
			return nil, err
		}
		data = key.Encrypt(data)
	}

	return data, nil
}

// decode は、encode したデータを元に戻す
func decode(data []byte, keys *KeyRing, attr ContentAttribute) ([]byte, error) {
	if attr.Crypted() {
		var err error
		data, err = keys.Decrypt(data)
		if err != nil {
			return nil, err
		}
	}

	if attr.Compressed() {
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}

		buf := bytes.NewBuffer(nil)
		_, err = io.Copy(buf, r)
		if err != nil {
			return nil, err
		}

		r.Close()
		data = buf.Bytes()
	}

	return data, nil
}
