FROM golang:1.16-buster AS build-plugin
ENV GOPROXY=https://proxy.golang.org
ENV PROJECTPATH=/go/src/github.com/replicatedhq/local-volume-provider/local-volume-fileserver
WORKDIR $PROJECTPATH
COPY Makefile ./
COPY go.mod ./
COPY go.sum ./
COPY cmd ./cmd
COPY pkg ./pkg
ARG VERSION=main
RUN CGO_ENABLED=0 go build -ldflags=" -X github.com/replicatedhq/local-volume-provider/pkg/version.version=$VERSION " -o /go/bin/local-volume-provider ./cmd/local-volume-provider

FROM golang:1.16-buster as build-fileserver
ENV GOPROXY=https://proxy.golang.org
ENV PROJECTPATH=/go/src/github.com/replicatedhq/local-volume-provider/local-volume-fileserver
WORKDIR $PROJECTPATH
COPY Makefile ./
COPY go.mod ./
COPY go.sum ./
COPY cmd ./cmd
COPY pkg ./pkg
ARG VERSION=main
RUN CGO_ENABLED=0 go build -ldflags=" -X github.com/replicatedhq/local-volume-provider/pkg/version.version=$VERSION " -o /go/bin/local-volume-fileserver ./cmd/local-volume-fileserver

FROM debian:buster-slim
RUN apt-get update && apt-get install --no-install-recommends -y dpkg
RUN mkdir /plugins
COPY --from=build-plugin /go/bin/local-volume-provider /plugins/
COPY --from=build-fileserver /go/bin/local-volume-fileserver .
CMD ["/bin/bash", "-c", "cp /plugins/* /target/."]
