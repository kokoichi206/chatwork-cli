// Package update は GitHub Releases から cw の最新バイナリを取得し、
// 実行中のバイナリ自身を置き換える。
package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	defaultReleasesAPI = "https://api.github.com/repos/kokoichi206/chatwork-cli/releases/latest"
	binaryName         = "cw"
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
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	ReleaseURL     string `json:"release_url"`
	AssetName      string `json:"asset_name,omitempty"`
	Updated        bool   `json:"updated"`
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

// Run は最新リリースが現在のバージョンと異なる場合にダウンロードして置き換える。
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
	if currentVersion == latestVersion {
		return result, nil
	}

	assetName := AssetName(latestVersion, options.GOOS, options.GOARCH)
	asset, ok := findAsset(release.Assets, assetName)
	if !ok {
		return Result{}, fmt.Errorf("release %s does not contain asset %s", release.TagName, assetName)
	}
	result.AssetName = asset.Name
	if options.DryRun {
		return result, nil
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
		if filepath.Base(header.Name) != binaryName {
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
		if filepath.Base(file.Name) != binaryName+".exe" {
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

// installBinary は同一ファイルシステム上の一時ファイル経由で rename し、
// 置き換え途中の中途半端なバイナリが残らないようにする。
func installBinary(source string, destination string) error {
	info, err := os.Stat(destination)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("executable path is a directory: %s", destination)
	}

	tmp := destination + ".tmp"
	if err := copyFile(source, tmp, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmp, destination)
}

func copyFile(source string, destination string, mode os.FileMode) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	return dst.Close()
}
