package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/haramako/cfs/internal/cfs"
	"github.com/haramako/cfs/internal/pack"
)

// --filter-cmd は、パスの一覧（1 行 1 パス）を標準入力で受け取り、残すパスを標準出力に出すコマンド

// filterManifest は、マニフェストのファイルを、フィルターのコマンドが残したものだけにする
func filterManifest(cmd string, m *cfs.Manifest) (*cfs.Manifest, error) {
	if cmd == "" {
		return m, nil
	}
	paths := make([]string, 0, len(m.Contents))
	for path := range m.Contents {
		paths = append(paths, path)
	}
	keep, err := filterPaths(cmd, paths)
	if err != nil {
		return nil, err
	}

	filtered := cfs.NewManifest()
	filtered.Tag = m.Tag
	for path, c := range m.Contents {
		if keep[path] {
			filtered.Contents[path] = c
		}
	}
	return filtered, nil
}

// filterPackFile は、パックのファイルを、フィルターのコマンドが残したものだけにする
func filterPackFile(cmd string, pak *pack.PackFile) (*pack.PackFile, error) {
	if cmd == "" {
		return pak, nil
	}
	paths := make([]string, 0, len(pak.Entries))
	for _, e := range pak.Entries {
		paths = append(paths, e.Path)
	}
	keep, err := filterPaths(cmd, paths)
	if err != nil {
		return nil, err
	}

	entries := make([]pack.Entry, 0, len(pak.Entries))
	for _, e := range pak.Entries {
		if keep[e.Path] {
			entries = append(entries, e)
		}
	}
	return pack.NewPackFile(entries), nil
}

// filterPaths は、フィルターのコマンドにパスの一覧を渡し、残すパスの集合を返す
func filterPaths(cmd string, paths []string) (map[string]bool, error) {
	args, err := splitCommand(cmd)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("empty filter command")
	}

	var stdout bytes.Buffer
	c := exec.Command(args[0], args[1:]...)
	c.Stdin = strings.NewReader(strings.Join(paths, "\n"))
	c.Stdout = &stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("filter command %q failed: %w", cmd, err)
	}

	keep := map[string]bool{}
	for _, line := range strings.FieldsFunc(stdout.String(), func(r rune) bool { return r == '\n' || r == '\r' }) {
		keep[line] = true
	}
	return keep, nil
}

// splitCommand は、コマンドの文字列を空白で区切る。"..." や '...' で囲めば、空白を含められる
func splitCommand(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inArg := false
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
			inArg = true
		case r == ' ' || r == '\t':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unclosed quote in command: %s", s)
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args, nil
}
