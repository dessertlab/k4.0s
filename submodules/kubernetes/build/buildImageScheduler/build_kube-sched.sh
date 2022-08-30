#!/bin/bash
cd ../../
make kube-scheduler
rm ./build/buildImageScheduler/kube-scheduler
cp ./_output/local/bin/linux/amd64/kube-scheduler ./build/buildImageScheduler/
cd ./build/buildImageScheduler
export LOCAL_REGISTRY=localhost:5000
export LOCAL_IMAGE=kube-scheduler:latest
export RELEASE_VERSION=v$(date +%Y%m%d)-2
docker build -f ./Dockerfile --build-arg ARCH="amd64" --build-arg RELEASE_VERSION="$RELEASE_VERSION" -t $LOCAL_REGISTRY/$LOCAL_IMAGE .
docker push $LOCAL_REGISTRY/$LOCAL_IMAGE
#docker build -f ./build/controller/Dockerfile --build-arg ARCH="amd64" -t $(LOCAL_REGISTRY)/$(LOCAL_CONTROLLER_IMAGE) .
