package usecase

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain"
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
