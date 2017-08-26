package cfs

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"io"
	"os"
)

func isWindows() bool {
	return os.PathSeparator == '\\' && os.PathListSeparator == ';'
}

func isHash(str string) bool {
	if len(str) != 32 {
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

func encode(origData []byte, encrypt_key string, encrypt_iv string, attr ContentAttribute) ([]byte, bool, error) {
	data := origData
	hash_changed := false

	if attr.Compressed() {
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		w.Write(origData)
		w.Close()
		data = buf.Bytes()
		hash_changed = true
	}

	if attr.Crypted() {
		block, err := aes.NewCipher([]byte(Option.EncryptKey))
		if err != nil {
			return nil, false, err
		}
		cfb := cipher.NewCFBEncrypter(block, []byte(Option.EncryptIv))
		cipher_data := make([]byte, len(data))
		cfb.XORKeyStream(cipher_data, data)
		data = cipher_data
		hash_changed = true
	}

	return data, hash_changed, nil
}

func decode(data []byte, encrypt_key string, encrypt_iv string, attr ContentAttribute) ([]byte, error) {

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

	if attr.Crypted() {
		block, err := aes.NewCipher([]byte(encrypt_key))
		if err != nil {
			return nil, err
		}
		cfb := cipher.NewCFBEncrypter(block, []byte(encrypt_iv))
		cipher_data := make([]byte, len(data))
		cfb.XORKeyStream(cipher_data, data)
		data = cipher_data
	}

	return data, nil
}
