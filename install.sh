#!/bin/sh
# anchr installer — curl -fsSL https://anchr.jackjakarta.xyz/install.sh | sh
#
# Environment variables:
#   ANCHR_VERSION      — specific version to install (e.g. v0.1.0), default: latest
#   ANCHR_INSTALL_DIR  — custom install directory, default: /usr/local/bin or ~/.local/bin
#   GITHUB_TOKEN       — GitHub token for API rate limit avoidance

set -e

main() {
    setup_colors
    check_dependencies
    detect_platform
    resolve_version

    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT INT TERM

    download
    verify_checksum
    install_binary

    printf "\n"
    printf "%s  anchr %s installed successfully!%s\n" "$GREEN" "$VERSION" "$RESET"
    printf "\n"
    printf "  Location: %s\n" "$INSTALL_PATH"
    printf "  Version:  %s\n" "$VERSION"
    printf "\n"
    printf "  To get started, create a config file at %s/.config/anchr/config.yaml%s\n" "$HOME" ""
    printf "  with your S3 bucket configuration, then run:\n"
    printf "\n"
    printf "    $ anchr\n"
    printf "\n"
}

setup_colors() {
    if [ -t 1 ]; then
        RED='\033[0;31m'
        GREEN='\033[0;32m'
        YELLOW='\033[0;33m'
        BLUE='\033[0;34m'
        BOLD='\033[1m'
        RESET='\033[0m'
    else
        RED=''
        GREEN=''
        YELLOW=''
        BLUE=''
        BOLD=''
        RESET=''
    fi
}

info() {
    printf "%s[info]%s %s\n" "$BLUE" "$RESET" "$1"
}

warn() {
    printf "%s[warn]%s %s\n" "$YELLOW" "$RESET" "$1"
}

error() {
    printf "%s[error]%s %s\n" "$RED" "$RESET" "$1" >&2
    exit 1
}

check_dependencies() {
    HAS_CURL=false
    HAS_WGET=false

    if command -v curl >/dev/null 2>&1; then
        HAS_CURL=true
    fi
    if command -v wget >/dev/null 2>&1; then
        HAS_WGET=true
    fi

    if [ "$HAS_CURL" = false ] && [ "$HAS_WGET" = false ]; then
        error "Either curl or wget is required but neither was found."
    fi

    if ! command -v tar >/dev/null 2>&1; then
        error "tar is required but was not found."
    fi

    if ! command -v uname >/dev/null 2>&1; then
        error "uname is required but was not found."
    fi

    HAS_SHA=false
    SHA_CMD=""
    if command -v sha256sum >/dev/null 2>&1; then
        HAS_SHA=true
        SHA_CMD="sha256sum"
    elif command -v shasum >/dev/null 2>&1; then
        HAS_SHA=true
        SHA_CMD="shasum -a 256"
    fi
}

detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        linux)  OS="linux" ;;
        darwin) OS="darwin" ;;
        *)      error "Unsupported operating system: $OS (supported: linux, darwin)" ;;
    esac

    case "$ARCH" in
        x86_64|amd64)   ARCH="amd64" ;;
        aarch64|arm64)  ARCH="arm64" ;;
        *)              error "Unsupported architecture: $ARCH (supported: amd64, arm64)" ;;
    esac

    info "Detected platform: ${OS}/${ARCH}"
}

resolve_version() {
    if [ -n "$ANCHR_VERSION" ]; then
        VERSION="$ANCHR_VERSION"
        info "Using specified version: ${VERSION}"
        return
    fi

    info "Fetching latest release version..."

    AUTH_HEADER=""
    if [ -n "$GITHUB_TOKEN" ]; then
        AUTH_HEADER="Authorization: token $GITHUB_TOKEN"
    fi

    API_URL="https://api.github.com/repos/jackjakarta/anchr/releases/latest"

    if [ "$HAS_CURL" = true ]; then
        if [ -n "$AUTH_HEADER" ]; then
            RESPONSE=$(curl -fsSL -H "$AUTH_HEADER" "$API_URL" 2>/dev/null) || error "Failed to fetch latest release from GitHub API."
        else
            RESPONSE=$(curl -fsSL "$API_URL" 2>/dev/null) || error "Failed to fetch latest release from GitHub API."
        fi
    else
        if [ -n "$AUTH_HEADER" ]; then
            RESPONSE=$(wget -qO- --header="$AUTH_HEADER" "$API_URL" 2>/dev/null) || error "Failed to fetch latest release from GitHub API."
        else
            RESPONSE=$(wget -qO- "$API_URL" 2>/dev/null) || error "Failed to fetch latest release from GitHub API."
        fi
    fi

    VERSION=$(printf '%s' "$RESPONSE" | grep '"tag_name"' | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

    if [ -z "$VERSION" ]; then
        error "Could not determine latest version. Set ANCHR_VERSION to install a specific version."
    fi

    info "Latest version: ${VERSION}"
}

download() {
    ARCHIVE="anchr-${OS}-${ARCH}.tar.gz"
    DOWNLOAD_URL="https://github.com/jackjakarta/anchr/releases/download/${VERSION}/${ARCHIVE}"

    info "Downloading ${ARCHIVE}..."

    if [ "$HAS_CURL" = true ]; then
        curl -fsSL -o "${TMPDIR}/${ARCHIVE}" "$DOWNLOAD_URL" || error "Failed to download ${DOWNLOAD_URL}"
    else
        wget -qO "${TMPDIR}/${ARCHIVE}" "$DOWNLOAD_URL" || error "Failed to download ${DOWNLOAD_URL}"
    fi
}

verify_checksum() {
    CHECKSUMS_URL="https://github.com/jackjakarta/anchr/releases/download/${VERSION}/checksums.txt"

    if [ "$HAS_CURL" = true ]; then
        curl -fsSL -o "${TMPDIR}/checksums.txt" "$CHECKSUMS_URL" 2>/dev/null
    else
        wget -qO "${TMPDIR}/checksums.txt" "$CHECKSUMS_URL" 2>/dev/null
    fi

    if [ ! -f "${TMPDIR}/checksums.txt" ] || [ ! -s "${TMPDIR}/checksums.txt" ]; then
        warn "Checksums file not available, skipping verification."
        return
    fi

    if [ "$HAS_SHA" = false ]; then
        warn "No SHA256 tool found (sha256sum or shasum), skipping checksum verification."
        return
    fi

    EXPECTED=$(grep "$ARCHIVE" "${TMPDIR}/checksums.txt" | awk '{print $1}')

    if [ -z "$EXPECTED" ]; then
        warn "No checksum found for ${ARCHIVE}, skipping verification."
        return
    fi

    ACTUAL=$(cd "$TMPDIR" && $SHA_CMD "$ARCHIVE" | awk '{print $1}')

    if [ "$EXPECTED" != "$ACTUAL" ]; then
        error "Checksum verification failed!\n  Expected: ${EXPECTED}\n  Actual:   ${ACTUAL}"
    fi

    info "Checksum verified."
}

install_binary() {
    tar -xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"

    if [ ! -f "${TMPDIR}/anchr" ]; then
        error "Archive did not contain 'anchr' binary."
    fi

    chmod +x "${TMPDIR}/anchr"

    # Determine install directory
    if [ -n "$ANCHR_INSTALL_DIR" ]; then
        INSTALL_DIR="$ANCHR_INSTALL_DIR"
    elif [ -w "/usr/local/bin" ]; then
        INSTALL_DIR="/usr/local/bin"
    elif command -v sudo >/dev/null 2>&1; then
        INSTALL_DIR="/usr/local/bin"
    else
        INSTALL_DIR="$HOME/.local/bin"
    fi

    INSTALL_PATH="${INSTALL_DIR}/anchr"

    # Create directory if it doesn't exist
    if [ ! -d "$INSTALL_DIR" ]; then
        mkdir -p "$INSTALL_DIR" 2>/dev/null || {
            if command -v sudo >/dev/null 2>&1; then
                sudo mkdir -p "$INSTALL_DIR"
            else
                error "Cannot create directory: ${INSTALL_DIR}"
            fi
        }
    fi

    # Install the binary
    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMPDIR}/anchr" "$INSTALL_PATH"
    elif command -v sudo >/dev/null 2>&1; then
        info "Elevated permissions required to install to ${INSTALL_DIR}"
        sudo mv "${TMPDIR}/anchr" "$INSTALL_PATH"
    else
        error "Cannot write to ${INSTALL_DIR} and sudo is not available. Set ANCHR_INSTALL_DIR to a writable directory."
    fi

    # macOS: remove quarantine attribute
    if [ "$OS" = "darwin" ]; then
        xattr -d com.apple.quarantine "$INSTALL_PATH" 2>/dev/null || true
    fi

    # Warn if install dir is not in PATH
    case ":$PATH:" in
        *":${INSTALL_DIR}:"*) ;;
        *)
            warn "${INSTALL_DIR} is not in your PATH."
            printf "  Add it by running:\n"
            printf "    export PATH=\"%s:\$PATH\"\n" "$INSTALL_DIR"
            printf "\n"
            ;;
    esac
}

main
