#!/bin/bash

su makerpm -c "rpmbuild -ba hyper-container.spec"
su makerpm -c "rpmbuild -ba hyperstart.spec"
su makerpm -c "rpmbuild -ba qemu-hyper.spec"
ls -lh ../RPMS/x86_64
sync
