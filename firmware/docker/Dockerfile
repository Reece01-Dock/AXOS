# Reproducible build environment for Asuswrt-Merlin (HND 5.04 / GT-AX6000).
# Upstream targets Ubuntu 20.04-era hosts for the prebuilt HND toolchains;
# do not bump the base image without re-verifying a full build.
FROM ubuntu:20.04

ENV DEBIAN_FRONTEND=noninteractive

# Package set per the Asuswrt-Merlin build documentation (superset is fine).
# libtool-bin: Debian/Ubuntu splits the actual /usr/bin/libtool executable
# script into this package — the plain "libtool" package alone only
# provides libtoolize and the aclocal macros. Without it, quagga's install
# step (which invokes `libtool --mode=install ...`) fails with "libtool:
# Command not found" (confirmed via a real build attempt reaching that far).
RUN dpkg --add-architecture i386 && apt-get update && apt-get install -y --no-install-recommends \
    autoconf automake autopoint bash bc bison bzip2 ca-certificates ccache cmake \
    cpio curl diffutils file flex g++ g++-multilib gawk gcc gcc-multilib gettext \
    git gperf groff-base intltool libattr1-dev libc6-dev:i386 libelf-dev \
    libglib2.0-dev libgtk2.0-dev libltdl-dev liblzo2-dev libncurses5 \
    libncurses5-dev libslang2 libssl-dev libtool libtool-bin libxml2-dev \
    libxml-parser-perl make patch perl pkg-config python2 python3 \
    python-is-python2 rsync ruby sed sudo texinfo unzip uuid-dev wget xxd \
    zip zlib1g-dev gengetopt \
    && rm -rf /var/lib/apt/lists/*

# The HND build expects toolchains under /opt/toolchains; build.sh bind-mounts
# the am-toolchains checkout to /opt/am-toolchains.
RUN ln -sfn /opt/am-toolchains/brcm-arm-hnd /opt/toolchains

# The prebuilt crosstools bundle their own libisl/libmpc/libmpfr/libgmp/etc
# (needed by cc1) under each toolchain's own lib/ dir, but their RPATH is
# baked in to the original build machine's path (e.g.
# /home/defjovi/temp3/toolchain/crosstools-.../lib), not wherever we mount
# them — confirmed directly via `readelf -d cc1`, which is why cc1 fails
# with "error while loading shared libraries: libisl.so.15: cannot open
# shared object file" even though the .so is right there. Registering every
# toolchain's lib dir with ldconfig fixes this for every bundled binary,
# not just cc1.
#
# This list is every crosstools-* directory in the pinned am-toolchains
# commit (firmware/.gitmodules), enumerated directly rather than globbed:
# /opt/toolchains doesn't exist yet at image-build time (build.sh
# bind-mounts am-toolchains there only at `docker run` time), so a glob
# here would silently match nothing. The two gcc-5.3 toolchains use
# usr/lib instead of a top-level lib/ — both are listed. A path that
# doesn't exist at ldconfig-run time (build.sh) is skipped harmlessly.
RUN mkdir -p /etc/ld.so.conf.d && printf '%s\n' \
    /opt/toolchains/crosstools-aarch64-gcc-10.3-linux-4.19-glibc-2.32-binutils-2.36.1/lib \
    /opt/toolchains/crosstools-aarch64-gcc-9.2-linux-4.19-glibc-2.30-binutils-2.32/lib \
    /opt/toolchains/crosstools-aarch64-gcc-5.5-linux-4.1-glibc-2.26-binutils-2.28.1/lib \
    /opt/toolchains/crosstools-aarch64-gcc-5.3-linux-4.1-glibc-2.22-binutils-2.25/usr/lib \
    /opt/toolchains/crosstools-arm-gcc-10.3-linux-4.19-glibc-2.32-binutils-2.36.1/lib \
    /opt/toolchains/crosstools-arm-gcc-9.2-linux-4.19-glibc-2.30-binutils-2.32/lib \
    /opt/toolchains/crosstools-arm-gcc-5.5-linux-4.1-glibc-2.26-binutils-2.28.1/lib \
    /opt/toolchains/crosstools-arm-gcc-5.3-linux-4.1-glibc-2.22-binutils-2.25/usr/lib \
    /opt/toolchains/crosstools-arm_softfp-gcc-10.3-linux-4.19-glibc-2.32-binutils-2.36.1/lib \
    > /etc/ld.so.conf.d/am-toolchains.conf

# Ubuntu's /bin/sh is dash by default, even with the bash package installed —
# installing bash doesn't make it the default shell. make.common's own
# prebuild_checks target explicitly requires /bin/sh to invoke bash (many of
# the HND SDK's makefiles/scripts rely on bash-specific behavior under
# "/bin/sh"), and fails the build with "ERROR: /bin/sh does not invoke bash
# shell" otherwise — confirmed against a real build attempt, not speculative.
RUN ln -sf /bin/bash /bin/sh

# Build as a normal user (parts of the build misbehave as root).
RUN useradd -m -s /bin/bash builder && echo 'builder ALL=(ALL) NOPASSWD:ALL' >> /etc/sudoers
USER builder
WORKDIR /build

# System tools MUST come before the toolchain dirs in PATH: the crosstools
# bundle their own pkg-config (and possibly automake/autoconf/libtoolize),
# scoped to their own sysroot — with the toolchain dirs first, prebuild_checks'
# `pkg-config --exists zlib` silently resolves to that shadowed pkg-config
# instead of the system one, and fails even though zlib1g-dev is installed
# (confirmed: both crosstools-*-gcc-5.5-.../{usr/,}bin/ contain their own
# pkg-config binary). This is safe: the actual cross-compiler is always
# invoked via TOOLCHAIN_TOP/CROSS_COMPILE's fully-qualified path
# ($(TOOLCHAIN_TOP)/bin/$(TOOLCHAIN_PREFIX)-gcc in make.common), never a
# bare `gcc`/etc. resolved from PATH, so PATH order doesn't affect which
# compiler actually gets used.
ENV LC_ALL=C \
    PATH=$PATH:/opt/toolchains/crosstools-aarch64-gcc-5.5-linux-4.1-glibc-2.26-binutils-2.28.1/usr/bin:/opt/toolchains/crosstools-arm-gcc-5.5-linux-4.1-glibc-2.26-binutils-2.28.1/usr/bin

CMD ["/bin/bash"]
