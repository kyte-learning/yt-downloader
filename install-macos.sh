#!/bin/bash

set -euo pipefail

APP_NAME="Kyte YouTube Downloader"
APP_BUNDLE="${APP_NAME}.app"
ASSET_NAME="kyte-yt-downloader-macos-arm64.zip"
DOWNLOAD_URL="https://github.com/kyte-learning/yt-downloader/releases/latest/download/${ASSET_NAME}"
INSTALL_DIR="/Applications"
INSTALL_PATH="${INSTALL_DIR}/${APP_BUNDLE}"

if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "This installer only supports macOS."
    exit 1
fi

if [[ "$(uname -m)" != "arm64" ]]; then
    echo "This installer currently supports Apple Silicon macOS only."
    exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

echo "Downloading ${APP_NAME}..."
curl -fL --retry 3 --retry-delay 2 -o "${TMP_DIR}/${ASSET_NAME}" "${DOWNLOAD_URL}"

echo "Extracting app..."
ditto -x -k "${TMP_DIR}/${ASSET_NAME}" "${TMP_DIR}"

APP_SOURCE="$(find "${TMP_DIR}" -maxdepth 2 -type d -name "${APP_BUNDLE}" -print -quit)"
if [[ -z "${APP_SOURCE}" ]]; then
    echo "Could not find ${APP_BUNDLE} in the downloaded archive."
    exit 1
fi

echo "Removing macOS quarantine attribute..."
xattr -dr com.apple.quarantine "${APP_SOURCE}" 2>/dev/null || true

INSTALL_CMD=()
if [[ ! -w "${INSTALL_DIR}" ]]; then
    INSTALL_CMD=(sudo)
fi

echo "Installing to ${INSTALL_PATH}..."
osascript -e "quit app \"${APP_NAME}\"" >/dev/null 2>&1 || true
"${INSTALL_CMD[@]}" rm -rf "${INSTALL_PATH}"
"${INSTALL_CMD[@]}" ditto "${APP_SOURCE}" "${INSTALL_PATH}"
"${INSTALL_CMD[@]}" xattr -dr com.apple.quarantine "${INSTALL_PATH}" 2>/dev/null || true

echo "Opening ${APP_NAME}..."
open "${INSTALL_PATH}"

echo "Installed ${APP_NAME}."
