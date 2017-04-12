#!/bin/bash

uv=$(dirname $0)/update-govendor.sh

exec $uv github.com/hyperhq/runv/...

