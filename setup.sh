#!/bin/bash
# Run this after extracting the project to resolve the WinRM dependency
set -e

echo "=== Resolving WinRM dependency ==="
go get github.com/masterzen/winrm@latest

echo "=== Tidying module ==="
go mod tidy

echo "=== Building ==="
go build -o admin_tool .

echo "=== Done! Run ./admin_tool to start ==="
