FROM centos:centos7

COPY ./kudu-1.17.0-SNAPSHOT.tar.gz /
RUN yum -y install sudo

RUN sudo yum install -y cyrus-sasl-gssapi cyrus-sasl-plain cyrus-sasl-devel krb5-server krb5-workstation openssl lzo-devel tzdata
RUN yum -y install httpd; yum clean all; systemctl enable httpd.service
RUN sudo mkdir -p /var/lib/kudu && \
  sudo mkdir -p /var/log/kudu

RUN sudo adduser -r kudu && \
  sudo chown -R kudu: /var/lib/kudu && \
  sudo chown -R kudu: /var/log/kudu

RUN sudo tar xvzf kudu-1.17.0-SNAPSHOT.tar.gz -C / && \
  sudo mkdir /opt/kudu/conf && \
  sudo chown -R kudu: /opt/kudu/ && \
  rm /kudu-1.17.0-SNAPSHOT.tar.gz

RUN yum install -y wget && \
  wget https://github.com/kubernetesbigdataeg/propgen/releases/latest/download/propgen

COPY docker-entrypoint.sh /
RUN chmod 777 /docker-entrypoint.sh && chmod 777 propgen

ENTRYPOINT ["/docker-entrypoint.sh"]
EXPOSE 8050 8051 7050 7051
