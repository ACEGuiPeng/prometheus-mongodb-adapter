#!/usr/bin/env bash
# setting
DOCKER_CONTAINER_NAME=prometheus-mongodb-adapter
DOCKER_IMAGE_NAME=${DOCKER_CONTAINER_NAME}

echo "begin to build: $DOCKER_IMAGE_NAME"
sudo docker build -t ${DOCKER_IMAGE_NAME} ./
#　清除none镜像
sudo docker ps -a | grep "Exited" | awk '{print $1 }'|xargs sudo docker stop
sudo docker ps -a | grep "Exited" | awk '{print $1 }'|xargs sudo docker rm
sudo docker images|grep none|awk '{print $3 }'|xargs sudo docker rmi