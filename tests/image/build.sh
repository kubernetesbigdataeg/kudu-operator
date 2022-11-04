podman build . --tag docker.io/kubernetesbigdataeg/kudu:1.17.0-1
podman login docker.io
podman push docker.io/kubernetesbigdataeg/kudu:1.17.0-1