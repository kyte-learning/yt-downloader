# Kyte YouTube Downloader

A Gio desktop YouTube downloader for macOS and Windows x64. The app manages `yt-dlp`, `ffmpeg`, and `ffprobe` automatically, checks for newer dependency builds at startup, and self-updates from the latest GitHub Actions build.

## Features

- **Native desktop builds**: macOS app bundle and Windows x64 executable
- **Clean GioUI interface**: Simple URL input, chapter split toggle, and status updates
- **Automatic dependency management**: Downloads and updates `yt-dlp`, `ffmpeg`, and `ffprobe`
- **Self auto-updates**: Downloads, installs, and restarts into the latest app build when one is available
- **Startup dependency updates**: Checks and updates managed dependency versions when the app starts
- **Chapter splitting**: Optional chapter splitting for videos with chapters, enabled by default
- **Terminal integration**: Opens Terminal on macOS or Command Prompt on Windows for download progress
- **Folder integration**: Opens the download folder when the download completes
- **Smart organization**: Downloads are organized by video ID under `~/Downloads/`

## Dependencies

The application stores managed tools in `~/.kyte-yt-downloader/` and reuses them across sessions.

- **yt-dlp**: Latest platform binary from [yt-dlp GitHub releases](https://github.com/yt-dlp/yt-dlp/releases)
- **macOS ffmpeg/ffprobe**: Latest snapshot ZIPs from [Martin Riedl's FFmpeg Build Server](https://ffmpeg.martin-riedl.de/)
- **Windows ffmpeg/ffprobe**: Latest Windows x64 ZIP from [BtbN FFmpeg Builds](https://github.com/BtbN/FFmpeg-Builds/releases/tag/latest)
- **Metadata**: `dependencies.json` records the version/source used for startup update checks

## GitHub Actions Builds

Pushing to `main` runs `.github/workflows/build-binaries.yml` automatically.

- Builds a macOS arm64 app bundle ZIP
- Builds a Windows x64 executable ZIP
- Uploads both files as GitHub Actions artifacts
- Publishes both files to a stable `latest` GitHub Release
- Embeds `AppVersion` and `AppCommit` so the app can detect and install newer `main` builds

## Self Updates

On startup, release builds check the `latest` GitHub Release. If the release points at a newer commit, the app downloads the matching platform ZIP, stages it under `~/.kyte-yt-downloader/app-update/`, starts a small replacement helper, exits, and reopens the updated app.

On macOS, the helper removes the quarantine attribute from the staged and installed app with:

```bash
xattr -dr com.apple.quarantine "Kyte YouTube Downloader.app"
```

If the app is installed in a directory the user cannot write to, the app leaves the current install in place and shows a manual download link instead.

## Building Locally

### Prerequisites

- Go 1.24.0 or later
- macOS for local macOS bundle builds
- Windows x64 or a supported cross-compile environment for Windows builds

### Build Commands

```bash
go mod tidy
./build-app.sh macos
./build-app.sh windows
./build-app.sh all
```

The macOS command creates `Kyte YouTube Downloader.app`. The Windows command creates `dist/Kyte YouTube Downloader.exe`.

You can also build a raw development binary:

```bash
go build -o kyte-yt-downloader
```

## Usage

### macOS

1. Open `Kyte YouTube Downloader.app` or copy it to `/Applications/`.
2. Paste a YouTube URL.
3. Choose whether to split chapters.
4. Click **Download**.
5. Monitor progress in the Terminal window.

### Windows x64

1. Run `Kyte YouTube Downloader.exe`.
2. Paste a YouTube URL.
3. Choose whether to split chapters.
4. Click **Download**.
5. Monitor progress in the Command Prompt window.

## Download Command

The application executes `yt-dlp` with these parameters:

```bash
"~/.kyte-yt-downloader/yt-dlp_macos or yt-dlp.exe" \
  "--ffmpeg-location" "~/.kyte-yt-downloader/" \
  "-S" "res,ext:mp4:m4a" \
  "--recode" "mp4" \
  "-P" "~/Downloads/youtube_VIDEO_ID" \
  "URL"
```

With chapter splitting enabled, it also passes:

```bash
"--split-chapters" \
"-o" "chapter:%(section_number)s %(section_title)s.%(ext)s"
```

## File Organization

- **Dependencies**: `~/.kyte-yt-downloader/`
- **macOS tools**: `yt-dlp_macos`, `ffmpeg`, `ffprobe`
- **Windows tools**: `yt-dlp.exe`, `ffmpeg.exe`, `ffprobe.exe`
- **Dependency metadata**: `dependencies.json`
- **Downloads**: `~/Downloads/youtube_VIDEO_ID/`

## Troubleshooting

### Reset Dependencies

```bash
rm -rf ~/.kyte-yt-downloader
```

### Dependency Download Failed

Check your internet connection and verify GitHub and the FFmpeg build provider for your platform are reachable.

### Terminal or Command Prompt Does Not Open

On macOS, ensure Terminal.app automation permissions are allowed. On Windows, ensure Command Prompt is not blocked by policy or antivirus software.

## License

Built with:

- [yt-dlp](https://github.com/yt-dlp/yt-dlp)
- [FFmpeg](https://ffmpeg.org/)
- [GioUI](https://gioui.org/)
