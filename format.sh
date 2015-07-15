#!/bin/bash
find * -name "*.go" |grep -v Godeps|while read f; do echo "go fmt github.com/hyperhq/hyper/${f%/*.go}";done |sort |uniq|bash
