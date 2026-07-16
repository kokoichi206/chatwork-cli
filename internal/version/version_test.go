package version_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/kokoichi206/chatwork-cli/internal/version"
)

func TestString(t *testing.T) {
	got := version.String()
	for _, want := range []string{version.Version, version.Commit, version.Date, runtime.Version()} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, want it to contain %q", got, want)
		}
	}
}
