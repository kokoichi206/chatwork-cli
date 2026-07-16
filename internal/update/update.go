// Package update は GitHub Releases から cw の最新バイナリを取得し、
// 実行中のバイナリ自身を置き換える。
package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	defaultReleasesAPI = "https://api.github.com/repos/kokoichi206/chatwork-cli/releases/latest"
	binaryName         = "cw"
	// checksumsAssetName は .goreleaser.yaml の checksum.name_template と一致させること。
	checksumsAssetName = "checksums.txt"
)

// Options は self-update の実行条件。
type Options struct {
	CurrentVersion string
	ExecutablePath string // 空なら os.Executable()
	GOOS           string // 空なら runtime.GOOS
	GOARCH         string // 空なら runtime.GOARCH
	DryRun         bool

	// releasesAPI はテストで httptest サーバーへ差し替えるための経路。空なら既定の GitHub API。
	releasesAPI string
}

// Result は選択されたリリースと実行結果。
type Result struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	ReleaseURL      string `json:"release_url"`
	AssetName       string `json:"asset_name,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Updated         bool   `json:"updated"`
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	HTMLURL string        `json:"html_url"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

// Run は最新リリースが現在のバージョンより新しい場合にダウンロードして置き換える。
// 開発ビルド("dev" 等、semver として解釈できない CurrentVersion)は常に更新対象とする。
func Run(ctx context.Context, client *http.Client, options Options) (Result, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if options.GOOS == "" {
		options.GOOS = runtime.GOOS
	}
	if options.GOARCH == "" {
		options.GOARCH = runtime.GOARCH
	}
	if options.ExecutablePath == "" {
		executable, err := os.Executable()
		if err != nil {
			return Result{}, err
		}
		options.ExecutablePath = executable
	}
	if options.releasesAPI == "" {
		options.releasesAPI = defaultReleasesAPI
	}

	release, err := latestRelease(ctx, client, options.releasesAPI)
	if err != nil {
		return Result{}, err
	}
	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(options.CurrentVersion, "v")
	result := Result{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
		ReleaseURL:     release.HTMLURL,
	}
	newer, err := isNewer(latestVersion, currentVersion)
	if err != nil {
		return Result{}, err
	}
	if !newer {
		return result, nil
	}
	result.UpdateAvailable = true

	assetName := AssetName(latestVersion, options.GOOS, options.GOARCH)
	asset, ok := findAsset(release.Assets, assetName)
	if !ok {
		return Result{}, fmt.Errorf("release %s does not contain asset %s", release.TagName, assetName)
	}
	result.AssetName = asset.Name
	if options.DryRun {
		return result, nil
	}

	checksumsAsset, ok := findAsset(release.Assets, checksumsAssetName)
	if !ok {
		return Result{}, fmt.Errorf("release %s does not contain %s", release.TagName, checksumsAssetName)
	}

	tmpDir, err := os.MkdirTemp("", "cw-update-*")
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archivePath := filepath.Join(tmpDir, asset.Name)
	if err := download(ctx, client, asset.DownloadURL, archivePath); err != nil {
		return Result{}, err
	}
	if err := verifyChecksum(ctx, client, checksumsAsset.DownloadURL, asset.Name, archivePath); err != nil {
		return Result{}, err
	}
	binaryPath, err := extractBinary(archivePath, tmpDir)
	if err != nil {
		return Result{}, err
	}
	if err := installBinary(binaryPath, options.ExecutablePath); err != nil {
		return Result{}, err
	}

	result.Updated = true
	return result, nil
}

// AssetName は .goreleaser.yaml の name_template と一致させること。
func AssetName(version string, goos string, goarch string) string {
	format := "tar.gz"
	if goos == "windows" {
		format = "zip"
	}
	return fmt.Sprintf("chatwork-cli_%s_%s_%s.%s", version, goos, goarch, format)
}

// isNewer は latest > current のとき true を返す。
// current が semver でない場合(開発ビルド)は true、latest が semver でない場合はエラー。
func isNewer(latest, current string) (bool, error) {
	latestParts, err := parseSemver(latest)
	if err != nil {
		return false, fmt.Errorf("latest release version %q is not semver: %w", latest, err)
	}
	currentParts, err := parseSemver(current)
	if err != nil {
		return true, nil
	}
	for i := range latestParts {
		if latestParts[i] != currentParts[i] {
			return latestParts[i] > currentParts[i], nil
		}
	}
	return false, nil
}

func parseSemver(version string) ([3]int, error) {
	var parts [3]int
	fields := strings.SplitN(version, ".", 3)
	if len(fields) != 3 {
		return parts, fmt.Errorf("expected 3 dot-separated fields, got %d", len(fields))
	}
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return parts, fmt.Errorf("invalid field %q", field)
		}
		parts[i] = n
	}
	return parts, nil
}

func latestRelease(ctx context.Context, client *http.Client, api string) (githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return githubRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return githubRelease{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	// GitHub の latest release API はリリースが 1 つもないリポジトリに対して 404 を返す。
	if resp.StatusCode == http.StatusNotFound {
		return githubRelease{}, errors.New("no releases are published yet")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return githubRelease{}, fmt.Errorf("fetch latest release: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return githubRelease{}, err
	}
	if release.TagName == "" {
		return githubRelease{}, errors.New("latest release response does not contain tag_name")
	}
	return release, nil
}

func findAsset(assets []githubAsset, name string) (githubAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return githubAsset{}, false
}

func download(ctx context.Context, client *http.Client, url string, destination string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}

	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// verifyChecksum はリリース同梱の checksums.txt と照合し、破損・改ざんされたアーカイブの展開を防ぐ。
func verifyChecksum(ctx context.Context, client *http.Client, checksumsURL string, assetName string, archivePath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumsURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", checksumsURL, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	want := ""
	for line := range strings.SplitSeq(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("%s does not contain an entry for %s", checksumsAssetName, assetName)
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	got := hex.EncodeToString(hash.Sum(nil))
	if got != want {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", assetName, got, want)
	}
	return nil
}

func extractBinary(archivePath string, destinationDir string) (string, error) {
	switch {
	case strings.HasSuffix(archivePath, ".tar.gz"):
		return extractTarGzBinary(archivePath, destinationDir)
	case strings.HasSuffix(archivePath, ".zip"):
		return extractZipBinary(archivePath, destinationDir)
	default:
		return "", fmt.Errorf("unsupported archive format: %s", archivePath)
	}
}

func extractTarGzBinary(archivePath string, destinationDir string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer func() { _ = gz.Close() }()

	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return "", fmt.Errorf("archive does not contain %s binary", binaryName)
		}
		if err != nil {
			return "", err
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != binaryName {
			continue
		}
		return writeExtractedBinary(reader, destinationDir)
	}
}

func extractZipBinary(archivePath string, destinationDir string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = reader.Close() }()

	for _, file := range reader.File {
		if !file.FileInfo().Mode().IsRegular() || filepath.Base(file.Name) != binaryName+".exe" {
			continue
		}
		src, err := file.Open()
		if err != nil {
			return "", err
		}
		defer func() { _ = src.Close() }()
		return writeExtractedBinary(src, destinationDir)
	}
	return "", fmt.Errorf("archive does not contain %s binary", binaryName)
}

func writeExtractedBinary(reader io.Reader, destinationDir string) (string, error) {
	path := filepath.Join(destinationDir, binaryName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(file, reader); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return path, nil
}

// installBinary は宛先と同じディレクトリに排他的な一時ファイルを作って rename することで、
// 置き換え途中の中途半端なバイナリが残らないようにする(同一ファイルシステム内の rename はアトミック)。
// Windows では実行中の exe を rename で直接上書きできないため、旧バイナリを .old へ退避してから入れ替える。
func installBinary(source string, destination string) error {
	info, err := os.Stat(destination)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("executable path is a directory: %s", destination)
	}

	tmp, err := os.CreateTemp(filepath.Dir(destination), "."+binaryName+"-update-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if err := fillFile(tmp, source, info.Mode().Perm()); err != nil {
		cleanup()
		return err
	}

	old := destination + ".old"
	// 前回更新の残骸。Windows で実行中だった旧バイナリが残ることがある。
	_ = os.Remove(old)
	if err := os.Rename(destination, old); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		_ = os.Rename(old, destination)
		cleanup()
		return err
	}
	// Windows では実行中の旧バイナリを削除できないため、失敗は無視して次回更新時に消す。
	_ = os.Remove(old)
	return nil
}

func fillFile(dst *os.File, sourcePath string, mode os.FileMode) error {
	src, err := os.Open(sourcePath)
	if err != nil {
		_ = dst.Close()
		return err
	}
	defer func() { _ = src.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	if err := dst.Chmod(mode); err != nil {
		_ = dst.Close()
		return err
	}
	return dst.Close()
}
