#!/bin/bash

su makerpm -c "rpmbuild -ba hyper.spec"
su makerpm -c "rpmbuild -ba hyperstart.spec"
su makerpm -c "rpmbuild -ba qemu-hyper.spec"
ls -lh ../RPMS/x86_64
sleep 60
sync
