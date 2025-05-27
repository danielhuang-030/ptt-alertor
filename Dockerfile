# building binary
FROM golang:1.21-alpine AS builder

ENV GOPATH=/go \
    GO_WORKDIR=$GOPATH/src/github.com/Ptt-Alertor/ptt-alertor \
    GO111MODULE=on \
    CGO_ENABLED=0

WORKDIR $GO_WORKDIR
COPY . $GO_WORKDIR

RUN go mod download
RUN go install

# building executable image
FROM alpine:latest

RUN set -eux; \
    apk add --no-cache ca-certificates

COPY public/ public/
COPY --from=builder /go/bin/ptt-alertor /usr/local/bin/ptt-alertor

ENTRYPOINT ["/usr/local/bin/ptt-alertor"]
EXPOSE 9090 6060
