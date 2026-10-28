# syntax=docker/dockerfile:1

ARG GO_VERSION="1.27"
ARG RUNNER_IMAGE="ubuntu"

# --------------------------------------------------------
# Builder
# --------------------------------------------------------

FROM golang:${GO_VERSION}-alpine AS builder

ARG GIT_VERSION
ARG GIT_COMMIT

WORKDIR /osmosis

RUN set -eux; apk add --no-cache ca-certificates build-base linux-headers

# Dependencies are fetched before the source is copied so that source changes
# reuse these layers.
COPY go.mod go.sum ./
RUN go mod download

# Cosmwasm - Download correct libwasmvm version
RUN ARCH=$(uname -m) && WASMVM_VERSION=$(go list -m github.com/CosmWasm/wasmvm/v2 | sed 's/.* //') && \
    wget https://github.com/CosmWasm/wasmvm/releases/download/$WASMVM_VERSION/libwasmvm_muslc.$ARCH.a \
    -O /lib/libwasmvm_muslc.$ARCH.a && \
    # verify checksum
    wget https://github.com/CosmWasm/wasmvm/releases/download/$WASMVM_VERSION/checksums.txt -O /tmp/checksums.txt && \
    sha256sum /lib/libwasmvm_muslc.$ARCH.a | grep $(cat /tmp/checksums.txt | grep libwasmvm_muslc.$ARCH | cut -d ' ' -f 1)

COPY . .

# --------------------------------------------------------
# sqsd
# --------------------------------------------------------

FROM builder AS sqsd

RUN --mount=type=cache,target=/root/.cache/go-build \
    BUILD_TAGS=muslc LINK_STATICALLY=true GOWORK=off go build -mod=readonly \
    -tags "netgo,ledger,muslc" \
    -ldflags \
    "-X github.com/osmosis-labs/sqs/version=${GIT_VERSION} \
    -w -s -linkmode=external -extldflags '-Wl,-z,muldefs -static'" \
    -v -o /osmosis/build/sqsd /osmosis/app/*.go 

# --------------------------------------------------------
# Runner
# --------------------------------------------------------

FROM ${RUNNER_IMAGE}
COPY --from=sqsd /osmosis/build/sqsd /bin/sqsd
ENV HOME=/osmosis
WORKDIR $HOME
EXPOSE 9092
EXPOSE 50051
RUN apt-get update && \
    apt-get install curl vim nano -y

# Use JSON array format for ENTRYPOINT
# If array is not used, the command arguments to docker run are ignored.
ENTRYPOINT ["/bin/sqsd"]

# Default CMD
CMD ["--host", "sqs-default-host"]
