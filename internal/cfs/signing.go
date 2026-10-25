package cfs

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

// マニフェストの署名（形式 v2）
//
// アップロードする側が Ed25519 の秘密鍵（環境変数 CFS_SIGNING_KEY）で署名し、
// ダウンロードする側は .cfsenv の VerifyKeys（公開鍵）で確かめる。
// マニフェストが正しければ、中身はハッシュで照合できるので、全体の改ざんを検出できる。

// SigningKey は、マニフェストに署名する秘密鍵
type SigningKey struct {
	private ed25519.PrivateKey
}

// ParseSigningKey は、base64 の秘密鍵（32 バイトのシードか、64 バイトの秘密鍵）を読む
func ParseSigningKey(s string) (*SigningKey, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid signing key: %w", err)
	}
	switch len(b) {
	case ed25519.SeedSize:
		return &SigningKey{private: ed25519.NewKeyFromSeed(b)}, nil
	case ed25519.PrivateKeySize:
		return &SigningKey{private: ed25519.PrivateKey(b)}, nil
	default:
		return nil, fmt.Errorf("invalid signing key length %d", len(b))
	}
}

// GenerateSigningKey は、新しい鍵の組を作り、秘密鍵（シード）と公開鍵を base64 で返す
func GenerateSigningKey() (privateKey string, publicKey string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(priv.Seed()), base64.StdEncoding.EncodeToString(pub), nil
}

// PublicKey は、検証用の公開鍵を base64 で返す
func (k *SigningKey) PublicKey() string {
	return base64.StdEncoding.EncodeToString(k.private.Public().(ed25519.PublicKey))
}

// KeyID は、公開鍵から決まる鍵 ID を返す
func (k *SigningKey) KeyID() string {
	return publicKeyID(k.private.Public().(ed25519.PublicKey))
}

func publicKeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:8])
}

// Sign はデータに署名し、署名を base64 で返す
func (k *SigningKey) Sign(data []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(k.private, data))
}

// Verifier は、マニフェストの署名を確かめる公開鍵の組
type Verifier struct {
	keys map[string]ed25519.PublicKey // 鍵 ID → 公開鍵
}

// NewVerifier は、base64 の公開鍵の一覧から Verifier を作る
func NewVerifier(publicKeys []string) (*Verifier, error) {
	v := &Verifier{keys: map[string]ed25519.PublicKey{}}
	for _, s := range publicKeys {
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil || len(b) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid verify key %q", s)
		}
		pub := ed25519.PublicKey(b)
		v.keys[publicKeyID(pub)] = pub
	}
	return v, nil
}

// Enabled は、確かめる鍵が設定されているかを返す
func (v *Verifier) Enabled() bool {
	return v != nil && len(v.keys) > 0
}

// Verify は、鍵 ID の公開鍵で署名を確かめる
func (v *Verifier) Verify(data []byte, keyID string, signature string) error {
	pub, ok := v.keys[keyID]
	if !ok {
		return fmt.Errorf("manifest is signed by unknown key %s", keyID)
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("invalid manifest signature: %w", err)
	}
	if !ed25519.Verify(pub, data, sig) {
		return errors.New("invalid manifest signature")
	}
	return nil
}
