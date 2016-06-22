#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

HYPER_ROOT=$(dirname "${BASH_SOURCE}")/..
PROTO_ROOT=${HYPER_ROOT}/types
_tmp="${HYPER_ROOT}/_tmp"

cleanup() {
  rm -rf "${_tmp}"
}

trap "cleanup" EXIT SIGINT

mkdir -p ${_tmp}
cp ${PROTO_ROOT}/types.pb.go ${_tmp}

ret=0
hack/update-generated-proto.sh
diff -I "gzipped FileDescriptorProto" -I "0x" -Naupr ${_tmp}/types.pb.go ${PROTO_ROOT}/types.pb.go || ret=$?
if [[ $ret -eq 0 ]]; then
    echo "Generated types from proto up to date."
else
    echo "Generated types from proto is out of date. Please run hack/update-generated-proto.sh"
    exit 1
fi
