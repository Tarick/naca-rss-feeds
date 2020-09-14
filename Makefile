SHELL:=/bin/bash

.DEFAULT_GOAL := help
# put here commands, that have the same name as files in dir
.PHONY: run clean generate build docker_build docker_push

BUILD_TAG=$(shell git describe --tags --abbrev=0 HEAD)
BUILD_HASH=$(shell git rev-parse --short HEAD)
BUILD_BRANCH=$(shell git symbolic-ref HEAD |cut -d / -f 3)
BUILD_VERSION="${BUILD_TAG}-${BUILD_HASH}"
BUILD_TIME=$(shell date --utc +%F-%H:%m:%SZ)
PACKAGE=naca-rss-feeds
LDFLAGS=-extldflags=-static -w -s -X ${PACKAGE}/internal/version.Version=${BUILD_VERSION} -X ${PACKAGE}/internal/version.BuildTime=${BUILD_TIME}
CONTAINER_IMAGE_REGISTRY=local/rss-feeds

help:
	@echo "build, build-images, deps, build-worker, build-api, build-worker-image, build-api-image, generate-api"
	
version:
	@echo "${BUILD_VERSION}"

# ONLY TABS IN THE START OF COMMAND, NO SPACES!
build: deps build-worker build-api

build-images: build-worker-image build-api-image build-migrations-image

clean:
	@echo "[INFO] Cleaning build files"
	rm -f build/*

version:

deps:
	@echo "[INFO] Downloading and installing dependencies"
	go mod download

build-worker: deps generate-worker
	@echo "[INFO] Building worker binary"
	go build -ldflags "${LDFLAGS}" -o build/feeds-worker ./cmd/feeds-worker
	@echo "[INFO] Build successful"

build-api: deps generate-api
	@echo "[INFO] Building API Server binary"
	go build -ldflags "${LDFLAGS}" -o build/feeds-api ./cmd/feeds-api
	@echo "[INFO] Build successful"

generate-api:
	@echo "[INFO] Running code generations for API"
	go generate cmd/feeds-api/main.go

build-worker-image:
	@echo "[INFO] Building worker container image"
	docker build -t ${CONTAINER_IMAGE_REGISTRY}/rss-feeds-worker:${BUILD_BRANCH}-${BUILD_HASH} \
	-t ${CONTAINER_IMAGE_REGISTRY}/rss-feeds-worker:${BUILD_VERSION} \
	--build-arg BUILD_VERSION=${BUILD_VERSION} -f cmd/feeds-worker/Dockerfile .

generate-worker:
	@echo "[INFO] Running code generations for Worker"
	go generate cmd/feeds-worker/main.go

build-api-image:
	@echo "[INFO] Building API container image"
	docker build -t ${CONTAINER_IMAGE_REGISTRY}/rss-feeds-api:${BUILD_BRANCH}-${BUILD_HASH} \
	-t ${CONTAINER_IMAGE_REGISTRY}/rss-feeds-api:${BUILD_VERSION} \
	--build-arg BUILD_VERSION=${BUILD_VERSION} -f cmd/feeds-api/Dockerfile .

build-migrations-image:
	@echo "[INFO] Building API container image"
	docker build -t ${CONTAINER_IMAGE_REGISTRY}/rss-feeds-sql-migrations:${BUILD_BRANCH}-${BUILD_HASH} \
	-t ${CONTAINER_IMAGE_REGISTRY}/rss-feeds-sql-migrations:${BUILD_VERSION} \
	-f migrations/Dockerfile .
