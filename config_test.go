package cfs

import (
	"testing"
)

func TestHomeDir(t *testing.T) {
	home := HomeDir()
	if home == "" {
		t.Errorf("cannot get HomeDir")
	}
}

func TestGlobalCacheDir(t *testing.T) {
	cache := GlobalCacheDir()
	if cache == "" {
		t.Errorf("cannot get CacheDir")
	}
}
