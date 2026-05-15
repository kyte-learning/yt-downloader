#!/bin/bash

echo "Building Kyte YouTube Downloader app bundle..."

# Remove old app bundle if it exists
if [ -d "Kyte YouTube Downloader.app" ]; then
    echo "Removing old app bundle..."
    rm -rf "Kyte YouTube Downloader.app"
fi

# Build new app bundle with icon
echo "Creating new app bundle..."
gogio -target macos -arch arm64 -icon icon.png -o "Kyte YouTube Downloader.app" .

if [ $? -eq 0 ]; then
    echo "✅ App bundle created successfully!"
    echo "📱 You can now:"
    echo "   • Open the app: open 'Kyte YouTube Downloader.app'"
    echo "   • Copy to Applications: cp -r 'Kyte YouTube Downloader.app' /Applications/"
    echo "   • Double-click in Finder to launch"
else
    echo "❌ Build failed!"
    exit 1
fi
