#!/bin/bash
set -e
git_hash=$(git rev-parse --short HEAD 2>/dev/null)
version="1.1.3-${git_hash}"
echo "Build: AutoInstall-${version}"
go build -ldflags "-X main.gitversion=${version}" -o dist/$BUILD_NAME main.go
ls dist