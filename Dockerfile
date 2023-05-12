# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

FROM golang:1.19.9-alpine as go-discover
RUN CGO_ENABLED=0 go install github.com/hashicorp/go-discover/cmd/discover@49f60c093101c9c5f6b04d5b1c80164251a761a6

# ===================================
#
#   Non-release images.
#
# ===================================

# devdeps installs deps so we don't need to do it each time
FROM golang:latest as devdeps
ARG BIN_NAME
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

# devbuild compiles the binary
# -----------------------------------
FROM devdeps AS devbuild
ARG BIN_NAME
# Escape the GOPATH
WORKDIR /build
COPY . ./
ENV CGO_ENABLED 0

RUN go build -o $BIN_NAME


# dev runs the binary from devbuild
# -----------------------------------
FROM alpine:latest AS dev
ARG BIN_NAME
# Export BIN_NAME for the CMD below, it can't see ARGs directly.
ENV BIN_NAME=$BIN_NAME
COPY --from=devbuild /build/$BIN_NAME /bin/
COPY --from=go-discover /go/bin/discover /bin/

ENTRYPOINT /bin/$BIN_NAME
CMD ["version"]

# ===================================
#
#   Release images.
#
# ===================================


# default release image
# -----------------------------------
FROM alpine:latest AS default

ARG BIN_NAME
# Export BIN_NAME for the CMD below, it can't see ARGs directly.
ENV BIN_NAME=$BIN_NAME
ARG PRODUCT_VERSION
ARG PRODUCT_REVISION
ARG PRODUCT_NAME=$BIN_NAME
# TARGETARCH and TARGETOS are set automatically when --platform is provided.
ARG TARGETOS TARGETARCH

LABEL maintainer="Team RelEng <team-rel-eng@hashicorp.com>"
LABEL version=$PRODUCT_VERSION
LABEL revision=$PRODUCT_REVISION

# Create a non-root user to run the software.
RUN addgroup $PRODUCT_NAME && \
    adduser -S -G $PRODUCT_NAME 100

COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /bin/
COPY --from=go-discover /go/bin/discover /bin/

USER 100
ENTRYPOINT /bin/$BIN_NAME
CMD ["version"]


# debian release image (just for the sake of example)
# -----------------------------------
FROM debian:latest AS debian

ARG BIN_NAME
# Export BIN_NAME for the CMD below, it can't see ARGs directly.
ENV BIN_NAME=$BIN_NAME
ARG PRODUCT_VERSION
ARG PRODUCT_REVISION
ARG PRODUCT_NAME=$BIN_NAME
# TARGETARCH and TARGETOS are set automatically when --platform is provided.
ARG TARGETOS TARGETARCH

LABEL maintainer="Team RelEng <team-rel-eng@hashicorp.com>"
LABEL version=$PRODUCT_VERSION
LABEL revision=$PRODUCT_REVISION

# Create a non-root user to run the software.
RUN addgroup $PRODUCT_NAME && \
    adduser --system --uid 101 --group $PRODUCT_NAME

COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /bin/
COPY --from=go-discover /go/bin/discover /bin/

USER 101
ENTRYPOINT /bin/$BIN_NAME
CMD ["version"]

# ===================================
#
#   Set default target to 'dev'.
#
# ===================================
FROM dev
