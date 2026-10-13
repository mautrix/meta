FROM rust:1.98.1-alpine3.24 AS opaque

WORKDIR /build
COPY pkg/messagix/securehistory/opaque /build
RUN cargo build --locked --release

FROM golang:1-alpine3.24 AS builder

RUN apk add --no-cache git ca-certificates build-base su-exec olm-dev

COPY --from=opaque /build/target/release/libvesta_opaque.a /usr/local/lib/libvesta_opaque.a
ENV CGO_LDFLAGS=-L/usr/local/lib

COPY . /build
WORKDIR /build
RUN ./build-ig.sh

FROM alpine:3.24

ENV UID=1337 \
    GID=1337

RUN apk add --no-cache ffmpeg su-exec ca-certificates olm bash jq yq-go curl

COPY --from=builder /build/mautrix-instagram /usr/bin/mautrix-instagram
COPY --from=builder /build/docker-run.sh /docker-run.sh
VOLUME /data

CMD ["/docker-run.sh"]
