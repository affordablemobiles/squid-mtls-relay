FROM golang:1.21 as build-env

ADD . /go/src/bitbucket.org/a1commsltd/squid-mtls
WORKDIR /go/src/bitbucket.org/a1commsltd/squid-mtls

ARG CGO_ENABLED=0

RUN go mod vendor
RUN go build -ldflags "-s -w" -o /go/bin/app

FROM gcr.io/distroless/static-debian12
COPY --from=build-env /go/bin/app /
CMD ["/app"]
