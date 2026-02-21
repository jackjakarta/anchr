#!/bin/bash

VERSION="${1:-dev}"
LDFLAGS="-s -w -X main.version=${VERSION}"

echo "Building anchr ${VERSION}..."
echo ""

echo "[1/4] Building linux/amd64..."
GOOS=linux GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o anchr && tar -czf anchr-linux-amd64.tar.gz anchr

echo "[2/4] Building linux/arm64..."
GOOS=linux GOARCH=arm64 go build -ldflags "${LDFLAGS}" -o anchr && tar -czf anchr-linux-arm64.tar.gz anchr

echo "[3/4] Building darwin/amd64..."
GOOS=darwin GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o anchr && tar -czf anchr-darwin-amd64.tar.gz anchr

echo "[4/4] Building darwin/arm64..."
GOOS=darwin GOARCH=arm64 go build -ldflags "${LDFLAGS}" -o anchr && tar -czf anchr-darwin-arm64.tar.gz anchr

rm anchr
echo ""
echo "Build complete!"
echo ""
echo "Checksums:"

shasum -a 256 anchr-darwin-arm64.tar.gz
shasum -a 256 anchr-darwin-amd64.tar.gz

shasum -a 256 anchr-linux-arm64.tar.gz
shasum -a 256 anchr-linux-amd64.tar.gz

shasum -a 256 anchr-*.tar.gz > checksums.txt
echo ""
echo "Checksums written to checksums.txt"
