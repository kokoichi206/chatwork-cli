package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAssetName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		goos    string
		goarch  string
		want    string
	}{
		{
			name:    "macOS arm64",
			version: "0.1.0",
			goos:    "darwin",
			goarch:  "arm64",
			want:    "chatwork-cli_0.1.0_darwin_arm64.tar.gz",
		},
		{
			name:    "Linux amd64",
			version: "0.1.0",
			goos:    "linux",
			goarch:  "amd64",
			want:    "chatwork-cli_0.1.0_linux_amd64.tar.gz",
		},
		{
			name:    "Windows amd64",
			version: "0.1.0",
			goos:    "windows",
			goarch:  "amd64",
			want:    "chatwork-cli_0.1.0_windows_amd64.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := AssetName(tt.version, tt.goos, tt.goarch)
			if got != tt.want {
				t.Fatalf("AssetName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func tarGzWithBinary(t *testing.T, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: binaryName, Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func releaseServer(t *testing.T, tag string, assetContent []byte) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	assetName := AssetName(tag, "linux", "amd64")
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		release := githubRelease{
			TagName: "v" + tag,
			HTMLURL: server.URL + "/release",
			Assets: []githubAsset{
				{Name: assetName, DownloadURL: server.URL + "/asset"},
			},
		}
		if err := json.NewEncoder(w).Encode(release); err != nil {
			t.Error(err)
		}
	})
	mux.HandleFunc("/asset", func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write(assetContent); err != nil {
			t.Error(err)
		}
	})
	return server
}

func TestRunReplacesBinaryWhenNewerVersionExists(t *testing.T) {
	t.Parallel()

	newBinary := []byte("new binary")
	server := releaseServer(t, "0.2.0", tarGzWithBinary(t, newBinary))

	executable := filepath.Join(t.TempDir(), "cw")
	if err := os.WriteFile(executable, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), server.Client(), Options{
		CurrentVersion: "0.1.0",
		ExecutablePath: executable,
		GOOS:           "linux",
		GOARCH:         "amd64",
		releasesAPI:    server.URL + "/releases/latest",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated {
		t.Fatalf("Updated = false, want true: %+v", result)
	}
	if result.LatestVersion != "0.2.0" {
		t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, "0.2.0")
	}
	got, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newBinary) {
		t.Fatalf("executable content = %q, want %q", got, newBinary)
	}
}

func TestRunDryRunDoesNotTouchBinary(t *testing.T) {
	t.Parallel()

	server := releaseServer(t, "0.2.0", tarGzWithBinary(t, []byte("new binary")))

	executable := filepath.Join(t.TempDir(), "cw")
	oldContent := []byte("old binary")
	if err := os.WriteFile(executable, oldContent, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), server.Client(), Options{
		CurrentVersion: "0.1.0",
		ExecutablePath: executable,
		GOOS:           "linux",
		GOARCH:         "amd64",
		DryRun:         true,
		releasesAPI:    server.URL + "/releases/latest",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated {
		t.Fatalf("Updated = true, want false: %+v", result)
	}
	if result.AssetName == "" {
		t.Fatal("AssetName is empty, want the selected asset")
	}
	got, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, oldContent) {
		t.Fatalf("executable content = %q, want unchanged %q", got, oldContent)
	}
}

func TestRunSkipsWhenAlreadyLatest(t *testing.T) {
	t.Parallel()

	server := releaseServer(t, "0.1.0", nil)

	executable := filepath.Join(t.TempDir(), "cw")
	if err := os.WriteFile(executable, []byte("current binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := Run(context.Background(), server.Client(), Options{
		CurrentVersion: "v0.1.0",
		ExecutablePath: executable,
		GOOS:           "linux",
		GOARCH:         "amd64",
		releasesAPI:    server.URL + "/releases/latest",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated {
		t.Fatalf("Updated = true, want false: %+v", result)
	}
}

func TestRunFailsWhenAssetMissing(t *testing.T) {
	t.Parallel()

	server := releaseServer(t, "0.2.0", nil)

	executable := filepath.Join(t.TempDir(), "cw")
	if err := os.WriteFile(executable, []byte("current binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := Run(context.Background(), server.Client(), Options{
		CurrentVersion: "0.1.0",
		ExecutablePath: executable,
		GOOS:           "darwin",
		GOARCH:         "arm64",
		releasesAPI:    server.URL + "/releases/latest",
	})
	if err == nil {
		t.Fatal("Run() returned nil error, want asset-not-found error")
	}
	want := fmt.Sprintf("release v0.2.0 does not contain asset %s", AssetName("0.2.0", "darwin", "arm64"))
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}
