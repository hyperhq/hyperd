#!/bin/bash
find . -name "*.go" | grep -v Godeps | xargs gofmt -s -w
