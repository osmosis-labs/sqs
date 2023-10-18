# Exporting bin folder to the path for makefile
export PATH   := $(PWD)/bin:$(PATH)
# Default Shell
export SHELL  := bash
# Type of OS: Linux or Darwin.
export OSTYPE := $(shell uname -s)

VERSION := $(shell echo $(shell git describe --tags) | sed 's/^v//')
COMMIT := $(shell git log -1 --format='%H')
GO_VERSION := $(shell cat go.mod | grep -E 'go [0-9].[0-9]+' | cut -d ' ' -f 2)

BUILD_FLAGS := -tags "$(build_tags)" -ldflags '$(ldflags)'
# check for nostrip option
ifeq (,$(findstring nostrip,$(OSMOSIS_BUILD_OPTIONS)))
  BUILD_FLAGS += -trimpath
endif


# --- Tooling & Variables ----------------------------------------------------------------
include ./misc/make/tools.Makefile

# Install local dependencies
install-deps: mockery

deps: $(MOCKERY) ## Checks for Global Development Dependencies.
deps:
	@echo "Required Tools Are Available"

generate-mocks: mockery
	bin/mockery --config mockery.yaml

run:
	go run app/main.go

redis-start:
	docker run -d --name redis-stack -p 6379:6379 -p 8001:8001 -v ./redis-cache/:/data redis/redis-stack:7.2.0-v3

redis-stop:
	docker container rm -f redis-stack

lint:
	@echo "--> Running linter"
	golangci-lint run --timeout=10m

###############################################################################
###                                Docker                                  ###
###############################################################################

docker-build:
	@DOCKER_BUILDKIT=1 docker build \
		-t sqs:local \
		-t sqs:local-distroless \
		--build-arg GO_VERSION=$(GO_VERSION) \
		--build-arg GIT_VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(COMMIT) \
		-f Dockerfile .
