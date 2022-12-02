podman build . --tag docker.io/kubernetesbigdataeg/kudu:1.17.0-2
podman login docker.io -u kubernetesbigdataeg
podman push docker.io/kubernetesbigdataeg/kudu:1.17.0-2
