#!/bin/sh

srcdir=`dirname $0`
test -z "$srcdir" && srcidr=.

cd $srcdir
DIE=0

test -f hypervisor/xen/hyperxl.c || {
	echo
	echo "You must run this script in the top-level hyper drectory."
	echo
	DIE=1
}

(autoconf --version) < /dev/null > /dev/null 2>&1 || {
	echo
	echo "You must have autoconf installed to generate the hyper."
	echo
	DIE=1
}

(autoheader --version) < /dev/null > /dev/null 2>&1 || {
	echo
	echo "You must have autoheader installed to generate the hyper."
	echo
	DIE=1
}

(automake --version) < /dev/null > /dev/null 2>&1 || {
	echo
	echo "You must have automake installed to generate the hyper."
	echo
	DIE=1
}
(autoreconf --version) < /dev/null > /dev/null 2>&1 || {
	echo
	echo "You must have autoreconf installed to generate the hyper."
	echo
	DIE=1
}

if test "$DIE" -eq 1; then
	exit 1
fi

echo
echo "Generating build-system with:"
echo "  aclocal:  $(aclocal --version | head -1)"
echo "  autoconf:  $(autoconf --version | head -1)"
echo "  autoheader:  $(autoheader --version | head -1)"
echo "  automake:  $(automake --version | head -1)"
echo

rm -rf autom4te.cache

aclocal
autoconf
autoheader
automake --add-missing

echo
echo "type '$srcdir/configure' and 'make' to compile hyper."
echo
