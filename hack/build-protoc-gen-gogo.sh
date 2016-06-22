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
HYPER_ROOT_ABS=$(cd ${HYPER_ROOT}; pwd)
export GOPATH=${HYPER_ROOT_ABS}/Godeps/_workspace:$GOPATH
cd ${HYPER_ROOT_ABS}/cmds/protoc-gen-gogo
go build

