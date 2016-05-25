Summary:            Hyper build Qemu with virtfs support
Name:               qemu-hyper
Version:            2.4.1
Release:            2%{?dist}
License:            Apache License, Version 2.0
Group:              System Environment/Base
Source0:            http://wiki.qemu-project.org/download/qemu-2.4.1.tar.bz2
URL:                https://qemu-project.org
ExclusiveArch:      x86_64
Requires:           librbd1
BuildRequires:      libcap-devel,libattr-devel,librbd1-devel

%define _unpackaged_files_terminate_build 0
%define _missing_doc_files_terminate_build 0

%description
Qemu is the powerful and popular Hardware emulator
Hyper build is for x86_64 arch and enable virtfs and rbd support

%prep
%setup -n qemu-2.4.1

%build
cd %{_builddir}/qemu-2.4.1
./configure --prefix=/usr --enable-virtfs --enable-rbd --disable-fdt --disable-vnc  --disable-guest-agent --audio-drv-list="" --disable-tpm --target-list=x86_64-softmmu
make

%install
%make_install

%clean
#rm -rf %{buildroot}

%files
%{_bindir}/qemu-system-x86_64
%{_datadir}/qemu/bios-256k.bin
%{_datadir}/qemu/efi-virtio.rom
%{_datadir}/qemu/kvmvapic.bin
%{_datadir}/qemu/linuxboot.bin

%changelog
* Fri Jan 29 2016 Hyper Dev Team <dev@hyper.sh> - 2.4.1-2
- Include virtio firmware
- Include librbd dependency
* Wed Dec 2 2015 Xu Wang <xu@hyper.sh> - 2.4.1-1
- config with virtfs and rbd
