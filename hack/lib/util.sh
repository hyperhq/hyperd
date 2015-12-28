#!/bin/bash

hyper::util::sortable_date() {
  date "+%Y%m%d-%H%M%S"
}

# this mimics the behavior of linux realpath which is not shipped by default with
# mac OS X 
hyper::util::realpath() {
  [[ $1 = /* ]] && echo "$1" | sed 's/\/$//' || echo "$PWD/${1#./}"  | sed 's/\/$//'
}

hyper::util::wait_for_url() {
  local url=$1
  local prefix=${2:-}
  local wait=${3:-0.5}
  local times=${4:-25}

  which curl >/dev/null || {
    hyper::log::usage "curl must be installed"
    exit 1
  }

  local i
  for i in $(seq 1 $times); do
    local out
    if out=$(curl -fs $url 2>/dev/null); then
      hyper::log::status "On try ${i}, ${prefix}: ${out}"
      return 0
    fi
    sleep ${wait}
  done
  hyper::log::error "Timed out waiting for ${prefix} to answer at ${url}; tried ${times} waiting ${wait} between each"
  return 1
}

# returns a random port
hyper::util::get_random_port() {
  awk -v min=1 -v max=65535 'BEGIN{srand(); print int(min+rand()*(max-min+1))}'
}

# use netcat to check if the host($1):port($2) is free (return 0 means free, 1 means used)
hyper::util::test_host_port_free() {
  local host=$1
  local port=$2
  local success=0
  local fail=1

  which nc >/dev/null || {
    hyper::log::usage "netcat isn't installed, can't verify if ${host}:${port} is free, skipping the check..."
    return ${success}
  }

  if [ ! $(nc -vz "${host} ${port}") ]; then
    hyper::log::status "${host}:${port} is free, proceeding..."
    return ${success}
  else
    hyper::log::status "${host}:${port} is already used"
    return ${fail}
  fi
}

# Example:  hyper::util::trap_add 'echo "in trap DEBUG"' DEBUG
# See: http://stackoverflow.com/questions/3338030/multiple-bash-traps-for-the-same-signal
hyper::util::trap_add() {
  local trap_add_cmd
  trap_add_cmd=$1
  shift

  for trap_add_name in "$@"; do
    local existing_cmd
    local new_cmd

    # Grab the currently defined trap commands for this trap
    existing_cmd=`trap -p "${trap_add_name}" |  awk -F"'" '{print $2}'`

    if [[ -z "${existing_cmd}" ]]; then
      new_cmd="${trap_add_cmd}"
    else
      new_cmd="${existing_cmd};${trap_add_cmd}"
    fi

    # Assign the test
    trap "${new_cmd}" "${trap_add_name}"
  done
}

# Opposite of hyper::util::ensure-temp-dir()
hyper::util::cleanup-temp-dir() {
  rm -rf "${HYPER_TEMP}"
}

# Create a temp dir that'll be deleted at the end of this bash session.
#
# Vars set:
#   HYPER_TEMP
hyper::util::ensure-temp-dir() {
  if [[ -z ${HYPER_TEMP-} ]]; then
    HYPER_TEMP=$(mktemp -d 2>/dev/null || mktemp -d -t hyper.XXXXXX)
    hyper::util::trap_add hyper::util::cleanup-temp-dir EXIT
  fi
}

# This figures out the host platform without relying on golang.  We need this as
# we don't want a golang install to be a prerequisite to building yet we need
# this info to figure out where the final binaries are placed.
hyper::util::host_platform() {
  local host_os
  local host_arch
  case "$(uname -s)" in
    Darwin)
      host_os=darwin
      ;;
    Linux)
      host_os=linux
      ;;
    *)
      hyper::log::error "Unsupported host OS.  Must be Linux or Mac OS X."
      exit 1
      ;;
  esac

  case "$(uname -m)" in
    x86_64*)
      host_arch=amd64
      ;;
    i?86_64*)
      host_arch=amd64
      ;;
    amd64*)
      host_arch=amd64
      ;;
    i?86*)
      host_arch=x86
      ;;
    *)
      hyper::log::error "Unsupported host arch. Must be x86_64, 386 or arm."
      exit 1
      ;;
  esac
  echo "${host_os}/${host_arch}"
}

hyper::util::find-binary() {
  local lookfor="${1}"
  local host_platform="$(hyper::util::host_platform)"
  local locations=(
    "${HYPER_ROOT}/_output/dockerized/bin/${host_platform}/${lookfor}"
    "${HYPER_ROOT}/_output/local/bin/${host_platform}/${lookfor}"
    "${HYPER_ROOT}/platforms/${host_platform}/${lookfor}"
  )
  local bin=$( (ls -t "${locations[@]}" 2>/dev/null || true) | head -1 )
  echo -n "${bin}"
}

# Wait for background jobs to finish. Return with
# an error status if any of the jobs failed.
hyper::util::wait-for-jobs() {
  local fail=0
  local job
  for job in $(jobs -p); do
    wait "${job}" || fail=$((fail + 1))
  done
  return ${fail}
}

# ex: ts=2 sw=2 et filetype=sh
