package usecase

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
)

type (
	IngestUseCaseImpl = ingestUseCase
)

func UpdateCurrentBlockLiquidityMapFromBalances(currentBlockLiquidityMap domain.DenomLiquidityMap, currentPoolBalances sdk.Coins, poolID uint64) domain.DenomLiquidityMap {
	return updateCurrentBlockLiquidityMapFromBalances(currentBlockLiquidityMap, currentPoolBalances, poolID)
}
