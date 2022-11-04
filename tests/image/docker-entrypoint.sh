#!/bin/bash
if [[ "$1" = 'master' ]]; then

  source /etc/environments/kudu.env
  ./propgen -label KUDU -render kudumaster -file /opt/kudu/conf/master.gflagfile
  sudo chown kudu: /opt/kudu/conf/master.gflagfile
  export KUDU_HOME=/var/lib/kudu
  exec /opt/kudu/usr/local/sbin/kudu-master --flagfile=/opt/kudu/conf/master.gflagfile

elif [[ "$1" = 'tserver' ]]; then

  source /etc/environments/kudu.env
  ./propgen -label KUDU -render kuduworker -file /opt/kudu/conf/tserver.gflagfile
  sudo chown kudu: /opt/kudu/conf/tserver.gflagfile
  export KUDU_HOME="/var/lib/kudu"
  exec /opt/kudu/usr/local/sbin/kudu-tserver --flagfile=/opt/kudu/conf/tserver.gflagfile
    
fi