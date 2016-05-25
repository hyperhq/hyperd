#!/bin/bash
which yum>/dev/null
if [[ $? != 0 ]]; then
  sudo apt-get install -y unzip
else
  sudo yum install -y unzip
fi


# Install protoc
cd /tmp
curl -sSL https://github.com/google/protobuf/releases/download/v3.0.0-beta-3/protoc-3.0.0-beta-3-linux-x86_64.zip -o protoc-3.0.0-beta-3-linux-x86_64.zip
unzip protoc-3.0.0-beta-3-linux-x86_64.zip
sudo mv protoc /usr/bin/protoc

# Install protoc-gen-go
go get -a github.com/golang/protobuf/protoc-gen-go
echo "gRPC installed success."
