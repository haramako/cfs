package pack

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestPack(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	entries := []Entry{
		{Path: "hoge", Hash: hash, Size: 4},
		{Path: "fugafuga", Hash: hash, Size: 8},
		{Path: "piyo", Hash: hash, Size: 4},
	}
	w := bytes.NewBuffer(nil)
	origPack := NewPackFile(entries)
	err := Pack(w, origPack, func(s string) io.Reader { return bytes.NewBufferString(s) })
	if err != nil {
		t.Error(err)
		return
	}

	bin := w.Bytes()

	r := bytes.NewBuffer(bin)
	pack, err := Parse(r)
	if err != nil {
		t.Error(err)
		return
	}

	for i, e := range pack.Entries {
		e2 := entries[i]
		if e.Path != e2.Path || e.Size != e2.Size || e.Hash != e2.Hash || e.Pos != e2.Pos {
			t.Errorf("not same entry %v %v", e, e2)
			return
		}
	}
}

func entryPaths(p *PackFile) string {
	var paths []string
	for _, e := range p.Entries {
		paths = append(paths, e.Path)
	}
	sort.Strings(paths)
	return strings.Join(paths, ",")
}

func TestNewPackFileFromDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sub"), 0777)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0666)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("bb"), 0666)
	t.Chdir(dir)

	// "." や末尾に区切りのあるパスでも、フォルダからの相対パスになる
	for _, d := range []string{dir, dir + string(filepath.Separator), ".", "./", "sub/.."} {
		p, err := NewPackFileFromDir(d)
		if err != nil {
			t.Fatal(err)
		}
		if got := entryPaths(p); got != "a.txt,sub/b.txt" {
			t.Errorf("NewPackFileFromDir(%q) = %s", d, got)
		}
		for _, e := range p.Entries {
			if e.Hash != fmt.Sprintf("%x", sha256.Sum256(e.Data)) || e.Size != len(e.Data) {
				t.Errorf("invalid entry %+v", e)
			}
		}
	}

	if _, err := NewPackFileFromDir(filepath.Join(dir, "none")); err == nil {
		t.Errorf("missing folder must be an error")
	}
}

func TestPatch(t *testing.T) {
	base := NewPackFile([]Entry{
		{Path: "same", Hash: "h1"},
		{Path: "changed", Hash: "h2"},
		{Path: "removed", Hash: "h3"},
	})
	current := NewPackFile([]Entry{
		{Path: "same", Hash: "h1"},
		{Path: "changed", Hash: "h2-new"},
		{Path: "added", Hash: "h4"},
	})
	patch, err := Patch(base, current)
	if err != nil {
		t.Fatal(err)
	}
	// 中身は変わったものと増えたものだけ。消えたものは Deleted に入る
	if got := entryPaths(patch); got != "added,changed" {
		t.Errorf("patch = %s", got)
	}
	if got := strings.Join(patch.Deleted, ","); got != "removed" {
		t.Errorf("deleted = %s", got)
	}
}

func TestPackDeletedRoundTrip(t *testing.T) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte("a")))
	pak := NewPackFile([]Entry{{Path: "a.txt", Hash: hash, Size: 1, Data: []byte("a")}})
	pak.Deleted = []string{"z/old.txt", "b.txt"}

	var buf bytes.Buffer
	if err := Pack(&buf, pak, nil); err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(parsed.Deleted, ","); got != "b.txt,z/old.txt" {
		t.Errorf("deleted must be kept in path order: %s", got)
	}
	if len(parsed.Entries) != 1 || string(parsed.Entries[0].Data) != "a" {
		t.Errorf("entries must be kept: %+v", parsed.Entries)
	}
}

func TestPackRejectsLongPathAndOtherVersion(t *testing.T) {
	long := strings.Repeat("a", 256)
	if err := Pack(io.Discard, NewPackFile([]Entry{{Path: long, Hash: strings.Repeat("0", 64), Data: []byte{}}}), nil); err == nil {
		t.Errorf("path over 255 bytes must be an error")
	}
	pak := NewPackFile(nil)
	pak.Deleted = []string{long}
	if err := Pack(io.Discard, pak, nil); err == nil {
		t.Errorf("deleted path over 255 bytes must be an error")
	}

	// 版が違うものは読まない
	if _, err := Parse(bytes.NewReader([]byte{'T', 'P', 2, 0, 0, 0, 0})); err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("other version must be rejected, but %v", err)
	}
}
