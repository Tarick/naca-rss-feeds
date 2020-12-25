SHELL:=/bin/bash

.DEFAULT_GOAL := help
# put here commands, that have the same name as files in dir
.PHONY: run clean generate build docker_build docker_push build-and-deploy build-migrations-image

BUILD_TAG=$(shell git describe --tags --abbrev=0 HEAD)
BUILD_HASH=$(shell git rev-parse --short HEAD)
BUILD_BRANCH=$(shell git symbolic-ref HEAD |cut -d / -f 3)
BUILD_VERSION="${BUILD_TAG}-${BUILD_HASH}"
BUILD_TIME=$(shell date --utc +%F-%H:%m:%SZ)
PACKAGE=naca-rss-feeds
LDFLAGS=-extldflags=-static -w -s -X ${PACKAGE}/internal/version.Version=${BUILD_VERSION} -X ${PACKAGE}/internal/version.BuildTime=${BUILD_TIME}
# This CONTAINER_REGISTRY must be sourced from environment and it must be FQDN,
# containerd registry plugin doesn't give a shit about short names even if they're present locally, appends docker.io to it
CONTAINER_IMAGE_REGISTRY=${CONTAINER_REGISTRY_FQDN}/rss-feeds

help:
	@echo "build, build-images, deps, build-worker, build-api, build-worker-image, build-api-image, generate-api, build-and-deploy, deploy-to-local-k8s"
	
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
	buildctl build --frontend dockerfile.v0 --opt build-arg:BUILD_VERSION=${BUILD_VERSION} \
	--local context=. --local dockerfile=cmd/feeds-worker  \
	--output type=image,\"name=${CONTAINER_IMAGE_REGISTRY}/rss-feeds-worker:${BUILD_BRANCH}-${BUILD_HASH},${CONTAINER_IMAGE_REGISTRY}/rss-feeds-worker:${BUILD_VERSION}\"
	@echo "[INFO] Image built successfully"

generate-worker:
	@echo "[INFO] Running code generations for Worker"
	go generate cmd/feeds-worker/main.go

build-api-image:
	@echo "[INFO] Building API container image"
	buildctl build --frontend dockerfile.v0 --opt build-arg:BUILD_VERSION=${BUILD_VERSION} \
	--local context=. --local dockerfile=cmd/feeds-api  \
	--output type=image,\"name=${CONTAINER_IMAGE_REGISTRY}/rss-feeds-api:${BUILD_BRANCH}-${BUILD_HASH},${CONTAINER_IMAGE_REGISTRY}/rss-feeds-api:${BUILD_VERSION}\"
	@echo "[INFO] Image built successfully"

build-migrations-image:
	@echo "[INFO] Building API container image"
	buildctl build --frontend dockerfile.v0 --opt build-arg:BUILD_VERSION=${BUILD_VERSION} \
	--local context=. --local dockerfile=migrations  \
	--output type=image,\"name=${CONTAINER_IMAGE_REGISTRY}/rss-feeds-sql-migrations:${BUILD_BRANCH}-${BUILD_HASH},${CONTAINER_IMAGE_REGISTRY}/rss-feeds-sql-migrations:${BUILD_VERSION}\"
	@echo "[INFO] Image built successfully"


build-and-deploy: build-images deploy-to-local-k8s

deploy-to-local-k8s: 
	@echo "[INFO] Deploying current RSS feeds to local k8s service"
	@echo "[INFO] Deleting old SQL migrations"
	helmfile --environment local --selector app_name=rss-feeds-sql-migrations -f ../naca-ops-config/helm/helmfile.yaml destroy
	# Ugly workaround for helm 3.4 'spec.clusterIP: Invalid value: "": field is immutable' bug
	helmfile --environment local --selector app_name=rss-feeds-api -f ../naca-ops-config/helm/helmfile.yaml destroy
	@echo "[INFO] Deploying rss-feeds images with tag ${BUILD_VERSION}"
	RSS_FEEDS_TAG=${BUILD_VERSION} helmfile --environment local --selector tier=naca-rss-feeds -f ../naca-ops-config/helm/helmfile.yaml sync --skip-deps
