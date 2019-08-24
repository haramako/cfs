package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/urfave/cli"
)

var packCommand = cli.Command{
	Name:      "pack",
	Usage:     "pack specified dir",
	Action:    doPack,
	ArgsUsage: "packfile.cfspack dir [...]",
}

func runFilter(cmdStr string, files []string) ([]string, error) {
	out, err := runFilterCommand(cmdStr, strings.Join(files, "\n"))
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimRight(out, "\n"), "\n"), nil
}

func runFilterCommand(cmdStr string, in string) (string, error) {
	commands := strings.Split(cmdStr, " ")
	cmd := exec.Command(commands[0], commands[1:]...)

	stdin, err := cmd.StdinPipe()
	check(err)

	go func() {
		_, err = io.Copy(stdin, bytes.NewBufferString(in))
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
		} else {
			return "", err
		}
	}

	return out, nil
}

func doPack(c *cli.Context) {
	loadConfig(c)

	var args = c.Args()
	if len(args) < 2 {
		fmt.Println("need at least 2 arguments")
		os.Exit(1)
	}

	filter := c.GlobalString("filter")
	if filter != "" {
	}

	/*
		packfile := args[0]

		dirs := args[1:]

		for _, dir := range dirs {

		}
	*/
}
