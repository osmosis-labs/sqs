package routertesting

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/balancer"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

// Pool USDT / ETH -> 0.01 spread factor & 5 USDTfor 1 ETH
func (s *RouterTestHelper) PoolOne() (uint64, poolmanagertypes.PoolI) {
	poolIDOne := s.PrepareCustomBalancerPool([]balancer.PoolAsset{
		{
			Token:  sdk.NewCoin(USDT, defaultAmount.MulRaw(5)),
			Weight: osmomath.NewInt(100),
		},
		{
			Token:  sdk.NewCoin(ETH, defaultAmount),
			Weight: osmomath.NewInt(100),
		},
	}, balancer.PoolParams{
		SwapFee: osmomath.NewDecWithPrec(1, 2),
		ExitFee: osmomath.ZeroDec(),
	})

	poolOne, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, poolIDOne)
	s.Require().NoError(err)

	return poolIDOne, poolOne
}

// Pool USDC / USDT -> 0.01 spread factor & 1 USDC for 1 USDT
func (s *RouterTestHelper) PoolTwo() (uint64, poolmanagertypes.PoolI) {
	poolIDTwo := s.PrepareCustomBalancerPool([]balancer.PoolAsset{
		{
			Token:  sdk.NewCoin(USDC, defaultAmount),
			Weight: osmomath.NewInt(100),
		},
		{
			Token:  sdk.NewCoin(USDT, defaultAmount),
			Weight: osmomath.NewInt(100),
		},
	}, balancer.PoolParams{
		SwapFee: osmomath.NewDecWithPrec(3, 2),
		ExitFee: osmomath.ZeroDec(),
	})

	poolTwo, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, poolIDTwo)
	s.Require().NoError(err)

	return poolIDTwo, poolTwo
}

// Pool ETH / USDC -> 0.005 spread factor & 4 USDC for 1 ETH
func (s *RouterTestHelper) PoolThree() (uint64, poolmanagertypes.PoolI) {
	poolIDThree := s.PrepareCustomBalancerPool([]balancer.PoolAsset{
		{
			Token:  sdk.NewCoin(ETH, defaultAmount),
			Weight: osmomath.NewInt(100),
		},
		{
			Token:  sdk.NewCoin(USDC, defaultAmount.MulRaw(4)),
			Weight: osmomath.NewInt(100),
		},
	}, balancer.PoolParams{
		SwapFee: osmomath.NewDecWithPrec(5, 3),
		ExitFee: osmomath.ZeroDec(),
	})

	poolThree, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, poolIDThree)
	s.Require().NoError(err)

	return poolIDThree, poolThree
}
