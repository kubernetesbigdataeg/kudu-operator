#!/bin/bash

git clone https://github.com/apache/hive.git
pushd /home/vagrant/build/hive
mvn clean package -DskipTests -Pdist

