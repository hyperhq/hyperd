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
  id=$(sudo hyperctl run -d busybox /bin/sh | sed -ne "s/POD id is \(.*\)/\1/p")
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

hyper::test::nfs_volume() {
  echo "create nfs volume server"
  server=$(sudo hyperctl run --memory 256 -d hyperhq/nfs-server-tester | sed -ne "s/POD id is \(.*\)/\1/p")
  ip=$(sudo hyperctl exec $server ip addr |sed -ne "s|.* \(.*\)/24.*|\1|p")
  sleep 10 # nfs-server-tester takes a bit long to init in hykins
  echo "create nfs volume client"
  sed -e "s/NFSSERVER/$ip/" ${HYPER_ROOT}/hack/pods/nfs-client.pod > ${HYPER_TEMP}/nfs-client.pod
  client=$(sudo hyperctl run -p ${HYPER_TEMP}/nfs-client.pod | sed -ne "s/POD id is \(.*\)/\1/p")
  sudo hyperctl exec $client touch /export/foo
  echo "check nfs file in nfs volume: /export/foo"
  res=$(sudo hyperctl exec $server ls /export | grep foo > /dev/null 2>&1; echo $?)
  echo "clean up nfs client/server"
  sudo hyperctl rm $server $client
  echo "check result should be 0, got: $res"
  test $res -eq 0
}

hyper::test::command() {
  id=$(sudo hyperctl run -t -d gcr.io/google_containers/etcd:2.0.9 /usr/local/bin/etcd | sed -ne "s/POD id is \(.*\)/\1/p")
  sudo hyperctl rm $id
}

hyper::test::integration() {
  go test github.com/hyperhq/hyperd/integration -check.vv -v
}

hyper::test::execvm() {
  echo "Pod execvm echo test"
  id=$(sudo hyperctl run -d busybox /bin/sh | sed -ne "s/POD id is \(.*\)/\1/p")
  echo "test pod ID is $id"
  res=$(sudo hyperctl exec -m $id /sbin/busybox sh -c "echo aaa")
  echo "should return aaa, actual: $res"
  test $res == aaa
  sudo hyperctl rm $id
}

# regression test for #542 and #537
hyper::test::remove_container_with_volume() {
  mkdir -p  ${HYPER_TEMP}/tmp
  sed -e "s|TMPDIR|${HYPER_TEMP}/tmp|" ${HYPER_ROOT}/hack/pods/simple-volume.pod > ${HYPER_TEMP}/simple-volume.pod
  echo "run pod with volume"
  pod_id=$(sudo hyperctl run -d -p ${HYPER_TEMP}/simple-volume.pod | sed -ne "s/POD id is \(.*\)/\1/p")
  echo "test pod ID is $pod_id"
  sudo hyperctl stop $pod_id
  echo "stop pod $pod_id"
  echo "remove container container-with-volume in $pod_id"
  res=$(sudo hyperctl rm -c container-with-volume > /dev/null 2>&1; echo $?)
  sudo hyperctl rm $pod_id
  echo "check result should be 0, got: $res"
  test $res -eq 0
}

hyper::test::imageuser() {
  echo "Pod image user config test"
  # irssi image has "User": "user"
  id=$(sudo hyperctl run -d --env="TERM=xterm" irssi:1 | sed -ne "s/POD id is \(.*\)/\1/p")
  res=$(sudo hyperctl exec $id ps aux | grep user > /dev/null 2>&1; echo $?)
  sudo hyperctl rm $id
  test $res -eq 0
}

hyper::test::imageusergroup() {
  echo "Pod image user group config test"
  # k8s-dns-sidecar-amd64:1.14.1 image has "User": "nobody" and "Group": "nobody"
  id=$(sudo hyperctl run -d gcr.io/google_containers/k8s-dns-sidecar-amd64:1.14.1 | sed -ne "s/POD id is \(.*\)/\1/p")
  res=$(sudo hyperctl exec $id ps aux | grep nobody > /dev/null 2>&1; echo $?)
  sudo hyperctl rm $id
  test $res -eq 0
}

hyper::test::specuseroverride() {
  echo "Pod spec user override test"
  # irssi image has "User": "user"
  # user-override.pod overrides it with "User": "nobody"
  id=$(sudo hyperctl run -p ${HYPER_ROOT}/hack/pods/user-override.pod | sed -ne "s/POD id is \(.*\)/\1/p")
  res=$(sudo hyperctl exec $id ps aux | grep nobody > /dev/null 2>&1; echo $?)
  sudo hyperctl rm $id
  test $res -eq 0
}

# regression test for #551 and #490
hyper::test::imagevolume() {
  echo "Pod image volume and /etc/hosts insertion test"
  id=$(sudo hyperctl run -d rethinkdb:2.3.5 | sed -ne "s/POD id is \(.*\)/\1/p")
  res1=$(sudo hyperctl exec $id mount | grep '/data' > /dev/null 2>&1; echo $?)
  res2=$(sudo hyperctl exec $id mount | grep '/etc/hosts' > /dev/null 2>&1; echo $?)
  sudo hyperctl rm $id
  test $res1 -eq 0 -a $res2 -eq 0
}

# regression test for #549
hyper::test::force_kill_container() {
  echo "Container force kill test"
  id=$(sudo hyperctl run -d -p ${HYPER_ROOT}/hack/pods/busybox-tty.pod | sed -ne "s/POD id is \(.*\)/\1/p")
  res=$(sudo hyperctl stop -c container-with-tty > /dev/null 2>&1; echo $?)
  sudo hyperctl rm $id
  test $res -eq 0
}

# regression test for #577
hyper::test::container_logs_no_newline() {
  echo "Container logs without newlines"
  id=$(sudo hyperctl run -d busybox echo -n foobar | sed -ne "s/POD id is \(.*\)/\1/p")
  sleep 3 # sleep a bit to let logger kick in
  res=$(sudo hyperctl logs $id)
  sudo hyperctl rm $id
  echo logs result $res
  test x$res = "xfoobar"
}

hyper::test::container_readonly_rootfs_and_volume() {
  echo "Container rootfs and volume readonly test"
  id=$(sudo hyperctl run -p ${HYPER_ROOT}/hack/pods/readonly-rootfs.pod | sed -ne 's/POD id is \(.*\)/\1/p')
  sudo hyperctl exec -t $id mount
  res1=$(sudo hyperctl exec $id touch /foobar || true)
  echo res1 test result is ${res1}
  res2=$(sudo hyperctl exec $id touch /tmp/foobar || true)
  echo res2 test result is ${res2}
  res1=$(echo res1 test result is ${res1} | grep 'Read-only file system' > /dev/null 2>&1; echo $?)
  res2=$(echo res2 test result is ${res2} | grep 'Read-only file system' > /dev/null 2>&1; echo $?)
  sudo hyperctl rm $id
  test $res1 -eq 0 -a $res2 -eq 0
}
