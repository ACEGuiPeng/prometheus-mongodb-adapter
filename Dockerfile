FROM ubuntu:16.04
COPY prometheus-mongodb-adapter /bin/prometheus-mongodb-adapter
RUN chmod +x /bin/prometheus-mongodb-adapter
RUN ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && echo 'Asia/Shanghai' >/etc/timezone
ENTRYPOINT [ "/bin/prometheus-mongodb-adapter" ]
