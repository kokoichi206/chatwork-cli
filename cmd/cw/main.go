package main

import (
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/kokoichi206/chatwork-cli/internal/cli"
)

// version は goreleaser の ldflags で上書きされる。
var version = "dev"

func main() {
	root := cli.New(cli.Deps{
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Stdin:       os.Stdin,
		StdoutIsTTY: term.IsTerminal(int(os.Stdout.Fd())),
		StdinIsTTY:  term.IsTerminal(int(os.Stdin.Fd())),
		Getenv:      os.Getenv,
		Version:     version,
		ReadPassword: func() ([]byte, error) {
			return term.ReadPassword(int(os.Stdin.Fd()))
		},
	})
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(cli.ExitCode(err))
	}
}
