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

hyper::test::exitcode() {
  echo "Pod exit code test"
  res=$(hyper run --rm busybox sh -c "exit 17" > /dev/null 2>&1 ; echo $?)
  echo "should return 17, return: $res"
  test $res -eq 17
}

hyper::test::exec() {
  echo "Pod exec and exit code test"
  id=$(hyper run -t -d busybox /bin/sh | sed -ne "s/POD id is \(pod-[0-9A-Za-z]\{1,\}\)/\1/p")
  echo "test pod ID is $id"
  res=$(hyper exec $id sh -c "exit 37" > /dev/null 2>&1 ; echo $?)
  echo "should return 37, return: $res"
  test $res -eq 37
  hyper rm $id
}

hyper::test::run_pod() {
  hyper run --rm -p $1
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

hyper::test::map_file() {
  rslv=$(cat /etc/resolv.conf | md5sum | cut -f1 -d\ )
  invm=$(hyper::test::run_attached_pod ${HYPER_ROOT}/hack/pods/file-mapping.pod | grep resolv.conf | cut -f1 -d\ )
  echo "$invm vs. $rslv"
  test "a$invm" = "a$rslv"
}

hyper::test::service() {
    hyper::test::run_pod ${HYPER_ROOT}/hack/pods/service.pod
}
