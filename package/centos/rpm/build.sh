#!/bin/bash

su makerpm -c "rpmbuild -ba hyper.spec"
su makerpm -c "rpmbuild -ba hyperstart.spec"
