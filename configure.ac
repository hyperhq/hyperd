#                                               -*- Autoconf -*-
# Process this file with autoconf to produce a configure script.

AC_PREREQ([2.69])
AC_INIT([hyper], [0.2], [hyper.sh])
AM_INIT_AUTOMAKE([-Wall -Werror foreign subdir-objects])
AC_CONFIG_SRCDIR([hypervisor/xen/hyperxl.c])
AC_CONFIG_HEADERS([hypervisor/xen/config.h])

# Checks for programs.
AC_PROG_CC
AM_PROG_AR
AC_PROG_RANLIB

# Checks for go tool chain
AC_CHECK_PROG([has_go], [go], [yes], [no])
if test "x$has_go" != "xyes" ; then
    AC_MSG_ERROR(Unable to find go binary in PATH)
fi

AC_CHECK_PROG([has_godep], [godep], [yes], [no])
if test "x$has_godep" != "xyes" ; then
    AC_MSG_ERROR(Unable to find godep binary in PATH)
fi

# Checks for libraries.

# Checks for header files.
AC_CHECK_HEADERS([stdlib.h string.h libxl.h])

# Checks for typedefs, structures, and compiler characteristics.
AC_CHECK_HEADER_STDBOOL
AC_TYPE_PID_T
AC_TYPE_UINT32_T
AC_TYPE_UINT8_T

# Checks for library functions.
AC_FUNC_MALLOC
AC_CHECK_FUNCS([strdup])

AC_ARG_WITH([xen],
            [AS_HELP_STRING([--without-xen],
                            [run hyperd with xen (libxl, need xen 4.5 or higher)])],
            [with_xen=no],[with_xen=yes])

if test "x$with_xen" != "xno" ; then
    # Checks for libxl
    AC_CHECK_HEADERS([libxl.h libxl_utils.h], [libxl_found=yes], [libxl_found=no])
    if test "x$libxl_found" != "xyes"; then
           with_xen=no
    fi
fi

if test "x$with_xen" != "xno" ; then
    AC_DEFINE_UNQUOTED([WITH_XEN], 1, [run hyperd with xen])
fi

AM_CONDITIONAL([WITH_XEN], [test "x$with_xen" == "xyes"])

AC_CONFIG_FILES([Makefile
                 hypervisor/Makefile
                 hypervisor/xen/Makefile])
AC_OUTPUT

AC_MSG_RESULT([
	${PACKAGE}  ${VERSION}
	prefix:	    ${prefix}

	has go:     ${has_go}
	has godep:  ${has_go}

	with xen:   ${with_xen}
])
