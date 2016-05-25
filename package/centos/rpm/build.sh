#!/bin/bash

rpmbuild -ba hyper-container.spec
rpmbuild -ba hyperstart.spec
rpmbuild -ba qemu-hyper.spec
ls -lh ../RPMS/x86_64
sync
