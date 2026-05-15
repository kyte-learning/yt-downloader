package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const (
	AppName = "Kyte YouTube Downloader"
	AppDir  = ".kyte-yt-downloader"
)

type App struct {
	th               *material.Theme
	urlEditor        widget.Editor
	splitChapters    widget.Bool
	downloadBtn      widget.Clickable
	statusText       string
	isDownloading    bool
	progressText     string
	homeDir          string
	appDir           string
	notificationText string
}

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func main() {
	go func() {
		w := new(app.Window)
		w.Option(app.Title(AppName))
		w.Option(app.Size(unit.Dp(600), unit.Dp(400)))

		if err := run(w); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(w *app.Window) error {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	appInstance := &App{
		th:      th,
		homeDir: homeDir,
		appDir:  filepath.Join(homeDir, AppDir),
	}

	// Initialize URL editor
	appInstance.urlEditor.SingleLine = true
	appInstance.urlEditor.Submit = true

	// Set split chapters as default
	appInstance.splitChapters.Value = true

	// Set default status
	appInstance.statusText = "Ready to download"

	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			appInstance.Layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (a *App) Layout(gtx layout.Context) layout.Dimensions {
	// Handle key events
	for {
		event, ok := gtx.Event(key.Filter{Name: key.NameReturn, Optional: key.ModCommand})
		if !ok {
			break
		}
		if e, ok := event.(key.Event); ok && e.State == key.Press {
			a.handleDownload()
		}
	}

	// Handle download button click
	if a.downloadBtn.Clicked(gtx) {
		a.handleDownload()
	}

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						title := material.H4(a.th, AppName)
						title.Alignment = text.Middle
						return title.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Body1(a.th, "YouTube URL:").Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						editor := material.Editor(a.th, &a.urlEditor, "https://www.youtube.com/watch?v=...")
						editor.Editor.Submit = true
						return editor.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.CheckBox(a.th, &a.splitChapters, "Split Chapters").Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := material.Button(a.th, &a.downloadBtn, "Download")
						if a.isDownloading {
							btn.Text = "Downloading..."
							btn.Background = a.th.Palette.ContrastBg
						}
						return btn.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Body2(a.th, a.statusText).Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if a.progressText != "" {
							return material.Body2(a.th, a.progressText).Layout(gtx)
						}
						return layout.Dimensions{}
					}),
				)
			}),
		)
	})
}

func (a *App) handleDownload() {
	if a.isDownloading {
		return
	}

	url := strings.TrimSpace(a.urlEditor.Text())
	if url == "" {
		a.statusText = "Please enter a YouTube URL"
		return
	}

	if !strings.Contains(url, "youtube.com") && !strings.Contains(url, "youtu.be") {
		a.statusText = "Please enter a valid YouTube URL"
		return
	}

	go a.performDownload(url)
}

func (a *App) performDownload(url string) {
	a.isDownloading = true
	a.statusText = "Preparing download..."
	a.progressText = ""

	// Ensure app directory exists
	if err := os.MkdirAll(a.appDir, 0755); err != nil {
		a.statusText = fmt.Sprintf("Error creating app directory: %v", err)
		a.progressText = ""
		a.isDownloading = false
		return
	}

	// Download dependencies if needed
	a.progressText = "Checking dependencies..."
	if err := a.ensureDependencies(); err != nil {
		a.statusText = fmt.Sprintf("Error downloading dependencies: %v", err)
		a.progressText = fmt.Sprintf("Dependency error: %v", err)
		a.isDownloading = false
		return
	}

	// Execute download
	a.progressText = "Launching download..."
	if err := a.executeDownload(url); err != nil {
		a.statusText = fmt.Sprintf("Download failed: %v", err)
		a.progressText = fmt.Sprintf("Execution error: %v", err)
		a.isDownloading = false
		return
	}

	a.statusText = "Download started in Terminal"
	a.progressText = "Check Terminal window for progress"
	a.isDownloading = false
}

func (a *App) ensureDependencies() error {
	a.progressText = "Checking dependencies..."

	// Check if yt-dlp exists
	ytDlpPath := filepath.Join(a.appDir, "yt-dlp_macos")
	if _, err := os.Stat(ytDlpPath); os.IsNotExist(err) {
		a.progressText = "Downloading yt-dlp..."
		if err := a.downloadYtDlp(); err != nil {
			return fmt.Errorf("failed to download yt-dlp: %w", err)
		}
		// Verify yt-dlp is executable
		if err := os.Chmod(ytDlpPath, 0755); err != nil {
			return fmt.Errorf("failed to make yt-dlp executable: %w", err)
		}
		// Wait for file system to be ready
		a.progressText = "Preparing yt-dlp..."
		time.Sleep(3 * time.Second)
	}

	// Check if ffmpeg exists
	ffmpegPath := filepath.Join(a.appDir, "ffmpeg")
	if _, err := os.Stat(ffmpegPath); os.IsNotExist(err) {
		a.progressText = "Downloading ffmpeg..."
		if err := a.downloadFFmpeg(); err != nil {
			return fmt.Errorf("failed to download ffmpeg: %w", err)
		}
		// Wait for file system to be ready
		a.progressText = "Preparing ffmpeg..."
		time.Sleep(3 * time.Second)
	}

	// Check if ffprobe exists
	ffprobePath := filepath.Join(a.appDir, "ffprobe")
	if _, err := os.Stat(ffprobePath); os.IsNotExist(err) {
		a.progressText = "Downloading ffprobe..."
		if err := a.downloadFFprobe(); err != nil {
			return fmt.Errorf("failed to download ffprobe: %w", err)
		}
		// Wait for file system to be ready
		a.progressText = "Preparing ffprobe..."
		time.Sleep(3 * time.Second)
	}

	a.progressText = "Dependencies ready"
	return nil
}

func (a *App) downloadYtDlp() error {
	// Get latest release info
	resp, err := http.Get("https://api.github.com/repos/yt-dlp/yt-dlp/releases/latest")
	if err != nil {
		return fmt.Errorf("failed to get release info: %w", err)
	}
	defer resp.Body.Close()

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to parse release info: %w", err)
	}

	// Find macOS binary
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == "yt-dlp_macos" {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("macOS binary not found in release")
	}

	// Download the binary
	return a.downloadFile(downloadURL, filepath.Join(a.appDir, "yt-dlp_macos"))
}

func (a *App) downloadFFmpeg() error {
	// Use Martin Riedl's FFmpeg Build Server for ARM64 Apple Silicon binary
	downloadURL := "https://ffmpeg.martin-riedl.de/redirect/latest/macos/arm64/snapshot/ffmpeg.zip"

	// Download to temp file first
	tempFile := filepath.Join(a.appDir, "ffmpeg.zip")
	if err := a.downloadFile(downloadURL, tempFile); err != nil {
		return err
	}
	defer os.Remove(tempFile)

	// Extract as zip
	return a.extractFFmpegBinary(tempFile, "ffmpeg")
}

func (a *App) downloadFFprobe() error {
	// Use Martin Riedl's FFmpeg Build Server for ARM64 Apple Silicon binary
	downloadURL := "https://ffmpeg.martin-riedl.de/redirect/latest/macos/arm64/snapshot/ffprobe.zip"

	// Download to temp file first
	tempFile := filepath.Join(a.appDir, "ffprobe.zip")
	if err := a.downloadFile(downloadURL, tempFile); err != nil {
		return err
	}
	defer os.Remove(tempFile)

	// Extract as zip
	return a.extractFFmpegBinary(tempFile, "ffprobe")
}

func (a *App) downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	// Make executable if it's a binary
	if strings.HasSuffix(filepath, "_macos") || strings.HasSuffix(filepath, "ffmpeg") || strings.HasSuffix(filepath, "ffprobe") {
		return os.Chmod(filepath, 0755)
	}

	return nil
}

func (a *App) extractFFmpegBinary(archivePath, binaryName string) error {
	// Extract as ZIP
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open ZIP archive: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, binaryName) {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			outPath := filepath.Join(a.appDir, binaryName)
			outFile, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			defer outFile.Close()

			_, err = io.Copy(outFile, rc)
			return err
		}
	}

	return fmt.Errorf("binary %s not found in archive", binaryName)
}

func (a *App) executeDownload(url string) error {
	a.progressText = "Starting download..."

	// Generate slug from URL
	slug := a.generateSlug(url)
	downloadDir := filepath.Join(a.homeDir, "Downloads", slug)

	// Ensure download directory exists
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %w", err)
	}

	// Build command
	ytDlpPath := filepath.Join(a.appDir, "yt-dlp_macos")
	args := []string{
		"--ffmpeg-location", a.appDir,
		"-S", "res,ext:mp4:m4a",
		"--recode", "mp4",
		"-P", downloadDir,
	}

	if a.splitChapters.Value {
		args = append(args, "--split-chapters", "-o", "chapter:%(section_number)s %(section_title)s.%(ext)s")
	}

	args = append(args, url)

	// Execute in terminal for progress monitoring
	return a.executeInTerminal(ytDlpPath, args, downloadDir)
}

func (a *App) executeInTerminal(command string, args []string, downloadDir string) error {
	// Build command with proper quoting
	allArgs := append([]string{command}, args...)
	quotedArgs := make([]string, len(allArgs))
	for i, arg := range allArgs {
		quotedArgs[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(arg, "'", "'\"'\"'"))
	}

	fullCommand := strings.Join(quotedArgs, " ")
	terminalCommand := fmt.Sprintf("%s && echo 'Download completed! Opening folder...' && open '%s' && echo 'Closing terminal in 3 seconds...' && sleep 3",
		fullCommand, downloadDir)

	// AppleScript that closes the window after execution
	appleScript := fmt.Sprintf(`tell application "Terminal"
	activate
	set newWindow to do script "%s"
	repeat
		delay 1
		if not busy of newWindow then exit repeat
	end repeat
	close newWindow
end tell`, strings.ReplaceAll(terminalCommand, `"`, `\"`))

	cmd := exec.Command("osascript", "-e", appleScript)
	return cmd.Run()
}

func (a *App) generateSlug(url string) string {
	// Extract video ID or use timestamp
	re := regexp.MustCompile(`(?:v=|/)([a-zA-Z0-9_-]{11})`)
	matches := re.FindStringSubmatch(url)

	if len(matches) > 1 {
		return fmt.Sprintf("youtube_%s", matches[1])
	}

	// Fallback to timestamp
	return fmt.Sprintf("youtube_%d", time.Now().Unix())
}

// Additional utility functions for Chrome cookies extraction can be added here
// This is a placeholder for the cookie extraction functionality
func (a *App) extractChromeCookies() error {
	// Implementation for Chrome cookie extraction
	// This would involve reading Chrome's cookie database
	// and extracting YouTube cookies to prevent rate limiting
	return nil
}
