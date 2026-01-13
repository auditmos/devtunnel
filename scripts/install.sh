#!/usr/bin/env bash
set -euo pipefail

REPO="auditmos/devtunnel"
RELEASES_URL="https://github.com/${REPO}/releases"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

detect_os() {
    local os
    os="$(uname -s)"
    case "$os" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        *) echo "unsupported: $os" >&2; exit 1 ;;
    esac
}

detect_arch() {
    local arch
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64) echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *) echo "unsupported: $arch" >&2; exit 1 ;;
    esac
}

get_latest_version() {
    curl -sSL -o /dev/null -w '%{url_effective}' "${RELEASES_URL}/latest" | grep -oE '[^/]+$'
}

download_file() {
    local url="$1"
    local dest="$2"
    if command -v curl &>/dev/null; then
        curl -sSL -o "$dest" "$url"
    elif command -v wget &>/dev/null; then
        wget -qO "$dest" "$url"
    else
        echo "error: curl or wget required" >&2
        exit 1
    fi
}

verify_checksum() {
    local file="$1"
    local expected="$2"
    local actual

    if command -v sha256sum &>/dev/null; then
        actual="$(sha256sum "$file" | awk '{print $1}')"
    elif command -v shasum &>/dev/null; then
        actual="$(shasum -a 256 "$file" | awk '{print $1}')"
    else
        echo "warning: sha256sum/shasum not found, skipping checksum" >&2
        return 0
    fi

    if [ "$actual" != "$expected" ]; then
        echo "error: checksum mismatch" >&2
        echo "expected: $expected" >&2
        echo "actual:   $actual" >&2
        exit 1
    fi
}

main() {
    local os arch version binary_name download_url checksums_url
    local tmpdir expected_checksum

    os="$(detect_os)"
    arch="$(detect_arch)"

    echo "detected: ${os}/${arch}"

    version="${VERSION:-$(get_latest_version)}"
    if [ -z "$version" ]; then
        echo "error: could not determine version" >&2
        exit 1
    fi
    echo "version: ${version}"

    binary_name="devtunnel-${os}-${arch}"
    [ "$os" = "windows" ] && binary_name="${binary_name}.exe"

    download_url="${RELEASES_URL}/download/${version}/${binary_name}"
    checksums_url="${RELEASES_URL}/download/${version}/checksums.txt"

    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT

    echo "downloading ${binary_name}..."
    download_file "$download_url" "${tmpdir}/${binary_name}"

    echo "downloading checksums..."
    download_file "$checksums_url" "${tmpdir}/checksums.txt"

    expected_checksum="$(grep "$binary_name" "${tmpdir}/checksums.txt" | awk '{print $1}')"
    if [ -z "$expected_checksum" ]; then
        echo "warning: no checksum found for ${binary_name}" >&2
    else
        echo "verifying checksum..."
        verify_checksum "${tmpdir}/${binary_name}" "$expected_checksum"
        echo "checksum verified"
    fi

    local dest_name="devtunnel"
    [ "$os" = "windows" ] && dest_name="devtunnel.exe"

    if [ -w "$INSTALL_DIR" ]; then
        mv "${tmpdir}/${binary_name}" "${INSTALL_DIR}/${dest_name}"
        chmod +x "${INSTALL_DIR}/${dest_name}"
        echo "installed: ${INSTALL_DIR}/${dest_name}"
    elif command -v sudo &>/dev/null; then
        echo "installing to ${INSTALL_DIR} (sudo required)..."
        sudo mv "${tmpdir}/${binary_name}" "${INSTALL_DIR}/${dest_name}"
        sudo chmod +x "${INSTALL_DIR}/${dest_name}"
        echo "installed: ${INSTALL_DIR}/${dest_name}"
    else
        mv "${tmpdir}/${binary_name}" "./${dest_name}"
        chmod +x "./${dest_name}"
        echo "installed: ./${dest_name} (add to PATH or move manually)"
    fi

    echo ""
    echo "run 'devtunnel --help' to get started"
}

main "$@"
