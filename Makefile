# Exporting bin folder to the path for makefile
export PATH   := $(PWD)/bin:$(PATH)
# Default Shell
export SHELL  := bash
# Type of OS: Linux or Darwin.
export OSTYPE := $(shell uname -s)

VERSION := $(shell echo $(shell git describe --tags) | sed 's/^v//')
COMMIT := $(shell git log -1 --format='%H')
GO_VERSION := $(shell cat go.mod | grep -E 'go [0-9].[0-9]+' | cut -d ' ' -f 2)
PACKAGES_UNIT=$(shell go list ./...)

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

swagger-gen:
	$(HOME)/go/bin/swag init -g app/main.go

run:
	go run -ldflags="-X github.com/osmosis-labs/sqs/version=${VERSION}" app/*.go  --config config.json

run-docker:
	docker rm -f sqs
	docker run -d --name sqs -p 9092:9092 -p 26657:26657 -v /root/sqs/config-testnet.json/:/osmosis/config.json --net host osmolabs/sqs:local "--config /osmosis/config.json"
	docker logs -f sqs

# Note: we migrated away from Redis.
# This is left in case we require more data in the near future
# prompting the need for Redis.
redis-start:
	docker run -d --name redis-stack -p 6379:6379 -p 8001:8001 -v ./redis-cache/:/data redis/redis-stack:7.2.0-v3

# Note: we migrated away from Redis.
# This is left in case we require more data in the near future
# prompting the need for Redis.
redis-stop:
	docker container rm -f redis-stack
osmosis-start:
	docker run -d --name osmosis -p 26657:26657 -p 9090:9090 -p 1317:1317 -p 9091:9091 -p 6060:6060 -v $(HOME)/.osmosisd/:/osmosis/.osmosisd/ --net host osmolabs/osmosis-dev:sqs-out-v0.2 "start"

osmosis-stop:
	docker container rm -f osmosis

all-stop: osmosis-stop

all-start: osmosis-start run

lint:
	@echo "--> Running linter"
	golangci-lint run --timeout=10m

test-unit:
	@VERSION=$(VERSION) go test -mod=readonly $(PACKAGES_UNIT)

build:
	BUILD_TAGS=muslc LINK_STATICALLY=true GOWORK=off go build -mod=readonly \
    -tags "netgo,ledger,muslc" \
    -ldflags "-w -s -linkmode=external -extldflags '-Wl,-z,muldefs -static'" \
    -v -o /osmosis/build/sqsd app/*.go 

###############################################################################
###                                Docker                                  ###
###############################################################################

docker-build:
	@DOCKER_BUILDKIT=1 docker build \
		-t osmolabs/sqs:0.7.3 \
		--build-arg GO_VERSION=$(GO_VERSION) \
		--build-arg GIT_VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(COMMIT) \
		-f Dockerfile .


###############################################################################
###                                Utils                                    ###
###############################################################################

load-test-ui:
	docker compose -f locust/docker-compose.yml up --scale worker=4

profile:
	go tool pprof -http=:8080 http://localhost:9092/debug/pprof/profile

# Validates that SQS concentrated liquidity pool state is
# consistent with the state of the chain.
validate-cl-state:
	scripts/validate-cl-state.sh "http://localhost:9092"

# Compares the quotes between SQS and chain over pool 1136
# which is concentrated.
quote-compare:
	scripts/quote.sh "http://localhost:9092"

sqs-quote-compare-stage:
	ingest/sqs/scripts/quote.sh "http://165.227.168.61"

# Updates go tests with the latest mainnet state
# Make sure that the node is running locally
sqs-update-mainnet-state:
	curl -X POST "http:/localhost:9092/router/store-state"
	mv pools.json router/usecase/routertesting/parsing/pools.json
	mv taker_fees.json router/usecase/routertesting/parsing/taker_fees.json

	curl -X POST "http:/localhost:9092/tokens/store-state"
	mv tokens.json router/usecase/routertesting/parsing/tokens.json

# Bench tests pricing
bench-pricing:
	go test -bench BenchmarkGetPrices -run BenchmarkGetPrices github.com/osmosis-labs/sqs/tokens/usecase -count=6

proto-gen:
	protoc --go_out=./ --go-grpc_out=./ --proto_path=./sqsdomain/proto ./sqsdomain/proto/ingest.proto
