FROM centos
MAINTAINER Xu Wang <xu@hyper.sh>

# RPM Build Environment
RUN yum install -y @development-tools centos-packager rpmdevtools
RUN /usr/sbin/useradd makerpm
RUN usermod -a -G mock makerpm
RUN su makerpm -c 'rpmdev-setuptree'

# Hyper dependency
RUN yum install -y automake autoconf gcc make
RUN yum install -y device-mapper-devel pcre-devel libsepol-devel libselinux-devel systemd-container-devel
RUN yum install -y sqlite-devel
RUN curl -sL https://storage.googleapis.com/golang/go1.5.1.linux-amd64.tar.gz | tar -C /usr/local -zxf -

ENV PATH /usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# Build RPM
USER makerpm
WORKDIR /home/makerpm/rpmbuild/SPECS
