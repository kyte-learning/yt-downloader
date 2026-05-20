package main

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	appReleaseAPIURL = "https://api.github.com/repos/kyte-learning/yt-downloader/releases/latest"
	appReleaseURL    = "https://github.com/kyte-learning/yt-downloader/releases/latest"
)

func (a *App) checkForAppUpdate() error {
	release, err := fetchGitHubRelease(appReleaseAPIURL)
	if err != nil {
		return err
	}

	if !appUpdateAvailable(release) {
		return nil
	}

	releaseName := firstNonEmpty(release.Name, release.TagName, "latest build")
	a.setNotification(fmt.Sprintf("Update available: %s. Installing automatically...", releaseName))
	if err := a.installAppUpdate(release); err != nil {
		a.setNotification(fmt.Sprintf("Auto update failed: %v. Download it from %s", err, firstNonEmpty(release.HTMLURL, appReleaseURL)))
		return err
	}

	return nil
}

func (a *App) installAppUpdate(release GitHubRelease) error {
	artifactName, err := appUpdateArtifactName()
	if err != nil {
		return err
	}

	asset, ok := release.FindAsset(artifactName)
	if !ok {
		return fmt.Errorf("release asset %q was not found", artifactName)
	}

	stageRoot := filepath.Join(a.appDir, "app-update")
	if err := os.RemoveAll(stageRoot); err != nil {
		return err
	}
	if err := os.MkdirAll(stageRoot, 0755); err != nil {
		return err
	}

	archivePath := filepath.Join(stageRoot, artifactName)
	a.setStatus("Installing app update...", "Downloading latest app build...")
	if err := downloadFile(asset.BrowserDownloadURL, archivePath); err != nil {
		return err
	}

	extractDir := filepath.Join(stageRoot, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return err
	}

	a.setStatus("Installing app update...", "Preparing latest app build...")
	if runtime.GOOS == "darwin" {
		if err := runCommand("ditto", "-x", "-k", archivePath, extractDir); err != nil {
			return err
		}
	} else if err := extractZipArchive(archivePath, extractDir); err != nil {
		return err
	}

	switch runtime.GOOS {
	case "darwin":
		return a.installMacAppUpdate(stageRoot, extractDir)
	case "windows":
		return a.installWindowsAppUpdate(stageRoot, extractDir)
	default:
		return fmt.Errorf("self update is not supported on %s", runtime.GOOS)
	}
}

func (a *App) installMacAppUpdate(stageRoot, extractDir string) error {
	destinationApp, err := currentMacAppBundle()
	if err != nil {
		return err
	}
	if err := ensureWritableDestination(destinationApp); err != nil {
		return err
	}

	sourceApp, err := findStagedMacApp(extractDir)
	if err != nil {
		return err
	}

	_ = runCommand("xattr", "-dr", "com.apple.quarantine", sourceApp)

	helperPath := filepath.Join(a.appDir, "apply-app-update.sh")
	logPath := filepath.Join(a.appDir, "app-update.log")
	script := `#!/bin/sh
set -eu
APP_PID="$1"
SOURCE_APP="$2"
DEST_APP="$3"
STAGE_ROOT="$4"
LOG_FILE="$5"
exec >> "$LOG_FILE" 2>&1
echo "Starting Kyte app update at $(date)"
while kill -0 "$APP_PID" 2>/dev/null; do
	sleep 0.2
done
TMP_APP="${DEST_APP}.updating"
BACKUP_APP="${DEST_APP}.previous"
rm -rf "$TMP_APP" "$BACKUP_APP"
ditto "$SOURCE_APP" "$TMP_APP"
/usr/bin/xattr -dr com.apple.quarantine "$TMP_APP" 2>/dev/null || true
chmod -R u+rwX "$TMP_APP" 2>/dev/null || true
if [ -d "$DEST_APP" ]; then
	mv "$DEST_APP" "$BACKUP_APP"
fi
if ! mv "$TMP_APP" "$DEST_APP"; then
	if [ -d "$BACKUP_APP" ]; then
		mv "$BACKUP_APP" "$DEST_APP"
	fi
	exit 1
fi
/usr/bin/xattr -dr com.apple.quarantine "$DEST_APP" 2>/dev/null || true
rm -rf "$BACKUP_APP" "$STAGE_ROOT"
open "$DEST_APP"
rm -f "$0"
`

	if err := os.WriteFile(helperPath, []byte(script), 0700); err != nil {
		return err
	}
	if err := os.Chmod(helperPath, 0700); err != nil {
		return err
	}

	cmd := exec.Command("/bin/sh", helperPath, strconv.Itoa(os.Getpid()), sourceApp, destinationApp, stageRoot, logPath)
	if err := cmd.Start(); err != nil {
		return err
	}

	a.finishSelfUpdate()
	return nil
}

func (a *App) installWindowsAppUpdate(stageRoot, extractDir string) error {
	destinationExe, err := currentExecutablePath()
	if err != nil {
		return err
	}
	if err := ensureWritableDestination(destinationExe); err != nil {
		return err
	}

	sourceExe, err := findStagedWindowsExe(extractDir)
	if err != nil {
		return err
	}

	helperPath := filepath.Join(a.appDir, "apply-app-update.cmd")
	logPath := filepath.Join(a.appDir, "app-update.log")
	script := `@echo off
setlocal
set "APP_PID=%~1"
set "SOURCE_EXE=%~2"
set "DEST_EXE=%~3"
set "STAGE_ROOT=%~4"
set "LOG_FILE=%~5"
call :apply >> "%LOG_FILE%" 2>&1
exit /b %ERRORLEVEL%

:apply
echo Starting Kyte app update at %DATE% %TIME%
powershell -NoProfile -ExecutionPolicy Bypass -Command "try { Wait-Process -Id %APP_PID% -ErrorAction SilentlyContinue } catch {}"
set "BACKUP_EXE=%DEST_EXE%.previous"
if exist "%BACKUP_EXE%" del /F /Q "%BACKUP_EXE%"
if exist "%DEST_EXE%" copy /Y "%DEST_EXE%" "%BACKUP_EXE%" >nul
copy /Y "%SOURCE_EXE%" "%DEST_EXE%" >nul
if errorlevel 1 (
	if exist "%BACKUP_EXE%" copy /Y "%BACKUP_EXE%" "%DEST_EXE%" >nul
	exit /b 1
)
if exist "%BACKUP_EXE%" del /F /Q "%BACKUP_EXE%"
start "" "%DEST_EXE%"
rmdir /S /Q "%STAGE_ROOT%"
del /F /Q "%~f0"
exit /b 0
`

	if err := os.WriteFile(helperPath, []byte(script), 0700); err != nil {
		return err
	}

	cmd := exec.Command("cmd", "/C", "start", "", helperPath, strconv.Itoa(os.Getpid()), sourceExe, destinationExe, stageRoot, logPath)
	if err := cmd.Start(); err != nil {
		return err
	}

	a.finishSelfUpdate()
	return nil
}

func (a *App) finishSelfUpdate() {
	a.setStatus("Installing app update...", "Restarting to complete update...")
	time.Sleep(750 * time.Millisecond)
	os.Exit(0)
}

func appUpdateArtifactName() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "kyte-yt-downloader-macos-arm64.zip", nil
		}
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "kyte-yt-downloader-windows-x64.zip", nil
		}
	}

	return "", fmt.Errorf("self update is not supported on %s/%s", runtime.GOOS, runtime.GOARCH)
}

func currentMacAppBundle() (string, error) {
	exePath, err := currentExecutablePath()
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(exePath)
	for {
		if strings.HasSuffix(dir, ".app") {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("current executable is not inside a macOS app bundle")
}

func currentExecutablePath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}

	resolvedPath, err := filepath.EvalSymlinks(exePath)
	if err == nil {
		return resolvedPath, nil
	}
	return exePath, nil
}

func ensureWritableDestination(destination string) error {
	testFile, err := os.CreateTemp(filepath.Dir(destination), ".kyte-update-*")
	if err != nil {
		return fmt.Errorf("%s is not writable: %w", filepath.Dir(destination), err)
	}
	testPath := testFile.Name()
	if err := testFile.Close(); err != nil {
		os.Remove(testPath)
		return err
	}
	return os.Remove(testPath)
}

func findStagedMacApp(root string) (string, error) {
	var appPath string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if appPath != "" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() && strings.HasSuffix(d.Name(), ".app") {
			appPath = path
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if appPath == "" {
		return "", fmt.Errorf("macOS app bundle was not found in update archive")
	}
	return appPath, nil
}

func findStagedWindowsExe(root string) (string, error) {
	const exeName = "Kyte YouTube Downloader.exe"
	var exePath string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if exePath != "" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), exeName) {
			exePath = path
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if exePath == "" {
		return "", fmt.Errorf("Windows executable was not found in update archive")
	}
	return exePath, nil
}

func extractZipArchive(archivePath, destination string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open ZIP archive: %w", err)
	}
	defer archive.Close()

	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}

	for _, file := range archive.File {
		filePath := filepath.Join(destination, filepath.FromSlash(file.Name))
		if !isPathInside(filePath, destination) {
			return fmt.Errorf("unsafe path in update archive: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, file.Mode()); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return err
		}

		source, err := file.Open()
		if err != nil {
			return err
		}

		mode := file.Mode()
		if mode == 0 {
			mode = 0644
		}
		target, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err != nil {
			source.Close()
			return err
		}

		_, copyErr := io.Copy(target, source)
		closeSourceErr := source.Close()
		closeTargetErr := target.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeSourceErr != nil {
			return closeSourceErr
		}
		if closeTargetErr != nil {
			return closeTargetErr
		}
		if err := os.Chmod(filePath, mode); err != nil {
			return err
		}
	}

	return nil
}

func isPathInside(path, parent string) bool {
	path = filepath.Clean(path)
	parent = filepath.Clean(parent)
	return path == parent || strings.HasPrefix(path, parent+string(os.PathSeparator))
}

func runCommand(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func appUpdateAvailable(release GitHubRelease) bool {
	if release.TagName == "latest" && AppCommit != "" && AppCommit != "dev" && release.TargetCommitish != "" && release.TargetCommitish != "main" {
		return !sameCommit(AppCommit, release.TargetCommitish)
	}

	if AppVersion == "" || AppVersion == "dev" || release.TagName == "" || release.TagName == "latest" {
		return false
	}

	comparison, ok := compareSemanticVersions(release.TagName, AppVersion)
	return ok && comparison > 0
}

func sameCommit(current, latest string) bool {
	current = strings.TrimSpace(current)
	latest = strings.TrimSpace(latest)
	return strings.HasPrefix(current, latest) || strings.HasPrefix(latest, current)
}

func compareSemanticVersions(latest, current string) (int, bool) {
	latestParts, ok := semanticVersionParts(latest)
	if !ok {
		return 0, false
	}

	currentParts, ok := semanticVersionParts(current)
	if !ok {
		return 0, false
	}

	maxLen := len(latestParts)
	if len(currentParts) > maxLen {
		maxLen = len(currentParts)
	}

	for i := 0; i < maxLen; i++ {
		latestValue := 0
		if i < len(latestParts) {
			latestValue = latestParts[i]
		}

		currentValue := 0
		if i < len(currentParts) {
			currentValue = currentParts[i]
		}

		if latestValue > currentValue {
			return 1, true
		}
		if latestValue < currentValue {
			return -1, true
		}
	}

	return 0, true
}

func semanticVersionParts(version string) ([]int, bool) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	version = strings.Split(version, "-")[0]
	if version == "" {
		return nil, false
	}

	segments := strings.Split(version, ".")
	parts := make([]int, 0, len(segments))
	for _, segment := range segments {
		value, err := strconv.Atoi(segment)
		if err != nil {
			return nil, false
		}
		parts = append(parts, value)
	}

	return parts, len(parts) > 0
}
