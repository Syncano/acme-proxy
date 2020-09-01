FROM alpine:3.11
ARG UID=1000
ARG GID=1000

RUN set -ex \
    && addgroup -S -g $GID acme-proxy \
    && adduser -S -D -G acme-proxy -u $UID acme-proxy
COPY acme_proxy /usr/local/bin/acme_proxy

WORKDIR /home/acme-proxy
COPY migrations /home/acme-proxy/migrations
USER acme-proxy
