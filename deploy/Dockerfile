FROM --platform=$BUILDPLATFORM golang:1.22-bullseye AS build-plugin
ENV GOPROXY=https://proxy.golang.org
ENV PROJECTPATH=/go/src/github.com/replicatedhq/local-volume-provider/local-volume-fileserver
ARG TARGETARCH
ARG BUILDPLATFORM
WORKDIR $PROJECTPATH
COPY Makefile ./
COPY go.mod ./
COPY go.sum ./
COPY cmd ./cmd
COPY pkg ./pkg
ARG VERSION=main
RUN CGO_ENABLED=0 GOARCH=${TARGETARCH} go build -ldflags=" -X github.com/replicatedhq/local-volume-provider/pkg/version.version=$VERSION " -o /go/bin/local-volume-provider ./cmd/local-volume-provider

FROM --platform=$BUILDPLATFORM golang:1.22-bullseye AS build-fileserver
ENV GOPROXY=https://proxy.golang.org
ENV PROJECTPATH=/go/src/github.com/replicatedhq/local-volume-provider/local-volume-fileserver
WORKDIR $PROJECTPATH
COPY Makefile ./
COPY go.mod ./
COPY go.sum ./
COPY cmd ./cmd
COPY pkg ./pkg
ARG VERSION=main
RUN CGO_ENABLED=0 GOARCH=${TARGETARCH} go build -ldflags=" -X github.com/replicatedhq/local-volume-provider/pkg/version.version=$VERSION " -o /go/bin/local-volume-fileserver ./cmd/local-volume-fileserver

FROM debian:bullseye-slim
RUN mkdir /plugins
COPY --from=build-plugin /go/bin/local-volume-provider /plugins/
COPY --from=build-fileserver /go/bin/local-volume-fileserver .
CMD ["/bin/bash", "-c", "cp /plugins/* /target/."]
