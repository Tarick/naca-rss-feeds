FROM golang:1.15.6-alpine as build
RUN apk --no-cache add tzdata git
WORKDIR /app
COPY go.mod .
COPY go.sum .
RUN GOOS=linux GOARCH=amd64 go mod download
ENV PACKAGE=rss-feeds
ARG BUILD_VERSION
ADD . .
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -tags 'netgo static_build' -ldflags "-extldflags=-static -w -s -X ${PACKAGE}/internal/version.Version=$BUILD_VERSION -X ${PACKAGE}/internal/version.BuildTime=$(date --utc  +%FT%TZ)" -o ./build/feeds-worker ./cmd/feeds-worker 

FROM scratch as final
WORKDIR /
ENV TZ=UTC
COPY --from=build /app/build/feeds-worker /
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /etc/ssl/certs /etc/ssl/certs
ENTRYPOINT ["./feeds-worker"]
