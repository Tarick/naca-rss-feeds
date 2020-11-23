FROM golang:1.15.5-alpine3.12 as build
RUN apk add --no-cache git && \
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go get -v -tags 'netgo static_build' -ldflags "-extldflags=-static -w -s " github.com/jackc/tern

FROM alpine:3.12
RUN adduser -u 1000 -h /opt/app -D app 
WORKDIR /opt/app
USER app
COPY --from=build /go/bin/tern ./tern
COPY migrations/migrations ./migrations
ENTRYPOINT ["/opt/app/tern"]
CMD ["--help"]
# tern.conf file should be mounted to /opt/app/tern.conf
# CMD ["migrate", "--migrations", "migrations]
