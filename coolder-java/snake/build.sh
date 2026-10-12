#!/bin/sh
set -eu
mkdir -p build
go build -o build/snake ./src
