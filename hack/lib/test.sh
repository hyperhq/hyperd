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

hyper::test::run_attached_pod() {
  hyper run --rm -a -p $1
}

hyper::test::insert_file() {
  res=0
  count=5
  rslv=$(cat /etc/resolv.conf | md5sum)
  while read checksum file ; do 
    echo "$file: $checksum"
    case $file in
      resolv.conf)
        [ "x$checksum" = "x$rslv" ] || res=1
        $((count--))
        ;;
      logo.png)
        [ "x$checksum" = "x9f5e23e42360a072b7b597ce666dc3e1" ] || res=1
        $((count--))
        ;;
      t1)
        [ "x$checksum" = "xd41d8cd98f00b204e9800998ecf8427e" ] || res=1
        $((count--))
        ;;
      t2)
        [ "x$checksum" = "xe75fa12f18625654acd5aaa207fc78c5" ] || res=1
        $((count--))
        ;;
      t3)
        [ "x$checksum" = "x5c9b82816aafe7b8c3b1aa910b736355" ] || res=1
        $((count--))
        ;;
      *)
        ;;
    esac
  done < <(hyper::test::run_attached_pod ${HYPER_ROOT}/hack/pods/insert-file.pod)
  return $res && $count
}
