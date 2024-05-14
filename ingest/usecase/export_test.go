package usecase

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
)

type (
	IngestUseCaseImpl = ingestUseCase
)

func UpdateUniqueDenomData(uniqueDenomData map[string]domain.PoolDenomMetaData, balances sdk.Coins) {
	updateUniqueDenomData(uniqueDenomData, balances)
}
