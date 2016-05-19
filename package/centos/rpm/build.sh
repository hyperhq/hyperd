#!/bin/bash

rpmbuild -ba hyper.spec
rpmbuild -ba hyperstart.spec
rpmbuild -ba qemu-hyper.spec
ls -lh ../RPMS/x86_64
sleep 60
sync
