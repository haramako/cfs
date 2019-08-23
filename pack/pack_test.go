package pack

import "testing"

func TestPack(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef"
	entries := []packEntry{
		{path: "hoge", hash: hash, pos: 0, size: 1},
		{path: "fugafuga", hash: hash, pos: 0, size: 100},
		{path: "piyo", hash: hash, pos: 0, size: 0},
	}
	bin, err := encodePackEntryList(entries)
	if err != nil {
		t.Error(err)
	}

	entries2, err := decodePackEntryList(bin, len(entries))
	if err != nil {
		t.Error(err)
	}

	for i, entry := range entries {
		if entry != entries2[i] {
			t.Errorf("not same entry %v %v", entry, entries2[i])
			return
		}
	}
}
