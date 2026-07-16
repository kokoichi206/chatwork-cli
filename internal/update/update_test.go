package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestIsNewer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		latest  string
		current string
		want    bool
		wantErr bool
	}{
		{name: "newer patch", latest: "0.1.1", current: "0.1.0", want: true},
		{name: "same version", latest: "0.1.0", current: "0.1.0", want: false},
		{name: "current is newer", latest: "0.1.0", current: "0.2.0", want: false},
		{name: "numeric compare not lexicographic", latest: "0.10.0", current: "0.9.0", want: true},
		{name: "dev build always updates", latest: "0.1.0", current: "dev", want: true},
		{name: "invalid latest", latest: "nightly", current: "0.1.0", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := isNewer(tt.latest, tt.current)
			if tt.wantErr {
				if err == nil {
					t.Fatal("isNewer() returned nil error, want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("isNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

type tarEntry struct {
	name     string
	typeflag byte
	content  []byte
}

func tarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		header := &tar.Header{Name: e.name, Typeflag: e.typeflag, Mode: 0o755, Size: int64(len(e.content))}
		if e.typeflag == tar.TypeSymlink {
			header.Linkname = "/etc/passwd"
			header.Size = 0
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(e.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tarGzWithBinary(t *testing.T, content []byte) []byte {
	t.Helper()
	return tarGz(t, []tarEntry{{name: binaryName, typeflag: tar.TypeReg, content: content}})
}

type serverOptions struct {
	checksumOverride string
	omitChecksums    bool
}

func releaseServer(t *testing.T, tag string, assetContent []byte, opts serverOptions) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	assetName := AssetName(tag, "linux", "amd64")
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		assets := []githubAsset{
			{Name: assetName, DownloadURL: server.URL + "/asset"},
		}
		if !opts.omitChecksums {
			assets = append(assets, githubAsset{Name: checksumsAssetName, DownloadURL: server.URL + "/checksums"})
		}
		release := githubRelease{
			TagName: "v" + tag,
			HTMLURL: server.URL + "/release",
			Assets:  assets,
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
	mux.HandleFunc("/checksums", func(w http.ResponseWriter, _ *http.Request) {
		sum := opts.checksumOverride
		if sum == "" {
			hash := sha256.Sum256(assetContent)
			sum = hex.EncodeToString(hash[:])
		}
		fmt.Fprintf(w, "%s  %s\n", sum, assetName)
	})
	return server
}

func runOptions(server *httptest.Server, executable string) Options {
	return Options{
		CurrentVersion: "0.1.0",
		ExecutablePath: executable,
		GOOS:           "linux",
		GOARCH:         "amd64",
		releasesAPI:    server.URL + "/releases/latest",
	}
}

func writeExecutable(t *testing.T, content []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "cw")
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunReplacesBinaryWhenNewerVersionExists(t *testing.T) {
	t.Parallel()

	newBinary := []byte("new binary")
	server := releaseServer(t, "0.2.0", tarGzWithBinary(t, newBinary), serverOptions{})
	executable := writeExecutable(t, []byte("old binary"))

	result, err := Run(context.Background(), server.Client(), runOptions(server, executable))
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
	entries, err := os.ReadDir(filepath.Dir(executable))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("leftover files in executable dir: %v", entries)
	}
}

func TestRunSkipsNonRegularTarEntries(t *testing.T) {
	t.Parallel()

	newBinary := []byte("new binary")
	archive := tarGz(t, []tarEntry{
		{name: "dir/" + binaryName, typeflag: tar.TypeDir},
		{name: binaryName, typeflag: tar.TypeSymlink},
		{name: "bin/" + binaryName, typeflag: tar.TypeReg, content: newBinary},
	})
	server := releaseServer(t, "0.2.0", archive, serverOptions{})
	executable := writeExecutable(t, []byte("old binary"))

	result, err := Run(context.Background(), server.Client(), runOptions(server, executable))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Updated {
		t.Fatalf("Updated = false, want true: %+v", result)
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

	server := releaseServer(t, "0.2.0", tarGzWithBinary(t, []byte("new binary")), serverOptions{})
	oldContent := []byte("old binary")
	executable := writeExecutable(t, oldContent)

	options := runOptions(server, executable)
	options.DryRun = true
	result, err := Run(context.Background(), server.Client(), options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated {
		t.Fatalf("Updated = true, want false: %+v", result)
	}
	if !result.UpdateAvailable {
		t.Fatalf("UpdateAvailable = false, want true: %+v", result)
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

	server := releaseServer(t, "0.1.0", nil, serverOptions{})
	executable := writeExecutable(t, []byte("current binary"))

	options := runOptions(server, executable)
	options.CurrentVersion = "v0.1.0"
	result, err := Run(context.Background(), server.Client(), options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated || result.UpdateAvailable {
		t.Fatalf("no update expected: %+v", result)
	}
}

func TestRunDoesNotDowngrade(t *testing.T) {
	t.Parallel()

	server := releaseServer(t, "0.1.0", tarGzWithBinary(t, []byte("older binary")), serverOptions{})
	currentContent := []byte("current binary")
	executable := writeExecutable(t, currentContent)

	options := runOptions(server, executable)
	options.CurrentVersion = "0.2.0"
	result, err := Run(context.Background(), server.Client(), options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Updated || result.UpdateAvailable {
		t.Fatalf("downgrade must not happen: %+v", result)
	}
	got, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, currentContent) {
		t.Fatalf("executable content = %q, want unchanged %q", got, currentContent)
	}
}

func TestRunFailsOnChecksumMismatch(t *testing.T) {
	t.Parallel()

	server := releaseServer(t, "0.2.0", tarGzWithBinary(t, []byte("new binary")), serverOptions{
		checksumOverride: strings.Repeat("0", 64),
	})
	oldContent := []byte("old binary")
	executable := writeExecutable(t, oldContent)

	_, err := Run(context.Background(), server.Client(), runOptions(server, executable))
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v, want checksum mismatch", err)
	}
	got, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, oldContent) {
		t.Fatalf("executable content = %q, want unchanged %q", got, oldContent)
	}
}

func TestRunFailsWhenChecksumsAssetMissing(t *testing.T) {
	t.Parallel()

	server := releaseServer(t, "0.2.0", tarGzWithBinary(t, []byte("new binary")), serverOptions{
		omitChecksums: true,
	})
	executable := writeExecutable(t, []byte("old binary"))

	_, err := Run(context.Background(), server.Client(), runOptions(server, executable))
	if err == nil || !strings.Contains(err.Error(), checksumsAssetName) {
		t.Fatalf("error = %v, want missing %s error", err, checksumsAssetName)
	}
}

func TestRunFailsWhenAssetMissing(t *testing.T) {
	t.Parallel()

	server := releaseServer(t, "0.2.0", nil, serverOptions{})
	executable := writeExecutable(t, []byte("current binary"))

	options := runOptions(server, executable)
	options.GOOS = "darwin"
	options.GOARCH = "arm64"
	_, err := Run(context.Background(), server.Client(), options)
	if err == nil {
		t.Fatal("Run() returned nil error, want asset-not-found error")
	}
	want := fmt.Sprintf("release v0.2.0 does not contain asset %s", AssetName("0.2.0", "darwin", "arm64"))
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err, want)
	}
}
