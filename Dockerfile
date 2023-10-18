# syntax=docker/dockerfile:1

ARG GO_VERSION="1.20"
ARG RUNNER_IMAGE="ubuntu"

# --------------------------------------------------------
# Builder
# --------------------------------------------------------

FROM golang:1.20-alpine as builder

ARG GIT_VERSION
ARG GIT_COMMIT

# Download go dependencies
WORKDIR /osmosis
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    go mod download

# Cosmwasm - Download correct libwasmvm version
RUN ARCH=$(uname -m) && WASMVM_VERSION=$(go list -m github.com/CosmWasm/wasmvm | sed 's/.* //') && \
    wget https://github.com/CosmWasm/wasmvm/releases/download/$WASMVM_VERSION/libwasmvm_muslc.$ARCH.a \
        -O /lib/libwasmvm_muslc.a && \
    # verify checksum
    wget https://github.com/CosmWasm/wasmvm/releases/download/$WASMVM_VERSION/checksums.txt -O /tmp/checksums.txt && \
    sha256sum /lib/libwasmvm_muslc.a | grep $(cat /tmp/checksums.txt | grep libwasmvm_muslc.$ARCH | cut -d ' ' -f 1)

# Copy the remaining files
COPY . .

RUN set -eux; apk add --no-cache ca-certificates build-base;

# needed by github.com/zondax/hid
RUN apk add linux-headers

# Cosmwasm - Download correct libwasmvm version
RUN ARCH=$(uname -m) && WASMVM_VERSION=$(go list -m github.com/CosmWasm/wasmvm | sed 's/.* //') && \
    wget https://github.com/CosmWasm/wasmvm/releases/download/$WASMVM_VERSION/libwasmvm_muslc.$ARCH.a \
        -O /lib/libwasmvm_muslc.a && \
    # verify checksum
    wget https://github.com/CosmWasm/wasmvm/releases/download/$WASMVM_VERSION/checksums.txt -O /tmp/checksums.txt && \
    sha256sum /lib/libwasmvm_muslc.a | grep $(cat /tmp/checksums.txt | grep libwasmvm_muslc.$ARCH | cut -d ' ' -f 1)

RUN BUILD_TAGS=muslc LINK_STATICALLY=true GOWORK=off go build \
    -mod=readonly \
    -tags "netgo,ledger,muslc" \
    -ldflags \
        "-X github.com/cosmos/cosmos-sdk/version.Name="osmosis" \
        -X github.com/cosmos/cosmos-sdk/version.AppName="osmosisd" \
        -X github.com/cosmos/cosmos-sdk/version.Version=${GIT_VERSION} \
        -X github.com/cosmos/cosmos-sdk/version.Commit=${GIT_COMMIT} \
        -X github.com/cosmos/cosmos-sdk/version.BuildTags=netgo,ledger,muslc \
        -w -s -linkmode=external -extldflags '-Wl,-z,muldefs -static'" \
    -v -o /osmosis/build/sqsd /osmosis/app/main.go 

# --------------------------------------------------------
# Runner
# --------------------------------------------------------

FROM ${RUNNER_IMAGE}

COPY --from=builder /osmosis/build/sqsd /bin/sqsd
COPY --from=builder /osmosis/config-mainnet.json /osmosis/config.json

ENV HOME /osmosis
WORKDIR $HOME

EXPOSE 9092

ENTRYPOINT ["/bin/sqsd"]

