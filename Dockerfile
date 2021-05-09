FROM golang:alpine AS builder
ENV GO111MODULE on
RUN apk add --no-cache git
WORKDIR $GOPATH/src/github.com/talos-systems/wglan-manager
COPY . .
RUN go get -d -v
RUN go build -o /go/bin/web

FROM alpine
RUN apk add --no-cache ca-certificates
COPY --from=builder /go/bin/web /go/bin/web
ENTRYPOINT ["/go/bin/web"]
