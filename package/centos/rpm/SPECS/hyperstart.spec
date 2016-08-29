Summary:            Hyperstart is the initrd for hyper VM
Name:               hyperstart
Version:            0.6.2
Release:            1%{?dist}
License:            Apache License, Version 2.0
Group:              System Environment/Base
# The source for this package was pulled from upstream's git repo. Use the
# following commands to generate the tarball:
#  git archive --format=tar.gz master > hyperstart-%{version}.tar.gz
Source0:            %{name}-%{version}.tar.gz
URL:                https://hyper.sh/
ExclusiveArch:      x86_64

%description
Hyperstart is the initrd for hyper VM, hyperstart 
includes the kernel and initrd, qboot bios and cbfs rom
image.

%prep
mkdir -p %{_builddir}/src/github.com/hyperhq/hyperstart
tar -C %{_builddir}/src/github.com/hyperhq/hyperstart -xvf %SOURCE0

%build
cd %{_builddir}/src/github.com/hyperhq/hyperstart
./autogen.sh
./configure
make %{?_smp_mflags}

%install
mkdir -p %{buildroot}%{_sharedstatedir}/hyper
cp %{_builddir}/src/github.com/hyperhq/hyperstart/build/kernel %{buildroot}%{_sharedstatedir}/hyper/
cp %{_builddir}/src/github.com/hyperhq/hyperstart/build/hyper-initrd.img %{buildroot}%{_sharedstatedir}/hyper/

%clean
rm -rf %{buildroot}

%files
%{_sharedstatedir}/*

%changelog
* Mon Aug 29 2016 Hyper Dev Team <dev@hyper.sh> - 0.6.2-1
- update source to 0.6.2
* Thu Apr 28 2016 Hyper Dev Team <dev@hyper.sh> - 0.6-1
- update source to 0.6
- kernel update to 4.4.7 with modules provided
- volume population support
- tty processing improvement
- many other fix and improvement
* Sat Jan 30 2016 Xu Wang <xu@hyper.sh> - 0.5-1
- update source to 0.5
* Fri Jan 29 2016 Xu Wang <xu@hyper.sh> - 0.4-2
- Fix firmware path
* Sat Nov 21 2015 Xu Wang <xu@hyper.sh> - 0.4-1
- Initial rpm packaging for hyperstart
