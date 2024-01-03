# syntax=docker/dockerfile:1

ARG GO_VERSION="1.21"
ARG RUNNER_IMAGE="ubuntu"

# --------------------------------------------------------
# Builder
# --------------------------------------------------------

FROM golang:1.21-alpine as builder

ARG GIT_VERSION
ARG GIT_COMMIT

WORKDIR /osmosis

COPY go.mod go.sum ./
COPY . .

RUN set -eux; apk add --no-cache ca-certificates build-base linux-headers && \
    go mod download

RUN ARCH=$(uname -m) && WASMVM_VERSION=$(go list -m github.com/CosmWasm/wasmvm | sed 's/.* //') && \
    wget https://github.com/CosmWasm/wasmvm/releases/download/$WASMVM_VERSION/libwasmvm_muslc.$ARCH.a \
        -O /lib/libwasmvm_muslc.a && \
    wget https://github.com/CosmWasm/wasmvm/releases/download/$WASMVM_VERSION/checksums.txt -O /tmp/checksums.txt && \
    sha256sum /lib/libwasmvm_muslc.a | grep $(cat /tmp/checksums.txt | grep libwasmvm_muslc.$ARCH | cut -d ' ' -f 1)

RUN BUILD_TAGS=muslc LINK_STATICALLY=true GOWORK=off go build -mod=readonly \
    -tags "netgo,ledger,muslc" \
    -ldflags "-w -s -linkmode=external -extldflags '-Wl,-z,muldefs -static'" \
    -v -o /osmosis/build/sqsd /osmosis/app/*.go 

# --------------------------------------------------------
# Runner
# --------------------------------------------------------

FROM ${RUNNER_IMAGE}
COPY --from=builder /osmosis/build/sqsd /bin/sqsd
ENV HOME /osmosis
WORKDIR $HOME
EXPOSE 9092
RUN apt-get update && \
    apt-get install curl vim nano -y

ENTRYPOINT ["/bin/sqsd"]

