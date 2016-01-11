#!/bin/sh

srcdir=`dirname $0`
test -z "$srcdir" && srcidr=.

cd $srcdir

die()
{
	echo
	echo "$1"
	echo
	exit 1
}

test -f hyperd.go || die "You must run this script in the top-level hyper directory."

aclocal --version < /dev/null > /dev/null 2>&1 || die "You must have aclocal installed to generate the hyper."
autoconf --version < /dev/null > /dev/null 2>&1 || die "You must have autoconf installed to generate the hyper."
automake --version < /dev/null > /dev/null 2>&1 || die "You must have automake installed to generate the hyper."
pkg-config --version < /dev/null > /dev/null 2>&1 || die "You must have pkg-config installed to generate the hyper."

echo
echo "Generating build-system with:"
echo "  aclocal:  $(aclocal --version | head -1)"
echo "  autoconf:  $(autoconf --version | head -1)"
echo "  automake:  $(automake --version | head -1)"
echo "  pkg-config:  $(pkg-config --version | head -1)"
echo

rm -rf autom4te.cache

aclocal
autoconf
automake --add-missing

echo
echo "type '$srcdir/configure' and 'make' to compile hyper."
echo
