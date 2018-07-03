#!/bin/bash

set -e

cidir=$(dirname "$0")

source "${cidir}/lib.sh"

clone_repo "github.com/kata-containers/runtime"

${cidir}/setup_env_ubuntu.sh || true 
${cidir}/install_kata_image.sh && ${cidir}/install_kata_kernel.sh && ${cidir}/install_qemu.sh
