#!/bin/bash

export KUDU_HOME=/var/lib/kudu

function create_data_folders() {
  [ ! -d ${KUDU_HOME}/wal ] && mkdir ${KUDU_HOME}/wal
  [ ! -d ${KUDU_HOME}/data ] && mkdir ${KUDU_HOME}/data
}

if [[ "$1" = 'master' ]]; then

  source /etc/environments/kudu.env
  ./propgen -label KUDU -render kudumaster -file /opt/kudu/conf/master.gflagfile
  sudo chown kudu: /opt/kudu/conf/master.gflagfile
  create_data_folders
  exec /opt/kudu/usr/local/sbin/kudu-master --flagfile=/opt/kudu/conf/master.gflagfile

elif [[ "$1" = 'tserver' ]]; then

  source /etc/environments/kudu.env
  ./propgen -label KUDU -render kudutserver -file /opt/kudu/conf/tserver.gflagfile
  sudo chown kudu: /opt/kudu/conf/tserver.gflagfile
  create_data_folders
  exec /opt/kudu/usr/local/sbin/kudu-tserver --flagfile=/opt/kudu/conf/tserver.gflagfile
    
fi
