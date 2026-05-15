# Kyte YouTube Downloader

A modern GUI YouTube downloader built with GioUI for macOS. This application automatically downloads and manages `yt-dlp`, `ffmpeg`, and `ffprobe` dependencies, then provides a simple interface to download YouTube videos.

## Features

- **Native macOS App**: Proper app bundle with custom icon and app switching support
- **Clean GioUI Interface**: Modern, native-looking GUI
- **Automatic Dependency Management**: Downloads yt-dlp, ffmpeg, and ffprobe automatically
- **Chapter Splitting**: Optional chapter splitting for videos with chapters (enabled by default)
- **Terminal Integration**: Opens downloads in Terminal for progress monitoring
- **Finder Integration**: Automatically opens download folder when complete
- **Smart Organization**: Downloads organized by video ID in ~/Downloads/

## Dependencies

The application automatically downloads and manages these tools:

- **yt-dlp**: Latest macOS binary from [GitHub releases](https://github.com/yt-dlp/yt-dlp/releases)
- **ffmpeg**: Latest snapshot from [evermeet.cx](https://evermeet.cx/ffmpeg/)
- **ffprobe**: Latest snapshot from [evermeet.cx](https://evermeet.cx/ffmpeg/)

All dependencies are stored in `~/.kyte-yt-downloader/` and reused across sessions.

## Building

### Prerequisites

- Go 1.24.0 or later
- macOS (tested on macOS 14+)
- Xcode Command Line Tools

### Build Steps

#### Option 1: macOS App Bundle (Recommended)

1. Clone or download the project
2. Navigate to the project directory
3. Install gogio and build the app bundle:

```bash
go mod tidy
go install gioui.org/cmd/gogio@latest
./build-app.sh
```

4. Install the app:
```bash
cp -r "Kyte YouTube Downloader.app/Kyte YouTube Downloader_arm64.app" /Applications/
```

#### Option 2: Command Line Binary

```bash
go mod tidy
go build -o kyte-yt-downloader
```

## Usage

### macOS App Bundle
1. **Launch from Applications**: Open from Launchpad or Applications folder
2. **Launch from Finder**: Double-click the app bundle
3. **Launch from Terminal**: `open "/Applications/Kyte YouTube Downloader.app"`

### Command Line Binary
1. **Launch the Application**:
   ```bash
   ./yt-downloader
   ```

2. **Enter YouTube URL**: Paste any YouTube URL in the input field

3. **Choose Options**:
   - "Split Chapters" is enabled by default for videos with chapters
   - Uncheck if you want a single file download instead

4. **Download**: Click the "Download" button

5. **Monitor Progress**: A Terminal window will open showing download progress

6. **Automatic Completion**: When finished, the app will:
   - Open the download folder in Finder
   - Show "Closing terminal in 3 seconds..."
   - Automatically close the Terminal window

## Download Command

The application executes yt-dlp with these parameters:

### Without Chapter Splitting:
```bash
"~/.kyte-yt-downloader/yt-dlp_macos" \
  "--ffmpeg-location" "~/.kyte-yt-downloader/" \
  "-S" "res,ext:mp4:m4a" \
  "--recode" "mp4" \
  "-P" "~/Downloads/youtube_VIDEO_ID" \
  "URL"
```

### With Chapter Splitting:
```bash
"~/.kyte-yt-downloader/yt-dlp_macos" \
  "--ffmpeg-location" "~/.kyte-yt-downloader/" \
  "--split-chapters" \
  "-o" "chapter:%(section_number)s %(section_title)s.%(ext)s" \
  "-S" "res,ext:mp4:m4a" \
  "--recode" "mp4" \
  "-P" "~/Downloads/youtube_VIDEO_ID" \
  "URL"
```

## File Organization

- **Dependencies**: `~/.kyte-yt-downloader/`
  - `yt-dlp_macos`: YouTube downloader binary
  - `ffmpeg`: Video processing binary
  - `ffprobe`: Video analysis binary

- **Downloads**: `~/Downloads/youtube_VIDEO_ID/`
  - Downloads are organized by video ID
  - Each video gets its own folder

## Requirements

- **macOS**: Primary target platform
- **Disk Space**: ~50MB for dependencies + download space
- **Network**: Internet connection for downloading dependencies and videos
- **Permissions**: May require permission to access Downloads folder

## Development

### Architecture

- **GioUI**: Cross-platform GUI framework
- **Concurrent Downloads**: Dependencies downloaded in parallel when missing
- **AppleScript Integration**: Direct Terminal command execution and Finder automation
- **Error Handling**: Comprehensive error messages and status updates

### Adding Features

The codebase is structured for easy extension:

- **Download Logic**: Modify `executeDownload()` for different formats
- **UI Elements**: Add widgets in the `Layout()` method
- **Dependencies**: Add new tools in `ensureDependencies()`

## Troubleshooting

### Common Issues

1. **"Permission Denied"**:
   - Ensure the binary has execute permissions: `chmod +x yt-downloader`

2. **"Dependencies Download Failed"**:
   - Check internet connection
   - Verify GitHub/evermeet.cx are accessible

3. **"ZIP extraction failed"**:
   - Verify the downloaded files are not corrupted
   - Check available disk space in ~/.kyte-yt-downloader/

4. **Terminal doesn't open**:
   - Ensure Terminal.app has proper permissions
   - Check System Preferences > Security & Privacy

### Reset

To reset all dependencies:
```bash
rm -rf ~/.kyte-yt-downloader
```

## License

Built with love for the YouTube downloading community. Uses:
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) for video downloading
- [FFmpeg](https://ffmpeg.org/) for video processing
- [GioUI](https://gioui.org/) for the user interface

## Contributing

Feel free to submit issues and enhancement requests!
