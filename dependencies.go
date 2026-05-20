package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	dependencyMetadataFile = "dependencies.json"
	ytDlpReleaseAPIURL     = "https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest"
	btbnReleaseAPIURL      = "https://api.github.com/repos/BtbN/FFmpeg-Builds/releases/tags/latest"
)

var downloadClient = &http.Client{Timeout: 30 * time.Minute}

type GitHubRelease struct {
	TagName         string        `json:"tag_name"`
	Name            string        `json:"name"`
	HTMLURL         string        `json:"html_url"`
	TargetCommitish string        `json:"target_commitish"`
	PublishedAt     string        `json:"published_at"`
	UpdatedAt       string        `json:"updated_at"`
	Assets          []GitHubAsset `json:"assets"`
}

type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
	UpdatedAt          string `json:"updated_at"`
}

type platformConfig struct {
	YtDlpAssetName    string
	YtDlpFileName     string
	FFmpegFileName    string
	FFprobeFileName   string
	FFmpegURL         string
	FFprobeURL        string
	WindowsFFmpegName string
}

type dependencyMetadata struct {
	YtDlp   managedDependency `json:"yt_dlp"`
	FFmpeg  managedDependency `json:"ffmpeg"`
	FFprobe managedDependency `json:"ffprobe"`
}

type managedDependency struct {
	Version      string `json:"version"`
	SourceURL    string `json:"source_url"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

type remoteFileInfo struct {
	URL          string
	ETag         string
	LastModified string
}

func platformDependencies() (platformConfig, error) {
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64" {
			return platformConfig{}, fmt.Errorf("unsupported macOS architecture %s", runtime.GOARCH)
		}

		ffmpegURL := fmt.Sprintf("https://ffmpeg.martin-riedl.de/redirect/latest/macos/%s/snapshot/ffmpeg.zip", runtime.GOARCH)
		ffprobeURL := fmt.Sprintf("https://ffmpeg.martin-riedl.de/redirect/latest/macos/%s/snapshot/ffprobe.zip", runtime.GOARCH)
		return platformConfig{
			YtDlpAssetName:  "yt-dlp_macos",
			YtDlpFileName:   "yt-dlp_macos",
			FFmpegFileName:  "ffmpeg",
			FFprobeFileName: "ffprobe",
			FFmpegURL:       ffmpegURL,
			FFprobeURL:      ffprobeURL,
		}, nil
	case "windows":
		if runtime.GOARCH != "amd64" {
			return platformConfig{}, fmt.Errorf("unsupported Windows architecture %s", runtime.GOARCH)
		}

		return platformConfig{
			YtDlpAssetName:    "yt-dlp.exe",
			YtDlpFileName:     "yt-dlp.exe",
			FFmpegFileName:    "ffmpeg.exe",
			FFprobeFileName:   "ffprobe.exe",
			WindowsFFmpegName: "ffmpeg-master-latest-win64-gpl.zip",
		}, nil
	default:
		return platformConfig{}, fmt.Errorf("unsupported platform %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func (a *App) ensureDependencies(checkUpdates bool) error {
	a.depsMu.Lock()
	defer a.depsMu.Unlock()

	cfg, err := platformDependencies()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(a.appDir, 0755); err != nil {
		return fmt.Errorf("failed to create app directory: %w", err)
	}

	metadata, err := a.loadDependencyMetadata()
	if err != nil {
		metadata = dependencyMetadata{}
	}

	if err := a.ensureYtDlp(cfg, &metadata, checkUpdates); err != nil {
		return fmt.Errorf("failed to prepare yt-dlp: %w", err)
	}

	if err := a.ensureFFmpeg(cfg, &metadata, checkUpdates); err != nil {
		return fmt.Errorf("failed to prepare ffmpeg: %w", err)
	}

	return a.saveDependencyMetadata(metadata)
}

func (a *App) ensureYtDlp(cfg platformConfig, metadata *dependencyMetadata, checkUpdates bool) error {
	localPath := filepath.Join(a.appDir, cfg.YtDlpFileName)
	missing := !fileExists(localPath)
	if !missing && !checkUpdates {
		return nil
	}

	release, err := fetchGitHubRelease(ytDlpReleaseAPIURL)
	if err != nil {
		return err
	}

	if !missing && metadata.YtDlp.Version == release.TagName {
		return nil
	}

	asset, ok := release.FindAsset(cfg.YtDlpAssetName)
	if !ok {
		return fmt.Errorf("asset %q not found in yt-dlp release %q", cfg.YtDlpAssetName, release.TagName)
	}

	a.setStatus("Checking for updates...", "Downloading yt-dlp...")
	if err := downloadFile(asset.BrowserDownloadURL, localPath); err != nil {
		return err
	}

	metadata.YtDlp = managedDependency{
		Version:   release.TagName,
		SourceURL: asset.BrowserDownloadURL,
		UpdatedAt: firstNonEmpty(release.PublishedAt, release.UpdatedAt, asset.UpdatedAt),
	}
	return nil
}

func (a *App) ensureFFmpeg(cfg platformConfig, metadata *dependencyMetadata, checkUpdates bool) error {
	if runtime.GOOS == "windows" {
		return a.ensureWindowsFFmpeg(cfg, metadata, checkUpdates)
	}

	if err := a.ensureFFmpegZipBinary(cfg.FFmpegFileName, cfg.FFmpegURL, &metadata.FFmpeg, checkUpdates); err != nil {
		return err
	}
	return a.ensureFFmpegZipBinary(cfg.FFprobeFileName, cfg.FFprobeURL, &metadata.FFprobe, checkUpdates)
}

func (a *App) ensureFFmpegZipBinary(binaryName, downloadURL string, metadata *managedDependency, checkUpdates bool) error {
	localPath := filepath.Join(a.appDir, binaryName)
	missing := !fileExists(localPath)
	if !missing && !checkUpdates {
		return nil
	}

	remoteInfo, err := fetchRemoteFileInfo(downloadURL)
	if err != nil {
		if !missing {
			return nil
		}
	} else {
		version := remoteInfo.VersionKey()
		if !missing && metadata.Version == version {
			return nil
		}
	}

	version := remoteInfo.VersionKey()
	a.setStatus("Checking for updates...", fmt.Sprintf("Downloading %s...", binaryName))
	archivePath := filepath.Join(a.appDir, binaryName+".zip")
	if err := downloadFile(downloadURL, archivePath); err != nil {
		return err
	}
	defer os.Remove(archivePath)

	if err := a.extractBinariesFromZip(archivePath, []string{binaryName}); err != nil {
		return err
	}
	if version == "" {
		checksum, err := fileSHA256(archivePath)
		if err != nil {
			return err
		}
		version = "sha256:" + checksum
	}

	*metadata = managedDependency{
		Version:      version,
		SourceURL:    downloadURL,
		ETag:         remoteInfo.ETag,
		LastModified: remoteInfo.LastModified,
	}
	return nil
}

func (a *App) ensureWindowsFFmpeg(cfg platformConfig, metadata *dependencyMetadata, checkUpdates bool) error {
	ffmpegPath := filepath.Join(a.appDir, cfg.FFmpegFileName)
	ffprobePath := filepath.Join(a.appDir, cfg.FFprobeFileName)
	missing := !fileExists(ffmpegPath) || !fileExists(ffprobePath)
	if !missing && !checkUpdates {
		return nil
	}

	release, err := fetchGitHubRelease(btbnReleaseAPIURL)
	if err != nil {
		return err
	}

	asset, ok := release.FindAsset(cfg.WindowsFFmpegName)
	if !ok {
		return fmt.Errorf("asset %q not found in FFmpeg release %q", cfg.WindowsFFmpegName, release.Name)
	}

	version := firstNonEmpty(asset.Digest, asset.UpdatedAt, release.PublishedAt, release.UpdatedAt, release.Name)
	if !missing && metadata.FFmpeg.Version == version {
		return nil
	}

	a.setStatus("Checking for updates...", "Downloading FFmpeg for Windows...")
	archivePath := filepath.Join(a.appDir, cfg.WindowsFFmpegName)
	if err := downloadFile(asset.BrowserDownloadURL, archivePath); err != nil {
		return err
	}
	defer os.Remove(archivePath)

	if err := a.extractBinariesFromZip(archivePath, []string{cfg.FFmpegFileName, cfg.FFprobeFileName}); err != nil {
		return err
	}

	metadata.FFmpeg = managedDependency{
		Version:   version,
		SourceURL: asset.BrowserDownloadURL,
		UpdatedAt: firstNonEmpty(asset.UpdatedAt, release.PublishedAt, release.UpdatedAt),
	}
	metadata.FFprobe = metadata.FFmpeg
	return nil
}

func (a *App) extractBinariesFromZip(archivePath string, binaryNames []string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open ZIP archive: %w", err)
	}
	defer archive.Close()

	remaining := make(map[string]bool, len(binaryNames))
	for _, binaryName := range binaryNames {
		remaining[binaryName] = true
	}

	for _, file := range archive.File {
		binaryName := path.Base(file.Name)
		if !remaining[binaryName] {
			continue
		}

		if err := a.extractZipEntry(file, filepath.Join(a.appDir, binaryName)); err != nil {
			return err
		}
		delete(remaining, binaryName)
	}

	if len(remaining) > 0 {
		missing := make([]string, 0, len(remaining))
		for binaryName := range remaining {
			missing = append(missing, binaryName)
		}
		return fmt.Errorf("binary not found in archive: %s", strings.Join(missing, ", "))
	}

	return nil
}

func (a *App) extractZipEntry(file *zip.File, destination string) error {
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()

	tempPath := destination + ".download"
	writer, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(writer, reader)
	closeErr := writer.Close()
	if copyErr != nil {
		os.Remove(tempPath)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(tempPath)
		return closeErr
	}

	if err := os.Chmod(tempPath, 0755); err != nil {
		os.Remove(tempPath)
		return err
	}

	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		os.Remove(tempPath)
		return err
	}

	return os.Rename(tempPath, destination)
}

func (a *App) loadDependencyMetadata() (dependencyMetadata, error) {
	metadataPath := filepath.Join(a.appDir, dependencyMetadataFile)
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return dependencyMetadata{}, nil
		}
		return dependencyMetadata{}, err
	}

	var metadata dependencyMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return dependencyMetadata{}, err
	}
	return metadata, nil
}

func (a *App) saveDependencyMetadata(metadata dependencyMetadata) error {
	metadataPath := filepath.Join(a.appDir, dependencyMetadataFile)
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metadataPath, append(data, '\n'), 0644)
}

func fetchGitHubRelease(apiURL string) (GitHubRelease, error) {
	var release GitHubRelease
	if err := getJSON(apiURL, &release); err != nil {
		return GitHubRelease{}, err
	}
	return release, nil
}

func (r GitHubRelease) FindAsset(name string) (GitHubAsset, bool) {
	for _, asset := range r.Assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return GitHubAsset{}, false
}

func getJSON(url string, target any) error {
	req, err := newHTTPRequest(http.MethodGet, url)
	if err != nil {
		return err
	}

	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("GET %s returned %s", url, resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func fetchRemoteFileInfo(url string) (remoteFileInfo, error) {
	req, err := newHTTPRequest(http.MethodHead, url)
	if err != nil {
		return remoteFileInfo{}, err
	}

	resp, err := downloadClient.Do(req)
	if err != nil {
		return remoteFileInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return remoteFileInfo{}, fmt.Errorf("HEAD %s returned %s", url, resp.Status)
	}

	return remoteFileInfo{
		URL:          resp.Request.URL.String(),
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}, nil
}

func (r remoteFileInfo) VersionKey() string {
	return strings.Join(nonEmptyStrings(r.URL, r.ETag, r.LastModified), "|")
}

func downloadFile(url, destination string) error {
	req, err := newHTTPRequest(http.MethodGet, url)
	if err != nil {
		return err
	}

	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("GET %s returned %s", url, resp.Status)
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}

	tempPath := destination + ".download"
	out, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		os.Remove(tempPath)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(tempPath)
		return closeErr
	}
	if err := os.Chmod(tempPath, 0755); err != nil {
		os.Remove(tempPath)
		return err
	}

	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		os.Remove(tempPath)
		return err
	}

	return os.Rename(tempPath, destination)
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func newHTTPRequest(method, url string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "kyte-yt-downloader/"+AppVersion)
	return req, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func nonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return result
}
