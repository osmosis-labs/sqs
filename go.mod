module github.com/osmosis-labs/router

go 1.12

require (
	github.com/cosmos/cosmos-sdk v0.47.5
	github.com/go-redis/redismock/v9 v9.2.0
	github.com/labstack/echo v3.3.10+incompatible
	github.com/labstack/gommon v0.4.0 // indirect
	github.com/onsi/ginkgo/v2 v2.13.0
	github.com/onsi/gomega v1.28.0
	github.com/osmosis-labs/osmosis/osmomath v0.0.8-0.20230926014346-27a13ec134bd
	github.com/osmosis-labs/osmosis/v19 v19.2.0
	github.com/redis/go-redis/v9 v9.2.1
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/viper v1.17.0
	github.com/stretchr/testify v1.8.4
	github.com/valyala/fasttemplate v1.2.2 // indirect
)

replace (
	// Our cosmos-sdk branch is:  https://github.com/osmosis-labs/cosmos-sdk, current branch: osmosis-main. Direct commit link: https://github.com/osmosis-labs/cosmos-sdk/commit/05346fa12992
	github.com/cosmos/cosmos-sdk => github.com/osmosis-labs/cosmos-sdk v0.45.0-rc1.0.20230922030206-734f99fba785
	// use cosmos-compatible protobufs
	github.com/gogo/protobuf => github.com/regen-network/protobuf v1.3.3-alpha.regen.1

	// Informal Tendermint fork
	github.com/tendermint/tendermint => github.com/informalsystems/tendermint v0.34.24
)
