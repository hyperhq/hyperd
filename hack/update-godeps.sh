#!/bin/bash
godep save . github.com/hyperhq/hyperd/cmds/protoc-gen-gogo github.com/hyperhq/hyperd/integration

echo "Godeps updated."
echo "Don't forget remove runv from Godeps."
