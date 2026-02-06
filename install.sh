#!/bin/bash

set -e # Exit immediately if a command exits with a non-zero status

echo "🚀 Starting installation of brrtfetch (dev branch)..."

# 1. Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed. Please install Go first."
    exit 1
fi

# 2. Create a temporary directory
TEMP_DIR=$(mktemp -d)
cleanup() {
    rm -rf "$TEMP_DIR"
}
trap cleanup EXIT

# 3. Clone the dev branch
echo "⬇️  Cloning dev branch..."
cd "$TEMP_DIR"
# The -b flag specifies the branch, --depth 1 makes it faster (no history)
git clone -b dev --depth 1 https://github.com/ferrebarrat/brrtfetch.git 
cd brrtfetch/go

# 4. Build
echo "🔨 Building binary..."
go build -o brrtfetch . 

# 5. Install to /usr/local/bin
echo "📦 Installing to /usr/local/bin (requires sudo)..."
if [ -w /usr/local/bin ]; then
    mv brrtfetch /usr/local/bin/brrtfetch
else
    sudo mv brrtfetch /usr/local/bin/brrtfetch
fi

# 6. Make executable (just in case)
sudo chmod +x /usr/local/bin/brrtfetch

echo "✅ Success! You can now run 'brrtfetch' in your terminal."