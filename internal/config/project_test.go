package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kokoichi206/chatwork-cli/internal/config"
)

func writeProject(t *testing.T, dir, content string) {
	t.Helper()
	confDir := filepath.Join(dir, ".config")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(confDir, config.ProjectConfigName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindProjectInCurrentDir(t *testing.T) {
	dir := t.TempDir()
	writeProject(t, dir, `{"rooms":{"main":405354226,"alerts":123}}`)

	proj, err := config.FindProject(dir, "")
	if err != nil {
		t.Fatalf("FindProject() error = %v", err)
	}
	if proj == nil {
		t.Fatal("project not found")
	}
	if proj.Rooms["main"] != 405354226 || proj.Rooms["alerts"] != 123 {
		t.Errorf("project = %+v", proj)
	}
	if proj.Dir != dir {
		t.Errorf("Dir = %q, want %q", proj.Dir, dir)
	}
	if got := proj.RoomAliases(); len(got) != 2 || got[0] != "alerts" || got[1] != "main" {
		t.Errorf("RoomAliases() = %v", got)
	}
}

func TestFindProjectWalksUpToParent(t *testing.T) {
	root := t.TempDir()
	writeProject(t, root, `{"rooms":{"main":1}}`)
	nested := filepath.Join(root, "sub", "dir")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	proj, err := config.FindProject(nested, "")
	if err != nil {
		t.Fatal(err)
	}
	if proj == nil || proj.Dir != root {
		t.Errorf("project = %+v, want found at %s", proj, root)
	}
}

func TestFindProjectStopsAtHome(t *testing.T) {
	home := t.TempDir()
	// HOME 直下の .config はユーザースコープであり、プロジェクト設定としては読まない。
	writeProject(t, home, `{"rooms":{"main":1}}`)
	workDir := filepath.Join(home, "repo")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}

	proj, err := config.FindProject(workDir, home)
	if err != nil {
		t.Fatal(err)
	}
	if proj != nil {
		t.Errorf("project = %+v, want nil (must stop at home)", proj)
	}
}

func TestFindProjectMissingReturnsNil(t *testing.T) {
	proj, err := config.FindProject(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if proj != nil {
		t.Errorf("project = %+v, want nil", proj)
	}
}

func TestFindProjectInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	writeProject(t, dir, `{broken`)

	if _, err := config.FindProject(dir, ""); err == nil {
		t.Error("want parse error")
	}
}
