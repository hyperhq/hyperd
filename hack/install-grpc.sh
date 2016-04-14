#!/bin/bash
which yum>/dev/null || sudo apt-get install -y unzip && sudo yum install -y unzip

# Install protoc
curl -sSL https://github.com/google/protobuf/releases/download/v3.0.0-beta-2/protoc-3.0.0-beta-2-linux-x86_64.zip -o protoc-3.0.0-beta-2-linux-x86_64.zip
unzip -fo protoc-3.0.0-beta-2-linux-x86_64.zip -d /usr/local/bin
# Install protoc-gen-go
go get -a github.com/golang/protobuf/protoc-gen-go
echo "gRPC installed success."