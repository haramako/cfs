package cfs

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// 暗号化したデータの形式（形式 v2）
//
//	1 バイト   形式のバージョン（cryptFormatVersion）
//	8 バイト   鍵 ID（どの鍵で暗号化したか）
//	16 バイト  IV
//	残り       AES-256-CTR で暗号化したデータ
//
// 鍵と IV は EncryptKey（任意の長さの文字列）から HMAC-SHA256 で導出する。
// IV は中身から決まる（同じ鍵で同じ中身なら同じ暗号文になる）ので、暗号化しても重複の排除が効く。
// 改ざんの検出は、暗号化ではなくハッシュ（と、マニフェストの署名）で行う。
const cryptFormatVersion = 1

const (
	keyIDSize    = 8
	cryptHeader  = 1 + keyIDSize + aes.BlockSize
	encKeyLabel  = "cfs-encrypt-key"
	ivKeyLabel   = "cfs-iv-key"
	keyIDLabel   = "cfs-key-id"
	keyIDHexSize = keyIDSize * 2
)

// EncryptionKey は、EncryptKey から導出した暗号化用の鍵
type EncryptionKey struct {
	ID     [keyIDSize]byte
	encKey []byte
	ivKey  []byte
}

func hmacSHA256(key []byte, data []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(data)
	return m.Sum(nil)
}

// NewEncryptionKey は、EncryptKey の文字列から鍵を作る
func NewEncryptionKey(secret string) (*EncryptionKey, error) {
	if len(secret) < 16 {
		return nil, fmt.Errorf("encryption key must be at least 16 characters")
	}
	k := &EncryptionKey{
		encKey: hmacSHA256([]byte(secret), []byte(encKeyLabel)),
		ivKey:  hmacSHA256([]byte(secret), []byte(ivKeyLabel)),
	}
	copy(k.ID[:], hmacSHA256([]byte(secret), []byte(keyIDLabel)))
	return k, nil
}

// IDString は、鍵 ID の 16 進表記を返す
func (k *EncryptionKey) IDString() string {
	return hex.EncodeToString(k.ID[:])
}

// Encrypt は、データを暗号化して、形式 v2 のバイト列を返す
func (k *EncryptionKey) Encrypt(plain []byte) []byte {
	iv := hmacSHA256(k.ivKey, plain)[:aes.BlockSize]

	block, err := aes.NewCipher(k.encKey)
	if err != nil {
		panic(err) // 鍵の長さは常に 32 バイトなので起こらない
	}

	out := make([]byte, cryptHeader+len(plain))
	out[0] = cryptFormatVersion
	copy(out[1:], k.ID[:])
	copy(out[1+keyIDSize:], iv)
	cipher.NewCTR(block, iv).XORKeyStream(out[cryptHeader:], plain)
	return out
}

func (k *EncryptionKey) decryptBody(data []byte) []byte {
	iv := data[1+keyIDSize : cryptHeader]
	block, err := aes.NewCipher(k.encKey)
	if err != nil {
		panic(err)
	}
	plain := make([]byte, len(data)-cryptHeader)
	cipher.NewCTR(block, iv).XORKeyStream(plain, data[cryptHeader:])
	return plain
}

// KeyRing は、暗号化に使う鍵（今の鍵）と、復号だけに使う古い鍵の組
type KeyRing struct {
	current *EncryptionKey
	keys    []*EncryptionKey
}

// NewKeyRing は、今の鍵と古い鍵の文字列から KeyRing を作る。current が空なら暗号化はできない
func NewKeyRing(current string, old []string) (*KeyRing, error) {
	r := &KeyRing{}
	if current != "" {
		k, err := NewEncryptionKey(current)
		if err != nil {
			return nil, err
		}
		r.current = k
		r.keys = append(r.keys, k)
	}
	for _, s := range old {
		if s == "" {
			continue
		}
		k, err := NewEncryptionKey(s)
		if err != nil {
			return nil, fmt.Errorf("invalid old encryption key: %w", err)
		}
		r.keys = append(r.keys, k)
	}
	return r, nil
}

// Current は暗号化に使う鍵を返す
func (r *KeyRing) Current() (*EncryptionKey, error) {
	if r == nil || r.current == nil {
		return nil, errors.New("no encryption key (set EncryptKey or CFS_ENCRYPT_KEY)")
	}
	return r.current, nil
}

// Decrypt は、データに書かれた鍵 ID の鍵で復号する
func (r *KeyRing) Decrypt(data []byte) ([]byte, error) {
	if len(data) < cryptHeader {
		return nil, errors.New("encrypted data is too short")
	}
	if data[0] != cryptFormatVersion {
		return nil, fmt.Errorf("unsupported encryption format version %d", data[0])
	}
	id := data[1 : 1+keyIDSize]
	if r != nil {
		for _, k := range r.keys {
			if bytes.Equal(k.ID[:], id) {
				return k.decryptBody(data), nil
			}
		}
	}
	return nil, fmt.Errorf("no encryption key for key id %s (set EncryptKey / OldEncryptKeys)", hex.EncodeToString(id))
}
