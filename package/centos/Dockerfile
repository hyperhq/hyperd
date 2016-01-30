FROM centos
MAINTAINER Xu Wang <xu@hyper.sh>

# RPM Build Environment
RUN yum install -y @development-tools centos-packager rpmdevtools
RUN /usr/sbin/useradd makerpm; usermod -a -G mock makerpm; su makerpm -c 'rpmdev-setuptree'

# Hyper dependency
RUN yum install -y automake autoconf gcc make glibc-devel glibc-devel.i686 device-mapper-devel pcre-devel libsepol-devel libselinux-devel systemd-devel sqlite-devel libvirt-devel
RUN curl -sL https://storage.googleapis.com/golang/go1.5.1.linux-amd64.tar.gz | tar -C /usr/local -zxf -

# CBFSTool
RUN cd /dev/shm; curl -sSL https://www.coreboot.org/releases/coreboot-4.2.tar.xz | tar --xz -xvf - ; cd coreboot-4.2/util/cbfstool ; make; cp cbfstool /usr/local/bin/

# Qemu Denpendency
RUN yum install -y gcc-c++ zlib-devel libcap-devel libattr-devel librbd1-devel libtool

ENV PATH /usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# Build RPM
USER makerpm
WORKDIR /home/makerpm/rpmbuild/SPECS
