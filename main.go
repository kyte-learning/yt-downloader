package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
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

var (
	AppVersion = "dev"
	AppCommit  = "dev"
)

type App struct {
	th            *material.Theme
	window        *app.Window
	urlEditor     widget.Editor
	splitChapters widget.Bool
	downloadBtn   widget.Clickable
	homeDir       string
	appDir        string

	mu               sync.Mutex
	depsMu           sync.Mutex
	statusText       string
	isDownloading    bool
	progressText     string
	notificationText string
}

func main() {
	go func() {
		w := new(app.Window)
		w.Option(app.Title(AppName))
		w.Option(app.Size(unit.Dp(600), unit.Dp(430)))

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
		th:         th,
		window:     w,
		homeDir:    homeDir,
		appDir:     filepath.Join(homeDir, AppDir),
		statusText: "Ready to download",
	}

	appInstance.urlEditor.SingleLine = true
	appInstance.urlEditor.Submit = true
	appInstance.splitChapters.Value = true

	go appInstance.checkStartupUpdates()

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
	for {
		event, ok := gtx.Event(key.Filter{Name: key.NameReturn, Optional: key.ModCommand})
		if !ok {
			break
		}
		if e, ok := event.(key.Event); ok && e.State == key.Press {
			a.handleDownload()
		}
	}

	if a.downloadBtn.Clicked(gtx) {
		a.handleDownload()
	}

	statusText, progressText, notificationText, isDownloading := a.snapshot()

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						title := material.H4(a.th, AppName)
						title.Alignment = text.Middle
						return title.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						version := material.Body2(a.th, buildLabel())
						version.Alignment = text.Middle
						return version.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
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
						if isDownloading {
							btn.Text = "Downloading..."
							btn.Background = a.th.Palette.ContrastBg
						}
						return btn.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Body2(a.th, statusText).Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if progressText == "" {
							return layout.Dimensions{}
						}
						return material.Body2(a.th, progressText).Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if notificationText == "" {
							return layout.Dimensions{}
						}
						return material.Body2(a.th, notificationText).Layout(gtx)
					}),
				)
			}),
		)
	})
}

func (a *App) handleDownload() {
	if a.downloading() {
		return
	}

	url := strings.TrimSpace(a.urlEditor.Text())
	if url == "" {
		a.setStatus("Please enter a YouTube URL", "")
		return
	}

	if !strings.Contains(url, "youtube.com") && !strings.Contains(url, "youtu.be") {
		a.setStatus("Please enter a valid YouTube URL", "")
		return
	}

	splitChapters := a.splitChapters.Value
	go a.performDownload(url, splitChapters)
}

func (a *App) performDownload(url string, splitChapters bool) {
	a.setDownloading(true)
	defer a.setDownloading(false)

	a.setStatus("Preparing download...", "")
	if err := os.MkdirAll(a.appDir, 0755); err != nil {
		a.setStatus(fmt.Sprintf("Error creating app directory: %v", err), "")
		return
	}

	a.setStatus("Preparing download...", "Checking dependencies...")
	if err := a.ensureDependencies(false); err != nil {
		a.setStatus(fmt.Sprintf("Error downloading dependencies: %v", err), fmt.Sprintf("Dependency error: %v", err))
		return
	}

	a.setStatus("Preparing download...", "Launching download...")
	if err := a.executeDownload(url, splitChapters); err != nil {
		a.setStatus(fmt.Sprintf("Download failed: %v", err), fmt.Sprintf("Execution error: %v", err))
		return
	}

	a.setStatus(fmt.Sprintf("Download started in %s", terminalName()), fmt.Sprintf("Check %s window for progress", terminalName()))
}

func (a *App) executeDownload(url string, splitChapters bool) error {
	a.setStatus("Preparing download...", "Starting download...")

	cfg, err := platformDependencies()
	if err != nil {
		return err
	}

	slug := a.generateSlug(url)
	downloadDir := filepath.Join(a.homeDir, "Downloads", slug)
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %w", err)
	}

	ytDlpPath := filepath.Join(a.appDir, cfg.YtDlpFileName)
	args := []string{
		"--ffmpeg-location", a.appDir,
		"-S", "res,ext:mp4:m4a",
		"--recode", "mp4",
		"-P", downloadDir,
	}

	if splitChapters {
		args = append(args, "--split-chapters", "-o", "chapter:%(section_number)s %(section_title)s.%(ext)s")
	}

	args = append(args, url)
	return a.executeInTerminal(ytDlpPath, args, downloadDir)
}

func (a *App) executeInTerminal(command string, args []string, downloadDir string) error {
	switch runtime.GOOS {
	case "darwin":
		return a.executeInMacTerminal(command, args, downloadDir)
	case "windows":
		return a.executeInWindowsTerminal(command, args, downloadDir)
	default:
		return fmt.Errorf("terminal integration is not supported on %s", runtime.GOOS)
	}
}

func (a *App) executeInMacTerminal(command string, args []string, downloadDir string) error {
	allArgs := append([]string{command}, args...)
	quotedArgs := make([]string, len(allArgs))
	for i, arg := range allArgs {
		quotedArgs[i] = quoteShellArg(arg)
	}

	fullCommand := strings.Join(quotedArgs, " ")
	terminalCommand := fmt.Sprintf("%s && echo 'Download completed! Opening folder...' && open %s && echo 'Closing terminal in 3 seconds...' && sleep 3",
		fullCommand, quoteShellArg(downloadDir))

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

func (a *App) executeInWindowsTerminal(command string, args []string, downloadDir string) error {
	allArgs := append([]string{command}, args...)
	quotedArgs := make([]string, len(allArgs))
	for i, arg := range allArgs {
		quotedArgs[i] = quoteWindowsCmdArg(arg)
	}

	scriptPath := filepath.Join(a.appDir, fmt.Sprintf("download-%d.cmd", time.Now().UnixNano()))
	script := fmt.Sprintf("@echo off\r\n%s\r\nset EXITCODE=%%ERRORLEVEL%%\r\nif not %%EXITCODE%%==0 (\r\n  echo Download failed with exit code %%EXITCODE%%.\r\n  pause\r\n  exit /b %%EXITCODE%%\r\n)\r\necho Download completed! Opening folder...\r\nexplorer %s\r\necho Closing window in 3 seconds...\r\ntimeout /t 3 /nobreak >nul\r\n",
		strings.Join(quotedArgs, " "), quoteWindowsCmdArg(downloadDir))

	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		return fmt.Errorf("failed to create download script: %w", err)
	}

	cmd := exec.Command("cmd", "/C", "start", "", scriptPath)
	return cmd.Start()
}

func quoteShellArg(arg string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(arg, "'", "'\"'\"'"))
}

func quoteWindowsCmdArg(arg string) string {
	arg = strings.ReplaceAll(arg, "%", "%%")
	arg = strings.ReplaceAll(arg, `"`, `\"`)
	return `"` + arg + `"`
}

func terminalName() string {
	if runtime.GOOS == "windows" {
		return "Command Prompt"
	}
	return "Terminal"
}

func (a *App) checkStartupUpdates() {
	a.setStatus("Checking for updates...", "")

	if err := os.MkdirAll(a.appDir, 0755); err != nil {
		a.setStatus("Ready to download", fmt.Sprintf("Update check failed: %v", err))
		return
	}

	if err := a.checkForAppUpdate(); err != nil {
		log.Printf("app update check failed: %v", err)
	}

	if err := a.ensureDependencies(true); err != nil {
		a.setStatus("Ready to download", fmt.Sprintf("Dependency update check failed: %v", err))
		return
	}

	a.setStatus("Ready to download", "Dependencies are up to date")
}

func (a *App) generateSlug(url string) string {
	re := regexp.MustCompile(`(?:v=|/)([a-zA-Z0-9_-]{11})`)
	matches := re.FindStringSubmatch(url)

	if len(matches) > 1 {
		return fmt.Sprintf("youtube_%s", matches[1])
	}

	return fmt.Sprintf("youtube_%d", time.Now().Unix())
}

func (a *App) snapshot() (statusText, progressText, notificationText string, isDownloading bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.statusText, a.progressText, a.notificationText, a.isDownloading
}

func (a *App) downloading() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.isDownloading
}

func (a *App) setDownloading(isDownloading bool) {
	a.mu.Lock()
	a.isDownloading = isDownloading
	a.mu.Unlock()
	a.invalidate()
}

func (a *App) setStatus(statusText, progressText string) {
	a.mu.Lock()
	a.statusText = statusText
	a.progressText = progressText
	a.mu.Unlock()
	a.invalidate()
}

func (a *App) setNotification(notificationText string) {
	a.mu.Lock()
	a.notificationText = notificationText
	a.mu.Unlock()
	a.invalidate()
}

func (a *App) invalidate() {
	if a.window != nil {
		a.window.Invalidate()
	}
}

func buildLabel() string {
	label := AppVersion
	if label == "" {
		label = "dev"
	}
	if AppCommit != "" && AppCommit != "dev" {
		label = fmt.Sprintf("%s (%s)", label, shortCommit(AppCommit))
	}
	return "Build: " + label
}

func shortCommit(commit string) string {
	if len(commit) <= 7 {
		return commit
	}
	return commit[:7]
}

// Additional utility functions for Chrome cookies extraction can be added here.
func (a *App) extractChromeCookies() error {
	return nil
}
