FROM golang:1.16-buster AS build
ENV GOPROXY=https://proxy.golang.org
WORKDIR /go/src/github.com/replciatedhq/local-volume-provider
COPY . .
RUN CGO_ENABLED=0 go build -o /go/bin/local-volume-provider ./cmd/local-volume-provider


FROM ubuntu:bionic
RUN mkdir /plugins
COPY --from=build /go/bin/local-volume-provider /plugins/
USER nobody:nogroup
ENTRYPOINT ["/bin/bash", "-c", "cp /plugins/* /target/."]
