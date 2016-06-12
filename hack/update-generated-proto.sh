#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

which protoc>/dev/null
if [[ $? != 0 ]]; then
    echo "Please install grpc from www.grpc.io"
    exit 1
fi

HYPER_ROOT=$(dirname "${BASH_SOURCE}")/..
hack/build-protoc-gen-go.sh
export PATH=${HYPER_ROOT}/Godeps/_workspace/src/github.com/golang/protobuf/protoc-gen-go/:$PATH

protoc --go_out=plugins=grpc:. types/types.proto
echo "Generated types from proto updated."
