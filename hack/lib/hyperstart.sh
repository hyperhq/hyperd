#!/bin/bash

# A set of helpers for building hyperstart for tests

hyper::hyperstart::build() {
  # build hyperstart
  hyper::log::info "clone hyperstart repo"
  git clone https://github.com/hyperhq/hyperstart ${HYPER_TEMP}/hyperstart
  cd ${HYPER_TEMP}/hyperstart
  hyper::log::info "build hyperstart"
  ./autogen.sh
  ./configure
  make

  KERNEL_PATH="${HYPER_TEMP}/hyperstart/build/kernel"
  if [ ! -f ${KERNEL_PATH} ]; then
    return 1
  fi
  INITRD_PATH="${HYPER_TEMP}/hyperstart/build/hyper-initrd.img"
  if [ ! -f ${INITRD_PATH} ]; then
    return 1
  fi
}

hyper::hyperstart::cleanup() {
  rm -rf "${HYPER_TEMP}/hyperstart"
}
