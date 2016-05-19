FROM centos
MAINTAINER Hyper Developers <dev@hyper.sh>

# RPM Build Environment
RUN yum install -y @development-tools centos-packager rpmdevtools || echo "WARNING: work around the yum failure"
RUN /usr/sbin/useradd makerpm; usermod -a -G mock makerpm; su makerpm -c 'rpmdev-setuptree'

# Hyper dependency
RUN yum install -y automake autoconf gcc make glibc-devel glibc-devel.i686 device-mapper-devel pcre-devel libsepol-devel libselinux-devel systemd-devel sqlite-devel libvirt-devel|| echo "WARNING: work around the yum failure"
RUN curl -sL https://storage.googleapis.com/golang/go1.5.1.linux-amd64.tar.gz | tar -C /usr/local -zxf -

# Qemu Denpendency
RUN yum install -y gcc-c++ zlib-devel libcap-devel libattr-devel librbd1-devel libtool || echo "WARNING: work around the yum failure"

ENV PATH /usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# Build RPM
USER makerpm
WORKDIR /home/makerpm/rpmbuild/SPECS
