#!/bin/bash

su makerpm -c "rpmbuild -ba hyper.spec"
su makerpm -c "rpmbuild -ba hyperstart.spec"
ls -lh ../RPMS/x86_64
sync
