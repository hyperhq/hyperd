Summary:            Hyper build Qemu with template and virtfs support
Name:               qemu-hyper
Version:            2.4.1
Release:            3%{?dist}
License:            Apache License, Version 2.0
Group:              System Environment/Base
Source0:            http://wiki.qemu-project.org/download/qemu-2.4.1.tar.bz2
URL:                https://qemu-project.org
ExclusiveArch:      x86_64
Requires:           librbd1
BuildRequires:      libcap-devel,libattr-devel,librbd1-devel

# enable full pathname hostmem backend file (for access saved memory)
Patch1: 0001-backends-hostmem-file-Allow-to-specify-full-pathname.patch
# just remove silly warning when we enable full pathname hostmem backend file
Patch2: 0002-exec-remove-warning-about-mempath-and-hugetlbfs.patch
# support 9p for migration
Patch3: 0003-virtio-9p-add-savem-handlers.patch
# don't need currently, we may need it for future 9p hotplug migration
Patch4: 0004-virtio-9p-device-add-minimal-unrealize-handler.patch
# vm-template for hyper (qemu/libvirt dirver)
Patch5: 0005-migration-add-migration-capability-to-bypass-the-sha.patch

# backport fix for cve-2016-9602
Patch6: 0006-9pfs-local-move-xattr-security-ops-to-9p-xattr.c.patch
Patch7: 0007-9pfs-remove-side-effects-in-local_init.patch
Patch8: 0008-9pfs-remove-side-effects-in-local_open-and-local_ope.patch
Patch9: 0009-9pfs-introduce-relative_openat_nofollow-helper.patch
Patch10: 0010-9pfs-local-keep-a-file-descriptor-on-the-shared-fold.patch
Patch11: 0011-9pfs-add-cleanup-operation-in-FileOperations.patch
Patch12: 0012-9pfs-local-open-opendir-don-t-follow-symlinks.patch
Patch13: 0013-9pfs-local-lgetxattr-don-t-follow-symlinks.patch
Patch14: 0014-9pfs-local-llistxattr-don-t-follow-symlinks.patch
Patch15: 0015-9pfs-local-lsetxattr-don-t-follow-symlinks.patch
Patch16: 0016-9pfs-local-lremovexattr-don-t-follow-symlinks.patch
Patch17: 0017-9pfs-local-unlinkat-don-t-follow-symlinks.patch
Patch18: 0018-9pfs-local-remove-don-t-follow-symlinks.patch
Patch19: 0019-9pfs-local-utimensat-don-t-follow-symlinks.patch
Patch20: 0020-9pfs-local-statfs-don-t-follow-symlinks.patch
Patch21: 0021-9pfs-local-truncate-don-t-follow-symlinks.patch
Patch22: 0022-9pfs-local-readlink-don-t-follow-symlinks.patch
Patch23: 0023-9pfs-local-lstat-don-t-follow-symlinks.patch
Patch24: 0024-9pfs-local-renameat-don-t-follow-symlinks.patch
Patch25: 0025-9pfs-local-rename-use-renameat.patch
Patch26: 0026-9pfs-local-improve-error-handling-in-link-op.patch
Patch27: 0027-9pfs-local-link-don-t-follow-symlinks.patch
Patch28: 0028-9pfs-local-chmod-don-t-follow-symlinks.patch
Patch29: 0029-9pfs-local-chown-don-t-follow-symlinks.patch
Patch30: 0030-9pfs-local-symlink-don-t-follow-symlinks.patch
Patch31: 0031-9pfs-local-mknod-don-t-follow-symlinks.patch
Patch32: 0032-9pfs-local-mkdir-don-t-follow-symlinks.patch
Patch33: 0033-9pfs-local-open2-don-t-follow-symlinks.patch
Patch34: 0034-9pfs-local-drop-unused-code.patch

%define _unpackaged_files_terminate_build 0
%define _missing_doc_files_terminate_build 0

%description
Qemu is the powerful and popular Hardware emulator
Hyper build is for x86_64 arch and enable virtfs and rbd support

%prep
%setup -n qemu-2.4.1

%patch1 -p1
%patch2 -p1
%patch3 -p1
%patch4 -p1
%patch5 -p1
%patch6 -p1
%patch7 -p1
%patch8 -p1
%patch9 -p1
%patch10 -p1
%patch11 -p1
%patch12 -p1
%patch13 -p1
%patch14 -p1
%patch15 -p1
%patch16 -p1
%patch17 -p1
%patch18 -p1
%patch19 -p1
%patch20 -p1
%patch21 -p1
%patch22 -p1
%patch23 -p1
%patch24 -p1
%patch25 -p1
%patch26 -p1
%patch27 -p1
%patch28 -p1
%patch29 -p1
%patch30 -p1
%patch31 -p1
%patch32 -p1
%patch33 -p1
%patch34 -p1

%build
cd %{_builddir}/qemu-2.4.1
./configure --prefix=/usr --enable-virtfs --enable-rbd --disable-fdt --disable-vnc  --disable-guest-agent --disable-gtk --disable-sdl --audio-drv-list="" --disable-tpm --target-list=x86_64-softmmu
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
* Fri Mar 3 2017 Hyper Dev Team <dev@hyper.sh> - 2.4.1-3
- template
- backport fix for cve-2016-9602
* Fri Jan 29 2016 Hyper Dev Team <dev@hyper.sh> - 2.4.1-2
- Include virtio firmware
- Include librbd dependency
* Wed Dec 2 2015 Xu Wang <xu@hyper.sh> - 2.4.1-1
- config with virtfs and rbd
