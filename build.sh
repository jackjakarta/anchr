#!/bin/bash

VERSION="${1:-dev}"
LDFLAGS="-s -w -X main.version=${VERSION}"

GOOS=linux GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o anchr && tar -czf anchr-linux-amd64.tar.gz anchr && \
GOOS=linux GOARCH=arm64 go build -ldflags "${LDFLAGS}" -o anchr && tar -czf anchr-linux-arm64.tar.gz anchr && \
GOOS=darwin GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o anchr && tar -czf anchr-darwin-amd64.tar.gz anchr && \
GOOS=darwin GOARCH=arm64 go build -ldflags "${LDFLAGS}" -o anchr && tar -czf anchr-darwin-arm64.tar.gz anchr

rm anchr
echo "Build complete!"
echo "Checksums:"

shasum -a 256 anchr-darwin-arm64.tar.gz
shasum -a 256 anchr-darwin-amd64.tar.gz

shasum -a 256 anchr-linux-arm64.tar.gz
shasum -a 256 anchr-linux-amd64.tar.gz

