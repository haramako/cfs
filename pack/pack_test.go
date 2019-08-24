package pack

import (
	"bytes"
	"io"
	"testing"
)

func TestPack(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef"
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
