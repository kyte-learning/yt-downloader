#!/bin/bash

set -euo pipefail

TARGET="${1:-macos}"
APP_VERSION="${APP_VERSION:-local}"
APP_COMMIT="${APP_COMMIT:-$(git rev-parse HEAD 2>/dev/null || echo dev)}"
LDFLAGS="-s -w -X main.AppVersion=${APP_VERSION} -X main.AppCommit=${APP_COMMIT}"

build_macos() {
    echo "Building macOS app bundle..."
    rm -rf "Kyte YouTube Downloader.app"
    go run gioui.org/cmd/gogio@latest \
        -target macos \
        -arch arm64 \
        -appid com.kytelearning.ytdownloader \
        -icon icon.png \
        -ldflags "${LDFLAGS}" \
        -o "Kyte YouTube Downloader.app" \
        .
    echo "macOS app bundle created: Kyte YouTube Downloader.app"
}

build_windows() {
    echo "Building Windows x64 executable..."
    mkdir -p dist
    go run gioui.org/cmd/gogio@latest \
        -target windows \
        -arch amd64 \
        -appid com.kytelearning.ytdownloader \
        -icon icon.png \
        -ldflags "${LDFLAGS}" \
        -o "dist/Kyte YouTube Downloader.exe" \
        .
    echo "Windows executable created: dist/Kyte YouTube Downloader.exe"
}

case "${TARGET}" in
    macos)
        build_macos
        ;;
    windows)
        build_windows
        ;;
    all)
        build_macos
        build_windows
        ;;
    *)
        echo "Usage: $0 [macos|windows|all]"
        exit 1
        ;;
esac
