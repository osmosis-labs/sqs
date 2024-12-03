package mocks

import (
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

type ChainPoolMock struct {
	ID   uint64
	Type poolmanagertypes.PoolType
}

// AsSerializablePool implements types.PoolI.
func (c *ChainPoolMock) AsSerializablePool() poolmanagertypes.PoolI {
	panic("unimplemented")
}

// GetAddress implements types.PoolI.
func (c *ChainPoolMock) GetAddress() types.AccAddress {
	panic("unimplemented")
}

// GetId implements types.PoolI.
func (c *ChainPoolMock) GetId() uint64 {
	return c.ID
}

// GetPoolDenoms implements types.PoolI.
func (c *ChainPoolMock) GetPoolDenoms(types.Context) []string {
	panic("unimplemented")
}

// GetSpreadFactor implements types.PoolI.
func (c *ChainPoolMock) GetSpreadFactor(ctx types.Context) math.LegacyDec {
	panic("unimplemented")
}

// GetType implements types.PoolI.
func (c *ChainPoolMock) GetType() poolmanagertypes.PoolType {
	return c.Type
}

// IsActive implements types.PoolI.
func (c *ChainPoolMock) IsActive(ctx types.Context) bool {
	panic("unimplemented")
}

// ProtoMessage implements types.PoolI.
func (c *ChainPoolMock) ProtoMessage() {
	panic("unimplemented")
}

// Reset implements types.PoolI.
func (c *ChainPoolMock) Reset() {
	panic("unimplemented")
}

// SpotPrice implements types.PoolI.
func (c *ChainPoolMock) SpotPrice(ctx types.Context, quoteAssetDenom string, baseAssetDenom string) (osmomath.BigDec, error) {
	panic("unimplemented")
}

// String implements types.PoolI.
func (c *ChainPoolMock) String() string {
	panic("unimplemented")
}

var _ poolmanagertypes.PoolI = &ChainPoolMock{}
