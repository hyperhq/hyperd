#!/bin/bash

# This command checks that the built commands can function together for
# simple scenarios.  It does not require Docker so it can run in travis.

set -o errexit
set -o nounset
set -o pipefail

HYPER_ROOT=$(readlink -f $(dirname "${BASH_SOURCE}")/..)
source "${HYPER_ROOT}/hack/lib/init.sh"

function cleanup()
{
  stop_hyperd

  rm -rf "${HYPER_TEMP}"

  hyper::log::status "Clean up complete"
}

# Executes curl against the proxy. $1 is the path to use, $2 is the desired
# return code. Prints a helpful message on failure.
function check-curl-access-code()
{
  local status
  local -r address=$1
  local -r desired=$2
  local -r full_address="${API_HOST}:${API_PORT}${address}"
  status=$(curl -w "%{http_code}" --silent --output /dev/null "${full_address}")
  if [ "${status}" == "${desired}" ]; then
    return 0
  fi
  echo "For address ${full_address}, got ${status} but wanted ${desired}"
  return 1
}

function start_hyperd()
{
  config="$1"
  sdriver="$2"
  if [[ -z "${config}" ]]; then
    echo "no configuration file provided!"
    return 1
  fi
  # Start hyperd
  hyper::log::status "Starting hyperd"
  sudo "${HYPER_OUTPUT_HOSTBIN}/hyperd" \
    --host="tcp://127.0.0.1:${API_PORT}" \
    --v=3 \
    --config="${config}" 1>&2 &
  HYPERD_PID=$!

  if [ "$sdriver" == "devicemapper" ]; then
    echo "waiting hyperd start up"
    sleep 600
  fi
  hyper::util::wait_for_url "http://127.0.0.1:${API_PORT}/info" "hyper-info"
  # Check hyperctl
  hyper::log::status "Running hyperctl command with no options"
  "${HYPER_OUTPUT_HOSTBIN}/hyperctl"
}

function stop_hyperd()
{
  if ps --ppid ${HYPERD_PID} > /dev/null 2>&1  ; then
    read  HYPERD_PID other < <(ps --ppid ${HYPERD_PID}|grep hyperd)
  fi
  [[ -n "${HYPERD_PID-}" ]] && sudo kill "${HYPERD_PID}" 1>&2 2>/dev/null
  t=1
  while ps -p ${HYPERD_PID} >/dev/null 2>&1 ; do
    echo "wait hyperd(${HYPERD_PID}) stop"
    sleep 1
    [ $((t++)) -ge 15 ] && break
  done
  HYPERD_PID=
}

function setup_libvirtd() {
    (cat <<EOF
user = "root"
group = "root"
clear_emulator_capabilities = 0
EOF
) | sudo tee /etc/libvirt/qemu.conf
    sudo /etc/init.d/libvirt-bin restart
}

hyper::util::trap_add cleanup EXIT SIGINT
hyper::util::ensure-temp-dir

API_PORT=${API_PORT:-12345}
API_HOST=${API_HOST:-127.0.0.1}
HYPER_TEMP=${HYPER_TEMP:-/tmp}
# ensure ~/.hyper/config isn't loaded by tests
HOME="${HYPER_TEMP}"

if [ "x${HYPER_RUNTIME:-}" = "x" ] ; then
  # build hyperstart Kernel and Initrd
  echo "Build Kernel and Initrd by hyperstart"
  hyper::hyperstart::build
else
  KERNEL_PATH=${HYPER_RUNTIME}/kernel
  INITRD_PATH=${HYPER_RUNTIME}/hyper-initrd.img
fi

runTests() {
  execdriver="$1"
  stordriver="$2"
  echo "Testing hyperd with execdriver: $1, storage driver: $2"
  if [ -z "${stordriver}" ]; then
    echo "no storage driver provided!"
    return 1
  else
    # add the execdriver and storage driver items into configuration file
  cat > ${HYPER_TEMP}/config << __EOF__
Kernel=${KERNEL_PATH}
Initrd=${INITRD_PATH}
StorageDriver=${stordriver}
Hypervisor=${execdriver}
gRPCHost=0.0.0.0:22318
__EOF__
  fi

  # setup libvirtd
  [ "$execdriver" = "libvirt" ] && setup_libvirtd

  # Start 'hyperd'
  start_hyperd "${HYPER_TEMP}/config" $stordriver

  # Passing no arguments to 'create' is an error
  ! sudo hyperctl create

  # Passing no arguments to 'info' is right
  sudo hyperctl info

  #######################
  # API status check    #
  #######################

  ######################
  # Image management   #
  ######################

  hyper::test::check_image busybox || hyper::test::pull_image busybox
  hyper::test::check_image busybox

  hyper::test::remove_image busybox
  ! hyper::test::check_image busybox

  hyper::test::pull_image busybox
  hyper::test::check_image busybox

  hyper::test::pull_image "haproxy:1.5"
  hyper::test::check_image "haproxy" "1.5"

  ########################
  # gRPC API integration #
  ########################
  hyper::test::integration

  ###########################
  # POD creation / deletion #
  ###########################

  hyper::test::command
  hyper::test::exitcode
  hyper::test::exec
  hyper::test::hostname
  hyper::test::insert_file
  hyper::test::map_file
  hyper::test::with_volume
  hyper::test::service

  stop_hyperd
}

# devicemapper storage driver takes too much time to init
hyper_storage_drivers=(
  "aufs"
)

hyper_exec_drivers=(
  ""
  "qemu"
  "libvirt"
)
for sdriver in "${hyper_storage_drivers[@]}"; do
  for edriver in "${hyper_exec_drivers[@]}"; do
    runTests "${edriver}" "${sdriver}"
  done
done

hyper::test::clear_all
hyper::log::status "TEST PASSED"
