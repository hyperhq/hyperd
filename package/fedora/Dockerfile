FROM fedora
MAINTAINER Hyper Developers <dev@hyper.sh>

# RPM Build Environment
RUN dnf install -y patch git fedora-packager rpmdevtools
RUN /usr/sbin/useradd makerpm; usermod -a -G mock makerpm; su makerpm -c 'rpmdev-setuptree'

# Hyper dependency
RUN dnf install -y automake autoconf gcc make glibc-devel glibc-devel.i686 device-mapper-devel pcre-devel libsepol-devel libselinux-devel systemd-devel sqlite-devel libvirt-devel xen-devel
RUN curl -sL https://storage.googleapis.com/golang/go1.5.1.linux-amd64.tar.gz | tar -C /usr/local -zxf -

ENV PATH /usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# Build RPM
USER makerpm
WORKDIR /home/makerpm/rpmbuild/SPECS
