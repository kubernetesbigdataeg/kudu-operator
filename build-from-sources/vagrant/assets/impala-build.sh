#!/bin/bash

git clone https://github.com/apache/impala.git
pushd /home/vagrant/build/impala/
export IMPALA_HOME=`pwd`
./bin/bootstrap_system.sh

source ./bin/impala-config.sh
./buildall.sh -noclean -notests
