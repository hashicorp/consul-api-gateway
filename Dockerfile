FROM alpine:3.13

RUN set -eux && \
    apk add --no-cache netcat-openbsd ca-certificates curl gnupg libcap openssl su-exec iputils libc6-compat iptables

COPY ./polar /bin/polar
ENTRYPOINT ["/bin/polar"]
CMD ["version"]