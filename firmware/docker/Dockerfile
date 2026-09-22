# Reproducible build environment for Asuswrt-Merlin (HND 5.04 / GT-AX6000).
# Upstream targets Ubuntu 20.04-era hosts for the prebuilt HND toolchains;
# do not bump the base image without re-verifying a full build.
FROM ubuntu:20.04

ENV DEBIAN_FRONTEND=noninteractive

# Package set per the Asuswrt-Merlin build documentation (superset is fine).
RUN dpkg --add-architecture i386 && apt-get update && apt-get install -y --no-install-recommends \
    autoconf automake autopoint bash bison bzip2 ca-certificates cmake cpio \
    curl diffutils file flex g++ g++-multilib gawk gcc gcc-multilib gettext \
    git gperf groff-base intltool libattr1-dev libc6-dev:i386 libelf-dev \
    libglib2.0-dev libgtk2.0-dev libltdl-dev liblzo2-dev libncurses5 \
    libncurses5-dev libslang2 libssl-dev libtool libxml2-dev \
    libxml-parser-perl make patch perl pkg-config python2 python3 \
    python-is-python2 rsync ruby sed sudo texinfo unzip uuid-dev wget xxd \
    zip zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

# The HND build expects toolchains under /opt/toolchains; build.sh bind-mounts
# the am-toolchains checkout to /opt/am-toolchains.
RUN ln -sfn /opt/am-toolchains/brcm-arm-hnd /opt/toolchains

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

ENV LC_ALL=C \
    PATH=/opt/toolchains/crosstools-aarch64-gcc-5.5-linux-4.1-glibc-2.26-binutils-2.28.1/usr/bin:/opt/toolchains/crosstools-arm-gcc-5.5-linux-4.1-glibc-2.26-binutils-2.28.1/usr/bin:$PATH

CMD ["/bin/bash"]
