#!/bin/bash

hyper::test::clear_all() {
  # add the clear items
  return 0
}

hyper::test::pull_image() {
  hyper pull $@
}

hyper::test::check_image() {
  img=$1
  tag="latest"
  res=1
  if [ $# -ge 2 ] ; then
    tag=$2
  fi
  while read i t o ; do
    [ "$img" = "$i" -a "$tag" = "$t" ] && res=0
  done < <(hyper images)
  return $res
}

hyper::test::remove_image() {
  hyper rmi $@
}

