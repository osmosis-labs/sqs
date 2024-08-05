package usecase

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
)

type (
	IngestUseCaseImpl = ingestUseCase
)

func UpdateCurrentBlockLiquidityMapFromBalances(currentBlockLiquidityMap domain.DenomPoolLiquidityMap, currentPoolBalances sdk.Coins, poolID uint64) domain.DenomPoolLiquidityMap {
	return updateCurrentBlockLiquidityMapFromBalances(currentBlockLiquidityMap, currentPoolBalances, poolID)
}

func TransferDenomLiquidityMap(transferTo, transferFrom domain.DenomPoolLiquidityMap) domain.DenomPoolLiquidityMap {
	return transferDenomLiquidityMap(transferTo, transferFrom)
}

func ProcessSQSModelMut(sqsModel *sqsdomain.SQSPool) error {
	return processSQSModelMut(sqsModel)
}

func UpdateCurrentBlockLiquidityMapAlloyed(currentBlockLiquidityMap domain.DenomPoolLiquidityMap, poolID uint64, alloyedDenom string) domain.DenomPoolLiquidityMap {
	return updateCurrentBlockLiquidityMapAlloyed(currentBlockLiquidityMap, poolID, alloyedDenom)
}

func ComputeStandardNormalizationFactor(assetConfigs []cosmwasmpool.TransmuterAssetConfig) (osmomath.Int, error) {
	return computeStandardNormalizationFactor(assetConfigs)
}

func ComputeNormalizationScalingFactors(standardNormalizationFactor osmomath.Int, assetConfigs []cosmwasmpool.TransmuterAssetConfig) ([]osmomath.Int, error) {
	return computeNormalizationScalingFactors(standardNormalizationFactor, assetConfigs)
}
