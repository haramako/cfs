package main

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/haramako/cfs"
	"github.com/haramako/cfs/pack"
)

func filterBucket(cmd string, b *cfs.Bucket) (*cfs.Bucket, error) {
	if cmd == "" {
		return b, nil
	}

	entries := b.Contents
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		files = append(files, e.Path)
	}
	files, err := runFilter(cmd, files)
	check(err)

	fileDict := make(map[string]bool, len(entries))
	for _, f := range files {
		fileDict[f] = true
	}

	newEntries := make(map[string]cfs.Content, len(entries))
	for _, e := range entries {
		_, ok := fileDict[e.Path]
		if ok {
			newEntries[e.Path] = e
		}
	}
	entries = newEntries

	return &cfs.Bucket{
		HashType: "md5",
		Contents: entries,
	}, nil
}

func filterPackFile(cmd string, pak *pack.PackFile) (*pack.PackFile, error) {
	entries := pak.Entries
	if cmd == "" {
		return pak, nil
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		files = append(files, e.Path)
	}
	files, err := runFilter(cmd, files)
	check(err)

	fileDict := make(map[string]bool, len(entries))
	for _, f := range files {
		fileDict[f] = true
	}

	newEntries := make([]pack.Entry, 0, len(entries))
	for _, e := range entries {
		_, ok := fileDict[e.Path]
		if ok {
			newEntries = append(newEntries, e)
		}
	}
	entries = newEntries

	return &pack.PackFile{
		Version: pack.PackFileVersion,
		Entries: entries,
	}, nil
}

func runFilter(cmdStr string, files []string) ([]string, error) {
	out, err := runCommand(cmdStr, strings.Join(files, "\n"))
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(out, "\n"), "\n"), nil
}

func runCommand(cmdStr string, input string) (string, error) {
	commands := strings.Split(cmdStr, " ")
	cmd := exec.Command(commands[0], commands[1:]...)

	stdin, err := cmd.StdinPipe()
	check(err)

	go func() {
		_, err = io.Copy(stdin, bytes.NewBufferString(input))
		check(err)
		err = stdin.Close()
		check(err)
	}()

	stdout, err := cmd.StdoutPipe()
	check(err)

	outbuf := bytes.NewBuffer(nil)
	go func() {
		_, err = io.Copy(outbuf, stdout)
		check(err)
		err = stdout.Close()
		check(err)
	}()

	err = cmd.Start()
	check(err)

	err = cmd.Wait()
	out := outbuf.String()
	if err != nil {
		if out == "" {
			return "", fmt.Errorf("no output from filter")
		}
		return "", err
	}

	return out, nil
}
