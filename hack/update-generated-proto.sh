#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

if [[ -z "$(which protoc)" || "$(protoc --version)" != "libprotoc 3.0."* ]]; then
  echo "Generating protobuf requires protoc 3.0.0-beta1 or newer. Please download and"
  echo "install the platform appropriate Protobuf package for your OS: "
  echo
  echo "  https://github.com/google/protobuf/releases"
  echo
  echo "WARNING: Protobuf changes are not being validated"
  exit 1
fi

HYPER_ROOT=$(dirname "${BASH_SOURCE}")/..
PROTO_ROOT=${HYPER_ROOT}/types
export PATH=${HYPER_ROOT}/cmds/protoc-gen-gogo:$PATH

function cleanup {
	rm -f ${PROTO_ROOT}/types.pb.go.bak
}

trap cleanup EXIT

hack/build-protoc-gen-gogo.sh
protoc -I${PROTO_ROOT} --gogo_out=plugins=grpc:${PROTO_ROOT} ${PROTO_ROOT}/types.proto
echo "Generated types from proto updated."
