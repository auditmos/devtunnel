#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}"

OUTPUT_DIR="${OUTPUT_DIR:-dist}"
mkdir -p "$OUTPUT_DIR"

PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
    "windows/arm64"
)

echo "Building devtunnel ${VERSION} (${COMMIT})"
echo "Output: ${OUTPUT_DIR}"
echo ""

for platform in "${PLATFORMS[@]}"; do
    GOOS="${platform%/*}"
    GOARCH="${platform#*/}"

    output_name="devtunnel-${GOOS}-${GOARCH}"
    if [ "$GOOS" = "windows" ]; then
        output_name="${output_name}.exe"
    fi

    echo "Building ${GOOS}/${GOARCH}..."
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build \
        -ldflags "$LDFLAGS" \
        -o "${OUTPUT_DIR}/${output_name}" \
        ./cmd/devtunnel
done

echo ""
echo "Generating checksums..."
cd "$OUTPUT_DIR"
shasum -a 256 devtunnel-* > checksums.txt
cat checksums.txt

echo ""
echo "Build complete!"
ls -lh devtunnel-*
