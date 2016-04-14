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
_tmp="${HYPER_ROOT}/_tmp"

cleanup() {
  rm -rf "${_tmp}"
}

trap "cleanup" EXIT SIGINT

mkdir -p ${_tmp}
protoc --go_out=plugins=grpc:${_tmp} types/types.proto

ret=0
diff -I "gzipped FileDescriptorProto" -I "0x" -Naupr ${_tmp}/types/types.pb.go types/types.pb.go || ret=$?
if [[ $ret -eq 0 ]]; then
    echo "Generated types from proto up to date."
else
    echo "Generated types from proto is out of date. Please run hack/update-generated-proto.sh"
    exit 1
fi
