#!/bin/sh
set -eu
mkdir -p build
go build -o build/snake ./src
go build -o .webcool-go-app ./src
go test ./...
