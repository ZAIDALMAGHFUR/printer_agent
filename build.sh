#!/usr/bin/env bash
set -e

echo "==> Building Linux binary..."
go build -ldflags "-s -w" -o bin/smart-print-agent-linux ./cmd/agent

echo "==> Building Windows GUI binary (no console window)..."
GOOS=windows GOARCH=amd64 go build -ldflags "-H=windowsgui -s -w" -o bin/smart-print-agent.exe ./cmd/agent

echo "==> Done! Binaries generated in bin/"
ls -lh bin/smart-print-agent*
