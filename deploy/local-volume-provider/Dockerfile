FROM golang:1.16-buster AS build-plugin
ENV GOPROXY=https://proxy.golang.org
WORKDIR /go/src/github.com/replciatedhq/local-volume-provider
COPY . .
RUN CGO_ENABLED=0 go build -o /go/bin/local-volume-provider ./cmd/local-volume-provider

FROM golang:1.16-buster as build-fileserver
ENV PROJECTPATH=/go/src/github.com/replicatedhq/local-volume-provider/local-volume-fileserver
WORKDIR $PROJECTPATH
COPY Makefile ./
COPY go.mod ./
COPY go.sum ./
COPY cmd ./cmd
COPY pkg ./pkg

RUN CGO_ENABLED=0 go build -o /go/bin/local-volume-fileserver ./cmd/local-volume-fileserver

FROM ubuntu:bionic
RUN mkdir /plugins
COPY --from=build-plugin /go/bin/local-volume-provider /plugins/
COPY --from=build-fileserver /go/bin/local-volume-fileserver .
CMD ["/bin/bash", "-c", "cp /plugins/* /target/."]
