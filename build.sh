#!/bin/bash
set -e
git_hash=$(git rev-parse --short HEAD 2>/dev/null)
version="1.5.0-${git_hash}"
version="${BUILD_VERSION:-${version}}"
echo "Build: AutoInstall-${version}"
go build -ldflags "-X main.gitversion=${version}" -o "dist/${BUILD_NAME}" .
ls dist
