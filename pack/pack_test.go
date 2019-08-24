package pack

import (
	"bytes"
	"testing"
)

func TestPack(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef"
	entries := []Entry{
		{path: "hoge", hash: hash, pos: 0, size: 1},
		{path: "fugafuga", hash: hash, pos: 0, size: 100},
		{path: "piyo", hash: hash, pos: 0, size: 0},
	}
	bin, err := encodePackEntryList(entries)
	if err != nil {
		t.Error(err)
	}

	r := bytes.NewBuffer(bin)
	pack, err := Unpack(r)
	if err != nil {
		t.Error(err)
	}

	for i, entry := range pack.Entries {
		if entry != entries[i] {
			t.Errorf("not same entry %v %v", entry, entries[i])
			return
		}
	}
}
