#!/usr/bin/env bash
DOCKER_CONTAINER_NAME=prometheus-mongodb-adapter
DOCKER_IMAGE_NAME=${DOCKER_CONTAINER_NAME}
SERVER_PORT=8000
MONGO_URL=mongodb://192.168.0.146:27017/prometheus
LISTEN_ADDRESS=0.0.0.0:${SERVER_PORT}


sudo docker stop ${DOCKER_CONTAINER_NAME}
sudo docker rm ${DOCKER_CONTAINER_NAME}
sudo docker run --name ${DOCKER_CONTAINER_NAME} -d -p ${SERVER_PORT}:${SERVER_PORT} -e "MONGO_URL=$MONGO_URL" -e "LISTEN_ADDRESS=$LISTEN_ADDRESS" --log-opt max-size=50m --log-opt max-file=5  ${DOCKER_IMAGE_NAME}
# sudo docker logs -f ${DOCKER_CONTAINER_NAME}
