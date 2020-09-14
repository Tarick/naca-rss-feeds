FROM golang:1.15.0-alpine as build
RUN apk add --no-cache git && \
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go get -v -tags 'netgo static_build' -ldflags "-extldflags=-static -w -s " github.com/rubenv/sql-migrate/...

FROM alpine:3.12
RUN adduser -u 1000 -h /opt/app -D app 
WORKDIR /opt/app
USER app
COPY --from=build /go/bin/sql-migrate ./sql-migrate
COPY migrations/ ./migrations
ENTRYPOINT ["/opt/app/sql-migrate"]
CMD ["--help"]
# Always env - production
# dbconfig.yml file should be mounted to /opt/app/dbconfig.yml
# CMD ["up", , "-env", "production"]