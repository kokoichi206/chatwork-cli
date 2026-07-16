// releaseguard はリリースタグの検証を CI で実行する入口。
// タグ検証は main と全タグの参照が手元にあることが前提(release workflow が fetch する)。
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kokoichi206/chatwork-cli/internal/releaseguard"
)

func main() {
	tag := flag.String("tag", os.Getenv("GITHUB_REF_NAME"), "release tag name")
	base := flag.String("base", "origin/main", "release base ref")
	flag.Parse()

	if *tag == "" {
		fmt.Fprintln(os.Stderr, "release tag is required")
		os.Exit(1)
	}

	tagCommit, err := git("rev-list", "-n", "1", *tag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	baseCommit, err := git("rev-parse", "--verify", *base+"^{commit}")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	tagOutput, err := git("tag", "--list", "v*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := releaseguard.Validate(*tag, tagCommit, baseCommit, strings.Fields(tagOutput)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}
