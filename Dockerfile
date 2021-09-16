FROM alpine:3.13

RUN set -eux && \
    apk add --no-cache netcat-openbsd ca-certificates curl gnupg libcap openssl su-exec iputils libc6-compat iptables

COPY ./consul-api-gateway /bin/consul-api-gateway
ENTRYPOINT ["/bin/consul-api-gateway"]
CMD ["version"]
