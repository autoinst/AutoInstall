#!/bin/bash
set -e
git_hash=$(git rev-parse --short HEAD 2>/dev/null)
version="1.1.0-${git_hash}"
echo "Build: AutoInstall-${version}"
go build -ldflags "-X main.version=${version}" -o dist/${build_name} main.go
ls dist