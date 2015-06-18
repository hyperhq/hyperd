#!/bin/bash
find * -name "*.go" |grep -v Godeps|while read f; do echo "go fmt hyper/${f%/*.go}";done |sort |uniq|bash
