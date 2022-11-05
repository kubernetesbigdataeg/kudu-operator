#!/bin/bash

sudo yum -y install autoconf automake curl cyrus-sasl-devel cyrus-sasl-gssapi \
  cyrus-sasl-plain flex gcc gcc-c++ gdb git java-1.8.0-openjdk-devel \
  krb5-server krb5-workstation libtool make openssl-devel patch pkgconfig \
  redhat-lsb-core rsync unzip vim-common which vim tree

sudo yum -y install centos-release-scl-rh
sudo yum -y install devtoolset-8

git clone https://github.com/apache/kudu

cd kudu

build-support/enable_devtoolset.sh thirdparty/build-if-necessary.sh
mkdir -p build/release
cd build/release

../../build-support/enable_devtoolset.sh \
  ../../thirdparty/installed/common/bin/cmake \
  -DCMAKE_BUILD_TYPE=release \
  ../..
make -j1

sudo mkdir /opt/kudu && sudo chown -R vagrant: /opt/kudu

make DESTDIR=/opt/kudu install


