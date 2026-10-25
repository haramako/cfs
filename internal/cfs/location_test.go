package cfs

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIsURL(t *testing.T) {
	for loc, expect := range map[string]bool{
		"s3://bucket":      true,
		"file:///C:/dir":   true,
		"http://localhost": true,
		"./cabinet":        false,
		"cabinet":          false,
		`C:\work\cabinet`:  false,
		"/var/cfs":         false,
	} {
		if IsURL(loc) != expect {
			t.Errorf("IsURL(%q) must be %v", loc, expect)
		}
	}
}

func TestResolveLocation(t *testing.T) {
	base := t.TempDir()

	cases := map[string]string{
		"s3://bucket":  "s3://bucket",
		"":             "",
		"cabinet":      filepath.Join(base, "cabinet"),
		"./cabinet":    filepath.Join(base, "cabinet"),
		"../cabinet":   filepath.Join(filepath.Dir(base), "cabinet"),
		"sub/cabinet/": filepath.Join(base, "sub", "cabinet"),
	}
	if runtime.GOOS == "windows" {
		vol := filepath.VolumeName(base)
		cases[`C:\work\cabinet`] = `C:\work\cabinet`
		cases["C:/work/cabinet"] = `C:\work\cabinet`
		cases[`\work\cabinet`] = vol + `\work\cabinet`
		cases["/work/cabinet"] = vol + `\work\cabinet`
		cases[`\\server\share\cabinet`] = `\\server\share\cabinet`
	} else {
		cases["/var/cfs"] = "/var/cfs"
	}

	for loc, expect := range cases {
		got, err := ResolveLocation(loc, base)
		if err != nil {
			t.Errorf("ResolveLocation(%q) failed: %v", loc, err)
			continue
		}
		if got != expect {
			t.Errorf("ResolveLocation(%q) = %q, want %q", loc, got, expect)
		}
	}
}

func TestResolveLocationDriveRelative(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	for _, loc := range []string{"C:hoge", "C:", "d:sub/dir"} {
		if _, err := ResolveLocation(loc, `D:\proj`); err == nil {
			t.Errorf("ResolveLocation(%q) must fail", loc)
		}
	}
}

func TestFileURLToPath(t *testing.T) {
	cases := map[string]string{}
	if runtime.GOOS == "windows" {
		cases["file:///C:/work/cabinet"] = `C:\work\cabinet`
		cases["file:///C:/work/cabinet/"] = `C:\work\cabinet`
		cases["file://localhost/C:/work"] = `C:\work`
		cases["file://server/share/dir"] = `\\server\share\dir`
		cases["file:///C:/My%20Docs"] = `C:\My Docs`
	} else {
		cases["file:///var/cfs"] = "/var/cfs"
		cases["file:///var/cfs/"] = "/var/cfs"
		cases["file://localhost/var/cfs"] = "/var/cfs"
		cases["file:///my%20docs"] = "/my docs"
	}

	for raw, expect := range cases {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		got, err := FileURLToPath(u)
		if err != nil {
			t.Errorf("FileURLToPath(%q) failed: %v", raw, err)
			continue
		}
		if got != expect {
			t.Errorf("FileURLToPath(%q) = %q, want %q", raw, got, expect)
		}
	}
}

func TestPathToFileURLRoundTrip(t *testing.T) {
	dir := t.TempDir()
	u := PathToFileURL(dir)
	got, err := FileURLToPath(u)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(dir) {
		t.Errorf("round trip of %q became %q (via %v)", dir, got, u)
	}
}

func TestLoadConfigEncryptKeyFromEnv(t *testing.T) {
	config := filepath.Join(t.TempDir(), ".cfsenv")
	if err := os.WriteFile(config, []byte(`{"EncryptKey": "key-in-file-123456789012345678901"}`), 0666); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CFS_ENCRYPT_KEY", "")
	t.Setenv("CFS_OLD_ENCRYPT_KEYS", "")
	t.Setenv("CFS_SIGNING_KEY", "")
	cfg, err := LoadConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EncryptKey != "key-in-file-123456789012345678901" {
		t.Errorf("EncryptKey must come from .cfsenv but %q", cfg.EncryptKey)
	}

	t.Setenv("CFS_ENCRYPT_KEY", "key-in-env-1234567890123456789012")
	t.Setenv("CFS_OLD_ENCRYPT_KEYS", "old-key-1-1234567890,old-key-2-1234567890")
	t.Setenv("CFS_SIGNING_KEY", "signing-key")
	if cfg, err = LoadConfig(config); err != nil {
		t.Fatal(err)
	}
	if cfg.EncryptKey != "key-in-env-1234567890123456789012" || len(cfg.OldEncryptKeys) != 2 || cfg.SigningKey != "signing-key" {
		t.Errorf("environment variables must take precedence but %q / %q / %q", cfg.EncryptKey, cfg.OldEncryptKeys, cfg.SigningKey)
	}

	if s := cfg.String(); strings.Contains(s, "key-in-env") || strings.Contains(s, "old-key") || strings.Contains(s, "signing-key") {
		t.Errorf("log must not contain the key: %s", s)
	}

	// .cfsenv がなくても環境変数は使われる
	if cfg, err = LoadConfig(filepath.Join(t.TempDir(), "none")); err != nil {
		t.Fatal(err)
	}
	if cfg.EncryptKey != "key-in-env-1234567890123456789012" || !cfg.Recursive || !cfg.Compress {
		t.Errorf("defaults and environment must be used without .cfsenv but %+v", cfg)
	}

	// 読めない .cfsenv は失敗にする
	os.WriteFile(config, []byte(`{broken`), 0666)
	if _, err := LoadConfig(config); err == nil {
		t.Errorf("broken .cfsenv must be an error")
	}
}

func TestConfigCacheDir(t *testing.T) {
	t.Setenv("CFS_CACHE_DIR", t.TempDir())
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "none"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CacheDir != os.Getenv("CFS_CACHE_DIR") {
		t.Errorf("CacheDir must come from CFS_CACHE_DIR but %q", cfg.CacheDir)
	}
	for _, f := range []func() (string, error){cfg.ManifestCacheDir, cfg.DataCacheDir} {
		dir, err := f()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(dir, cfg.CacheDir) {
			t.Errorf("cache must be under CFS_CACHE_DIR but %q", dir)
		}
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			t.Errorf("cache folder must be created: %v", err)
		}
	}
}

func TestLoadConfigRelativeCabinet(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "sub", ".cfsenv")
	if err := os.MkdirAll(filepath.Dir(config), 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`{"Cabinet": "../cabinet", "Url": "http://localhost/"}`), 0666); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if expect := filepath.Join(dir, "cabinet"); cfg.Cabinet != expect {
		t.Errorf("Cabinet must be %q (relative to .cfsenv) but %q", expect, cfg.Cabinet)
	}
	if cfg.Url != "http://localhost/" {
		t.Errorf("Url must not be changed but %q", cfg.Url)
	}
}
