#!/bin/bash

sudo rm -fr /opt/antelope/impala/
if [ ! -d /opt/antelope/impala ]; then
	sudo mkdir -p /opt/antelope/impala
	sudo chown -R vagrant: /opt/antelope
	for i in bin sbin lib64 conf systemd www lib; do
		mkdir /opt/antelope/impala/$i
	done
fi

sudo cp -r impala/www/ /opt/antelope/impala/
sudo cp impala/toolchain/toolchain-packages-gcc7.5.0/gcc-7.5.0/lib64/libgcc_s.so.1 /opt/antelope/impala/lib64/
sudo cp impala/toolchain/toolchain-packages-gcc7.5.0/gcc-7.5.0/lib64/libstdc++.so.6.0.24 /opt/antelope/impala/lib64/
sudo cp impala/toolchain/toolchain-packages-gcc7.5.0/kudu-f486f0813a/debug/lib64/libkudu_client.so.0.1.0 /opt/antelope/impala/lib64/
sudo cp -P impala/be/build/debug/service/* /opt/antelope/impala/sbin/
sudo cp impala/fe/target/impala-frontend-4.0.0-SNAPSHOT.jar /opt/antelope/impala/lib
sudo cp impala/fe/target/dependency/*.jar /opt/antelope/impala/lib
sudo cp -r impala/shell/build/impala-shell-4.0.0-SNAPSHOT /opt/antelope/impala/bin/shell

cat << EOF > /opt/antelope/impala/systemd/impala.service
[Unit]
Description=Apache Impala Daemon
Documentation=http://impala.apache.org

[Service]
EnvironmentFile=/opt/antelope/impala/conf/impala.env
ExecStart=/opt/antelope/impala/sbin/impalad --flagfile=/opt/antelope/impala/conf/impala.gflagfile
TimeoutStopSec=5
Restart=on-failure
User=impala

[Install]
WantedBy=multi-user.target
EOF

cat << EOF > /opt/antelope/impala/systemd/impala-catalog.service
[Unit]
Description=Apache Impala Catalog Daemon
Documentation=http://impala.apache.org

[Service]
EnvironmentFile=/opt/antelope/impala/conf/impala.env
ExecStart=/opt/antelope/impala/sbin/catalogd --flagfile=/opt/antelope/impala/conf/catalog.gflagfile
TimeoutStopSec=5
Restart=on-failure
User=impala

[Install]
WantedBy=multi-user.target
EOF

cat << EOF > /opt/antelope/impala/systemd/impala-statestore.service
[Unit]
Description=Apache Impala StateStore Daemon
Documentation=http://impala.apache.org

[Service]
EnvironmentFile=/opt/antelope/impala/conf/impala.env
ExecStart=/opt/antelope/impala/sbin/statestored --flagfile=/opt/antelope/impala/conf/statestore.gflagfile
TimeoutStopSec=5
Restart=on-failure
User=impala

[Install]
WantedBy=multi-user.target
EOF

cat << EOF > /opt/antelope/impala/systemd/impala-admission.service
[Unit]
Description=Apache Impala Admission Control Daemon
Documentation=http://impala.apache.org

[Service]
EnvironmentFile=/opt/antelope/impala/conf/impala.env
ExecStart=/opt/antelope/impala/sbin/admissiond --flagfile=/opt/antelope/impala/conf/admission.gflagfile
TimeoutStopSec=5
Restart=on-failure
User=impala

[Install]
WantedBy=multi-user.target
EOF

cat << EOF > /opt/antelope/impala/conf/impala.env
IMPALA_HOME=/opt/antelope/impala
JAVA_HOME=/usr/lib/jvm/java/
CLASSPATH=/opt/antelope/impala/lib/*:/opt/antelope/hive/lib/*
HADOOP_HOME=/opt/antelope/hadoop
HIVE_HOME=/opt/antelope/hive
HIVE_CONF=/opt/antelope/hive/conf
EOF

cat << EOF > /opt/antelope/impala/conf/impala.gflagfile
--abort_on_config_error=false
--log_dir=/var/log/impala
--state_store_host=masters.antelope.lan
--catalog_service_host=masters.antelope.lan
--admission_service_host=masters.antelope.lan
--kudu_master_hosts=masters.antelope.lan
EOF

cat << EOF > /opt/antelope/impala/conf/catalog.gflagfile
--kudu_master_hosts=masters.antelope.lan
--log_dir=/var/log/impala
EOF

cat << EOF > /opt/antelope/impala/conf/statestore.gflagfile
--kudu_master_hosts=masters.antelope.lan
--log_dir=/var/log/impala
EOF

cat << EOF > /opt/antelope/impala/conf/admission.gflagfile
--kudu_master_hosts=masters.antelope.lan
--log_dir=/var/log/impala
EOF

# decidimos que el Hadoop y el Hive sean los descargados en el proyecto.
# Asi garantizamos la compatibilidad.

sudo cp -r impala/toolchain/cdp_components-14156131/apache-hive-3.1.3000.7.2.11.0-94-bin /opt/antelope/hive
sudo cp -r impala/toolchain/cdp_components-14156131/hadoop-3.1.1.7.2.11.0-94 /opt/antelope/hadoop
sudo mkdir /opt/antelope/hive/systemd

cat << EOF > /opt/antelope/systemd/metastore.service 
[Unit]
Description=Apache Hive Metastore - HSM
Documentation=http://hive.apache.org

[Service]
Type=simple
Environment=JAVA_HOME=/usr/lib/jvm/java/
Environment=HADOOP_HOME=/opt/antelope/hadoop
ExecStart=/opt/antelope/hive/bin/hive --service metastore --hiveconf hive.root.logger=DEBUG,console
onsole
TimeoutStopSec=5
Restart=on-failure
User=hive

[Install]
WantedBy=multi-user.target
EOF

cat << EOF /opt/antelope/hive/conf/hive-site.xml
<configuration>
  <property>
    <name>metastore.thrift.uris</name>
    <value>thrift://masters.antelope.lan:9083</value>
    <description>Thrift URI for the remote metastore. 
      Used by metastore client to connect to remote metastore.
    </description>
  </property>

  <property>
    <name>hive.metastore.warehouse.dir</name>
    <value>/var/lib/hive/warehouse</value>
    <description>location of default database for the warehouse</description>
  </property>

  <property>
    <name>metastore.task.threads.always</name>
    <value>org.apache.hadoop.hive.metastore.events.EventCleanerTask</value>
  </property>

  <property>
    <name>metastore.expression.proxy</name>
    <value>org.apache.hadoop.hive.metastore.DefaultPartitionExpressionProxy</value>
  </property>

  <property>
    <name>javax.jdo.option.ConnectionDriverName</name>
    <value>org.postgresql.Driver</value>
  </property>

  <property>
    <name>javax.jdo.option.ConnectionURL</name>
    <value>jdbc:postgresql://masters.antelope.lan:5432/metastore</value>
  </property>

  <property>
    <name>javax.jdo.option.ConnectionUserName</name>
    <value>postgres</value>
  </property>

  <property>
    <name>javax.jdo.option.ConnectionPassword</name>
    <value>postgres</value>
  </property> 

  <property>
    <name>hive.metastore.transactional.event.listeners</name>
    <description>
      KEY connection with KUDU from (artifact from Kudu release): hms-plugin.jar
    </description>
    <value>      
      org.apache.hive.hcatalog.listener.DbNotificationListener,
      org.apache.kudu.hive.metastore.KuduMetastorePlugin
    </value> 
  </property> 

  <property>
    <name>hive.metastore.disallow.incompatible.col.type.changes</name>
    <value>false</value>
  </property>

  <property>
    <name>hive.metastore.notifications.add.thrift.objects</name>
    <value>true</value>
  </property> 

  <property>
     <name>hive.metastore.event.db.notification.api.auth</name>
     <value>false</value>
     <description>
       Should metastore do authorization against database notification 
       related APIs such as get_next_notification.
       If set to true, then only the superusers in proxy settings have the permission
     </description>
  </property>
</configuration>
EOF

cat << EOF /opt/antelope/hadoop/etc/hadoop/core-site.xml
<configuration>
  <property>
    <name>dfs.namenode.name.dir</name>
    <value>/cluster/nn</value>
  </property>

  <property>
    <name>dfs.datanode.data.dir</name>
    <value>/cluster/1/dn,/cluster/2/dn</value>
  </property>

  <property>
    <name>dfs.replication</name>
    <value>3</value>
  </property>
</configuration>
EOF

cat << EOF /opt/antelope/hadoop/etc/hadoop/hdfs-site.xml
<configuration>
 <property>
    <name>fs.default.name</name>
    <value>file://var/lib/hive</value>
 </property> 
</configuration>
EOF

tar czf antelope-impala.tar.gz \
    -P /opt/antelope/impala \
    -P /opt/antelope/hadoop \
    -P /opt/antelope/hive
