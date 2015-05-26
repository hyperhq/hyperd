#!/bin/sh

gopath=$GOPATH

if [ ! -e $gopath ]
then
  echo "Please make sure you have GOPATH in environment!\n"
  exit 1
fi

git clone https://github.com/gorilla/context $gopath/src/github.com/gorilla/context
git clone https://github.com/gorilla/mux $gopath/src/github.com/gorilla/mux
git clone https://github.com/syndtr/goleveldb $gopath/src/github.com/syndtr/goleveldb
git clone https://github.com/syndtr/gosnappy $gopath/src/github.com/syndtr/gosnappy
git clone https://github.com/jessevdk/go-flags $gopath/src/github.com/jessevdk/go-flags
