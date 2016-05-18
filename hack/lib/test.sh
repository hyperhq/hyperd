#!/bin/bash

hyper::test::clear_all() {
  # add the clear items
  return 0
}

hyper::test::pull_image() {
  sudo hyperctl pull $@
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
  done < <(sudo hyperctl images)
  return $res
}

hyper::test::remove_image() {
  sudo hyperctl rmi $@
}

hyper::test::exitcode() {
  echo "Pod exit code test"
  res=$(sudo hyperctl run --rm busybox sh -c "exit 17" > /dev/null 2>&1 ; echo $?)
  echo "should return 17, return: $res"
  test $res -eq 17
}

hyper::test::exec() {
  echo "Pod exec and exit code test"
  id=$(sudo hyperctl run -t -d busybox /bin/sh | sed -ne "s/POD id is \(pod-[0-9A-Za-z]\{1,\}\)/\1/p")
  echo "test pod ID is $id"
  res=$(sudo hyperctl exec $id sh -c "exit 37" > /dev/null 2>&1 ; echo $?)
  echo "should return 37, return: $res"
  test $res -eq 37
  sudo hyperctl rm $id
}

hyper::test::run_pod() {
  sudo hyperctl run --rm -p $1
}

hyper::test::run_attached_pod() {
  sudo hyperctl run --rm -a -p $1
}

hyper::test::hostname() {
  hname=$(hyper::test::run_attached_pod ${HYPER_ROOT}/hack/pods/hostname.pod | tr -d '\r')
  echo "hostname is ${hname}, expected myname"
  [ "${hname}x" != "mynamex" ] && return 1

  echo "check long hostname"
  hyper::test::run_attached_pod ${HYPER_ROOT}/hack/pods/hostname-err-long.pod && return 1
  echo "check invalid hostname"
  hyper::test::run_attached_pod ${HYPER_ROOT}/hack/pods/hostname-err-char.pod && return 1

  return 0
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

hyper::test::with_volume() {
  mkdir -p  ${HYPER_TEMP}/tmp
  echo 'hello, world' > ${HYPER_TEMP}/tmp/with-volume-test-1
  sed -e "s|TMPDIR|${HYPER_TEMP}/tmp|" ${HYPER_ROOT}/hack/pods/with-volume.pod > ${HYPER_TEMP}/with-volume.pod
  hyper::test::run_attached_pod ${HYPER_TEMP}/with-volume.pod | grep OK
  echo "check the out file: ${HYPER_TEMP}/tmp/with-volume-test-2"
  cat ${HYPER_TEMP}/tmp/with-volume-test-2
  grep -q OK ${HYPER_TEMP}/tmp/with-volume-test-2
}

hyper::test::service() {
    hyper::test::run_attached_pod ${HYPER_ROOT}/hack/pods/service.pod
}

hyper::test::command() {
  id=$(sudo hyperctl run -t -d gcr.io/google_containers/etcd:2.0.9 /usr/local/bin/etcd | sed -ne "s/POD id is \(pod-[0-9A-Za-z]\{1,\}\)/\1/p")
  sudo hyperctl rm $id
}

hyper::test::integration() {
  export GOPATH=${HYPER_ROOT}/Godeps/_workspace:$GOPATH
  go test github.com/hyperhq/hyperd/integration -check.vv -v
}
