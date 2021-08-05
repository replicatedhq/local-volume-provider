FROM golang:1.16 as builder

ENV PROJECTPATH=/go/src/github.com/replicatedhq/local-volume-provider/local-volume-fileserver
WORKDIR $PROJECTPATH
COPY Makefile ./
COPY go.mod ./
COPY go.sum ./
COPY cmd ./cmd
COPY pkg ./pkg

RUN CGO_ENABLED=0 go build -o /go/bin/local-volume-fileserver ./cmd/local-volume-fileserver


FROM scratch
COPY --from=builder /go/bin/local-volume-fileserver .

ENTRYPOINT ["/local-volume-fileserver"]
