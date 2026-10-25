package cfs

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestEncryptDeterministic(t *testing.T) {
	k, err := NewEncryptionKey("0123456789abcdef-secret")
	if err != nil {
		t.Fatal(err)
	}
	a := k.Encrypt([]byte("hello"))
	b := k.Encrypt([]byte("hello"))
	c := k.Encrypt([]byte("world"))
	if !bytes.Equal(a, b) {
		t.Errorf("same content must be encrypted to same data (for deduplication)")
	}
	if bytes.Equal(a[1+keyIDSize:cryptHeader], c[1+keyIDSize:cryptHeader]) {
		t.Errorf("different content must use different IV")
	}
	if bytes.Contains(a, []byte("hello")) {
		t.Errorf("data must be encrypted")
	}

	other, _ := NewEncryptionKey("another-secret-key-123")
	if bytes.Equal(a, other.Encrypt([]byte("hello"))) || k.ID == other.ID {
		t.Errorf("different key must make different data and id")
	}
}

func TestKeyRing(t *testing.T) {
	if _, err := NewEncryptionKey("short"); err == nil {
		t.Errorf("short key must be rejected")
	}

	oldRing, _ := NewKeyRing("old-secret-key-1234567", nil)
	oldKey, _ := oldRing.Current()
	encrypted := oldKey.Encrypt([]byte("data"))

	// 鍵を入れ替えた後も、OldEncryptKeys に入れれば読める
	ring, err := NewKeyRing("new-secret-key-1234567", []string{"old-secret-key-1234567"})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := ring.Decrypt(encrypted)
	if err != nil || string(plain) != "data" {
		t.Errorf("old key must decrypt (%q, %v)", plain, err)
	}

	// 鍵がなければ、鍵 ID 付きのエラーになる
	newOnly, _ := NewKeyRing("new-secret-key-1234567", nil)
	if _, err := newOnly.Decrypt(encrypted); err == nil || !strings.Contains(err.Error(), oldKey.IDString()) {
		t.Errorf("missing key must be reported with key id, but %v", err)
	}
}

func TestEncodeDecode(t *testing.T) {
	ring, _ := NewKeyRing("0123456789abcdef-secret", nil)
	for _, attr := range []ContentAttribute{NoContentAttribute, Compressed, Crypted, Compressed | Crypted} {
		data, err := encode([]byte("hello hello hello"), ring, attr)
		if err != nil {
			t.Fatal(err)
		}
		plain, err := decode(data, ring, attr)
		if err != nil || string(plain) != "hello hello hello" {
			t.Errorf("round trip with attr %d failed (%q, %v)", attr, plain, err)
		}
	}
}

func testManifest() *Manifest {
	m := NewManifest()
	m.Contents["a.txt"] = Content{Path: "a.txt", Hash: SumHash([]byte("x")), Size: 1, Time: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC), OrigHash: SumHash([]byte("a")), OrigSize: 1, Attr: Compressed}
	m.Contents["sub/b.ab"] = Content{Path: "sub/b.ab", Hash: SumHash([]byte("b")), Size: 1, Time: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC), OrigHash: SumHash([]byte("b")), OrigSize: 1, Attr: NoContentAttribute}
	return m
}

func TestManifestDumpParse(t *testing.T) {
	data := testManifest().Dump(nil)
	if !strings.HasPrefix(string(data), manifestHeader+"\n") {
		t.Errorf("manifest must start with header: %q", data)
	}

	m := NewManifest()
	if err := m.Parse(data, nil); err != nil {
		t.Fatal(err)
	}
	if len(m.Contents) != 2 || m.Contents["sub/b.ab"].Hash != SumHash([]byte("b")) || m.Contents["a.txt"].Attr != Compressed {
		t.Errorf("invalid parsed manifest %+v", m.Contents)
	}
	if !bytes.Equal(m.Dump(nil), data) {
		t.Errorf("dump must be stable")
	}

	// 古い形式（ヘッダーなし）や壊れた行は読まない
	if err := NewManifest().Parse([]byte("abc\tpath\t1\n"), nil); err == nil {
		t.Errorf("v1 manifest must be rejected")
	}
	if err := NewManifest().Parse([]byte(manifestHeader+"\nabc\tpath\t1\n"), nil); err == nil {
		t.Errorf("broken line must be rejected")
	}
}

func TestManifestSignature(t *testing.T) {
	private, public, err := GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ParseSigningKey(private)
	if err != nil {
		t.Fatal(err)
	}
	if signer.PublicKey() != public {
		t.Errorf("public key mismatch")
	}
	verifier, err := NewVerifier([]string{public})
	if err != nil {
		t.Fatal(err)
	}

	signed := testManifest().Dump(signer)
	if err := NewManifest().Parse(signed, verifier); err != nil {
		t.Errorf("signed manifest must be verified: %v", err)
	}

	// 署名のないマニフェストは、VerifyKeys があれば読まない
	if err := NewManifest().Parse(testManifest().Dump(nil), verifier); err == nil {
		t.Errorf("unsigned manifest must be rejected when VerifyKeys is set")
	}

	// 書き換えたら検出する
	tampered := bytes.Replace(signed, []byte("sub/b.ab"), []byte("sub/c.ab"), 1)
	if err := NewManifest().Parse(tampered, verifier); err == nil {
		t.Errorf("tampered manifest must be rejected")
	}

	// 知らない鍵の署名は読まない
	_, otherPublic, _ := GenerateSigningKey()
	otherVerifier, _ := NewVerifier([]string{otherPublic})
	if err := NewManifest().Parse(signed, otherVerifier); err == nil {
		t.Errorf("manifest signed by unknown key must be rejected")
	}

	// VerifyKeys がなければ、署名があっても読める
	if err := NewManifest().Parse(signed, nil); err != nil {
		t.Errorf("signed manifest must be readable without verifier: %v", err)
	}
}

// uploadTo は、キャビネットに内容をアップロードし、マニフェストを返す
func uploadTo(t *testing.T, s Storage, tag string, files map[string]string) *Manifest {
	t.Helper()
	return uploadWith(t, s, nil, tag, files)
}

// uploadWith は、設定を指定してアップロードする
func uploadWith(t *testing.T, s Storage, cfg *Config, tag string, files map[string]string) *Manifest {
	t.Helper()
	m := NewManifest()
	m.Tag = tag
	c := &Client{Manifest: m, Storage: s, Config: cfg}
	if err := c.Init(); err != nil {
		t.Fatal(err)
	}
	for path, content := range files {
		if _, err := c.AddContent(path, []byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Finish(); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestValidateCache(t *testing.T) {
	s, _ := NewDummyStorage("")
	m := uploadTo(t, s, "", map[string]string{"a.txt": "a"})

	if ok, err := m.ValidateCache(s); !ok || err != nil {
		t.Errorf("cache must be valid while the manifest exists (%v, %v)", ok, err)
	}

	// 前回のマニフェストが消えたら、キャッシュを使わない
	s.Delete([]string{manifestKey(m.Hash)})
	if ok, _ := m.ValidateCache(s); ok {
		t.Errorf("cache must be invalid after the manifest is deleted")
	}

	// ハッシュが分からないキャッシュも使わない
	m.Hash = ""
	if ok, _ := m.ValidateCache(s); ok {
		t.Errorf("cache without hash must be invalid")
	}
}

func TestDownloaderVerifiesHash(t *testing.T) {
	s, _ := NewDummyStorage("")
	m := uploadTo(t, s, "v1", map[string]string{"a.txt": "hello"})
	d := openTestDownloader(t, s, nil)

	loaded, err := d.LoadManifest("v1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Hash != m.Hash {
		t.Errorf("loaded manifest hash mismatch")
	}

	// 中身を書き換えると、ハッシュが合わずに失敗する
	c := loaded.Contents["a.txt"]
	s.Put(dataKey(c.Hash), []byte("tampered"), true)
	if _, err := d.Fetch(c.Hash, c.Attr); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Errorf("tampered data must be rejected, but %v", err)
	}
}

func TestSignedManifestRoundTrip(t *testing.T) {
	private, public, _ := GenerateSigningKey()
	signing := DefaultConfig()
	signing.SigningKey = private
	verifying := testConfig(t)
	verifying.VerifyKeys = []string{public}

	s, _ := NewDummyStorage("")
	uploadWith(t, s, signing, "v1", map[string]string{"a.txt": "hello"})
	d := openTestDownloader(t, s, verifying)
	if _, err := d.LoadManifest("v1"); err != nil {
		t.Errorf("signed manifest must be loaded: %v", err)
	}

	// 署名しないでアップロードしたものは、VerifyKeys があると読めない
	uploadTo(t, s, "v2", map[string]string{"a.txt": "unsigned"})
	if _, err := d.LoadManifest("v2"); err == nil {
		t.Errorf("unsigned manifest must be rejected")
	}
}

func TestValidateTagName(t *testing.T) {
	for _, ok := range []string{"v1", "report-123", "a.b_c-D"} {
		if err := ValidateTagName(ok); err != nil {
			t.Errorf("%q must be valid: %v", ok, err)
		}
	}
	for _, ng := range []string{"", ".", "..", "feature/foo", "日本語", "a b"} {
		if err := ValidateTagName(ng); err == nil {
			t.Errorf("%q must be invalid", ng)
		}
	}
}

func TestParseDuration(t *testing.T) {
	for s, expect := range map[string]time.Duration{"30d": 30 * 24 * time.Hour, "0d": 0, "72h": 72 * time.Hour} {
		d, err := ParseDuration(s)
		if err != nil || d != expect {
			t.Errorf("ParseDuration(%q) = %v, %v", s, d, err)
		}
	}
	for _, ng := range []string{"", "x", "-1d", "1y"} {
		if _, err := ParseDuration(ng); err == nil {
			t.Errorf("ParseDuration(%q) must fail", ng)
		}
	}
}
