#!/bin/bash
set -e

echo "Building videonode for ARM64..."

# Build UI first
echo "Building UI..."
cd ui
pnpm install
pnpm build
cd ..
echo "✅ UI build complete"

# Generate version info
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE=$(date -u +"%Y-%m-%d %H:%M")
BUILD_ID=$(openssl rand -hex 4)
VERSION=${VERSION:-"dev"}

echo "Version info:"
echo "  Git Commit: $GIT_COMMIT"
echo "  Build Date: $BUILD_DATE"
echo "  Build ID: $BUILD_ID"
echo "  Version: $VERSION"

# Ensure buildx is available and create builder if needed
if ! docker buildx ls | grep -q videonode-builder; then
    echo "Creating buildx builder..."
    docker buildx create --use --name videonode-builder
else
    docker buildx use videonode-builder
fi

# Build for ARM64
echo "Starting Docker buildx build for linux/arm64..."
docker buildx build \
    --platform linux/arm64 \
    --output "type=local,dest=./dist/arm64" \
    --build-arg GIT_COMMIT="$GIT_COMMIT" \
    --build-arg BUILD_DATE="$BUILD_DATE" \
    --build-arg BUILD_ID="$BUILD_ID" \
    --build-arg VERSION="$VERSION" \
    -f Dockerfile.arm64 \
    --target export \
    .

if [ -f ./dist/arm64/videonode-arm64 ]; then
    echo "✅ Build successful!"
    echo "Binary available at: ./dist/arm64/videonode-arm64"
    
    # Show binary info
    echo ""
    echo "Binary information:"
    file ./dist/arm64/videonode-arm64
else
    echo "❌ Build failed - binary not found"
    exit 1
fi