package cfs

import (
	"bytes"
	"compress/zlib"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"io"
	"io/ioutil"
	"time"
)

type TagFile struct {
	Name       string           `json:"name"`
	CreatedAt  time.Time        `json:"createdAt"`
	EncryptKey string           `json:"encryptKey"`
	EncryptIv  string           `json:"encryptIv"`
	Attr       ContentAttribute `json:"attr"`
	Hash       string           `json:"hash"`
}

func TagFileFromReader(r io.Reader) (*TagFile, error) {
	tagBytes, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var tag TagFile
	err = json.Unmarshal(tagBytes, &tag)
	if err != nil {
		return nil, err
	}

	return &tag, nil
}

func TagFileFromFile(tag_filepath string) (*TagFile, error) {
	tagBytes, err := ioutil.ReadFile(tag_filepath)
	if err != nil {
		return nil, err
	}

	var tag TagFile
	err = json.Unmarshal(tagBytes, &tag)
	if err != nil {
		return nil, err
	}

	return &tag, nil
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

func (t *TagFile) Bucket(sv *Server) (*Bucket, error) {
	bucket := Bucket{
		Contents: make(map[string]Content),
		HashType: "md5",
	}

	data, err := ioutil.ReadFile(sv.hashFilepath(t.Hash))
	if err != nil {
		return nil, err
	}

	data, err = decode(data, t.EncryptKey, t.EncryptIv, t.Attr)
	if err != nil {
		return nil, err
	}

	println("hoge", t.EncryptKey, t.EncryptIv, t.Attr, string(data))

	err = bucket.Parse(data)
	if err != nil {
		return nil, err
	}

	return &bucket, nil
}
