#!/bin/bash

if [ ! -d /opt/antelope/kudu ]; then
    sudo mkdir -p /opt/antelope/kudu
    sudo chown -R vagrant: /opt/antelope
    for i in bin sbin lib64 conf systemd; do
        mkdir /opt/antelope/kudu/$i
    done
fi

cp /opt/kudu/usr/local/bin/* /opt/antelope/kudu/bin/
cp /opt/kudu/usr/local/sbin/* /opt/antelope/kudu/sbin/
cp /opt/kudu/usr/local/lib64/libkudu_client.so.0.1.0 /opt/antelope/kudu/lib64/
cp -r /home/vagrant/build/kudu/www /opt/antelope/kudu/

cat << EOF > /opt/antelope/kudu/systemd/kudu-master.service
[Unit]
Description=Apache Kudu Master Server
Documentation=http://kudu.apache.org

[Service]
Environment=KUDU_HOME=/usr/lib/kudu
ExecStart=/opt/antelope/kudu/sbin/kudu-master --flagfile=/opt/antelope/kudu/conf/master.gflagfile
TimeoutStopSec=5
Restart=on-failure
User=kudu

[Install]
WantedBy=multi-user.target
EOF

cat << EOF > /opt/antelope/kudu/systemd/kudu-tserver.service
[Unit]
Description=Apache Kudu Tablet Server
Documentation=http://kudu.apache.org

[Service]
Environment=KUDU_HOME=/usr/lib/kudu
ExecStart=/opt/antelope/kudu/sbin/kudu-tserver --flagfile=/opt/antelope/kudu/conf/tserver.gflagfile
TimeoutStopSec=5
Restart=on-failure
User=kudu

[Install]
WantedBy=multi-user.target
EOF

cat << EOF > /opt/antelope/kudu/conf/master.gflagfile
## Comma-separated list of the RPC addresses belonging to all Masters in this cluster.
## NOTE: if not specified, configures a non-replicated Master.
#--master_addresses=

--webserver_doc_root=/opt/antelope/kudu/www
--log_dir=/var/log/kudu
--fs_wal_dir=/var/lib/kudu/master/wal
--fs_data_dirs=/var/lib/kudu/master/data
--stderrthreshold=0
--rpc_encryption=disabled
--rpc_authentication=disabled
--rpc_negotiation_timeout_ms=5000

## You can avoid the dependency on ntpd by running Kudu with
## This is not recommended for production environment.
## NOTE: If you run without hybrid time the tablet history GC will not work. 
## Therefore when you delete or update a row the history of that data will be kept 
## forever. Eventually you may run out of disk space. 
--use_hybrid_clock=false
EOF

cat << EOF > /opt/antelope/kudu/conf/tserver.gflagfile
##Comma separated addresses of the masters which the tablet server should connect to.
--tserver_master_addrs=masters.antelope.lan:7051

--webserver_doc_root=/opt/antelope/kudu/www
--log_dir=/var/log/kudu
--fs_wal_dir=/var/lib/kudu/tserver/wal
--fs_data_dirs=/var/lib/kudu/tserver/data
--stderrthreshold=0
--rpc_encryption=disabled
--rpc_authentication=disabled
--rpc_negotiation_timeout_ms=5000

## You can avoid the dependency on ntpd by running Kudu with
## This is not recommended for production environment.
## NOTE: If you run without hybrid time the tablet history GC will not work. 
## Therefore when you delete or update a row the history of that data will be kept 
## forever. Eventually you may run out of disk space. 
--use_hybrid_clock=false
EOF

tar cvzf antelope-kudu.tar.gz -P /opt/antelope/kudu
