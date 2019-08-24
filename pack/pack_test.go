package pack

import (
	"bytes"
	"testing"
)

func TestPack(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef"
	entries := []Entry{
		{Path: "hoge", Hash: hash, Size: 1},
		{Path: "fugafuga", Hash: hash, Size: 100},
		{Path: "piyo", Hash: hash, Size: 0},
	}
	w := bytes.NewBuffer(nil)
	origPack := NewPackFile(entries)
	err := Write(w, origPack)
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

	for i, entry := range pack.Entries {
		if entry != entries[i] {
			t.Errorf("not same entry %v %v", entry, entries[i])
			return
		}
	}
}
