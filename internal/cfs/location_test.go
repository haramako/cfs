package cfs

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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

func TestLoadDefaultOptionsRelativeCabinet(t *testing.T) {
	saved := *Option
	defer func() { *Option = saved }()

	dir := t.TempDir()
	config := filepath.Join(dir, "sub", ".cfsenv")
	if err := os.MkdirAll(filepath.Dir(config), 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte(`{"Cabinet": "../cabinet", "Url": "http://localhost/"}`), 0666); err != nil {
		t.Fatal(err)
	}

	if err := LoadDefaultOptions(config); err != nil {
		t.Fatal(err)
	}
	if expect := filepath.Join(dir, "cabinet"); Option.Cabinet != expect {
		t.Errorf("Cabinet must be %q (relative to .cfsenv) but %q", expect, Option.Cabinet)
	}
	if Option.Url != "http://localhost/" {
		t.Errorf("Url must not be changed but %q", Option.Url)
	}
}
