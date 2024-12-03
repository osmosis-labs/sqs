package parsing

import (
	"fmt"
	"os"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/json"

	concentratedmodel "github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/model"
	cosmwasmpoolmodel "github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/balancer"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/stableswap"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

// SerializedPool is a struct that is used to serialize a pool to JSON.
type SerializedPool struct {
	Type      poolmanagertypes.PoolType `json:"type"`
	ChainPool json.RawMessage           `json:"data"`
	SQSModel  sqsdomain.SQSPool         `json:"sqs_model"`
	TickModel *sqsdomain.TickModel      `json:"tick_model,omitempty"`
}

// StorePools stores the pools to a file.
func StorePools(actualPools []sqsdomain.PoolI, tickModelMap map[uint64]*sqsdomain.TickModel, poolsFile string) error {
	_, err := os.Stat(poolsFile)
	if os.IsNotExist(err) {
		file, err := os.Create(poolsFile)
		if err != nil {
			return err
		}
		defer file.Close()

		pools := make([]json.RawMessage, 0, len(actualPools))

		for _, pool := range actualPools {
			if pool.GetType() == poolmanagertypes.Concentrated {
				tickModel, ok := tickModelMap[pool.GetId()]
				if !ok {
					return fmt.Errorf("no tick model in map %s", domain.ConcentratedTickModelNotSetError{
						PoolId: pool.GetId(),
					})
				}
				if err := pool.SetTickModel(tickModel); err != nil {
					return err
				}
			}

			poolData, err := MarshalPool(pool)
			if err != nil {
				return err
			}

			pools = append(pools, poolData)
		}

		poolsJSON, err := json.Marshal(pools)
		if err != nil {
			return err
		}

		_, err = file.Write(poolsJSON)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return nil
}

// StoreTakerFees stores the taker fees to a file.
func StoreTakerFees(takerFeesFile string, takerFeesMap sqsdomain.TakerFeeMap) error {
	_, err := os.Stat(takerFeesFile)
	if os.IsNotExist(err) {
		file, err := os.Create(takerFeesFile)
		if err != nil {
			return err
		}
		defer file.Close()

		takerFeesJSON, err := json.Marshal(takerFeesMap)
		if err != nil {
			return err
		}

		_, err = file.Write(takerFeesJSON)
		if err != nil {
			return err
		}
	}

	return nil
}

// StoreTokensMetadata stores the tokens meta data to disk at the given path.
func StoreTokensMetadata(poolDenomMetaData map[string]domain.Token, tokensFile string) error {
	_, err := os.Stat(tokensFile)
	if os.IsNotExist(err) {
		file, err := os.Create(tokensFile)
		if err != nil {
			return err
		}
		defer file.Close()

		tokensJSON, err := json.Marshal(poolDenomMetaData)
		if err != nil {
			return err
		}

		_, err = file.Write(tokensJSON)
		if err != nil {
			return err
		}
	}

	return nil
}

// StorePoolDenomMetaData stores the pool denom meta data to disk at the given path.
func StorePoolDenomMetaData(poolDenomMetaData domain.PoolDenomMetaDataMap, poolDenomMetaDataFile string) error {
	_, err := os.Stat(poolDenomMetaDataFile)
	if os.IsNotExist(err) {
		file, err := os.Create(poolDenomMetaDataFile)
		if err != nil {
			return err
		}
		defer file.Close()

		takerFeesJSON, err := json.Marshal(poolDenomMetaData)
		if err != nil {
			return err
		}

		_, err = file.Write(takerFeesJSON)
		if err != nil {
			return err
		}
	}

	return nil
}

type candidateRouteSerializedData struct {
	Denom      string                              `json:"denom"`
	Pool       []json.RawMessage                   `json:"pool"`
	Orderbooks []candidateRouteOrderbookSearchData `json:"orderbooks"`
}

type candidateRouteOrderbookSearchData struct {
	PairDenom string          `json:"pair_denom"`
	Orderbook json.RawMessage `json:"orderbook"`
}

// StoreCandidateRouteSearchData stores the candidate route search data to disk at the given path.
func StoreCandidateRouteSearchData(candidateRouteSearchData map[string]domain.CandidateRouteDenomData, candidateRouteSearchDataFile string) error {
	_, err := os.Stat(candidateRouteSearchDataFile)
	if os.IsNotExist(err) {
		file, err := os.Create(candidateRouteSearchDataFile)
		if err != nil {
			return err
		}
		defer file.Close()

		serializedResult := make([]candidateRouteSerializedData, 0, len(candidateRouteSearchData))

		for denom, candidateRouteSearchData := range candidateRouteSearchData {
			serializedPools := make([]json.RawMessage, 0)
			for _, pool := range candidateRouteSearchData.SortedPools {
				serializedPool, err := MarshalPool(pool)
				if err != nil {
					return err
				}
				serializedPools = append(serializedPools, serializedPool)
			}

			serializedOrderbooks := make([]candidateRouteOrderbookSearchData, 0)
			for pairDenom, orderbook := range candidateRouteSearchData.CanonicalOrderbooks {
				serializedOrderbook, err := MarshalPool(orderbook)
				if err != nil {
					return err
				}

				orderbookData := candidateRouteOrderbookSearchData{
					PairDenom: pairDenom,
					Orderbook: serializedOrderbook,
				}

				serializedOrderbooks = append(serializedOrderbooks, orderbookData)
			}

			serializedResult = append(serializedResult, candidateRouteSerializedData{
				Denom:      denom,
				Pool:       serializedPools,
				Orderbooks: serializedOrderbooks,
			})
		}

		candidateRouteSearchDataJSON, err := json.Marshal(serializedResult)
		if err != nil {
			return err
		}

		_, err = file.Write(candidateRouteSearchDataJSON)
		if err != nil {
			return err
		}
	}

	return nil
}

// ReadPools reads the pools from a file and returns them
func ReadPools(poolsFile string) ([]sqsdomain.PoolI, map[uint64]*sqsdomain.TickModel, error) {
	poolBytes, err := os.ReadFile(poolsFile)
	if err != nil {
		return nil, nil, err
	}

	var serializedPools []SerializedPool
	err = json.Unmarshal(poolBytes, &serializedPools)
	if err != nil {
		return nil, nil, err
	}

	actualPools := make([]sqsdomain.PoolI, 0, len(serializedPools))

	tickMap := make(map[uint64]*sqsdomain.TickModel)

	for _, pool := range serializedPools {
		poolWrapper, err := UnmarshalPool(pool)
		if err != nil {
			return nil, nil, err
		}

		if poolWrapper.GetType() == poolmanagertypes.Concentrated {
			tickMap[poolWrapper.GetId()] = pool.TickModel
		}

		actualPools = append(actualPools, poolWrapper)
	}

	return actualPools, tickMap, nil
}

// ReadTakerFees reads the taker fees from a file and returns them
func ReadTakerFees(takerFeeFileName string) (sqsdomain.TakerFeeMap, error) {
	takerFeeBytes, err := os.ReadFile(takerFeeFileName)
	if err != nil {
		return nil, err
	}

	takerFeeMap := sqsdomain.TakerFeeMap{}
	err = json.Unmarshal(takerFeeBytes, &takerFeeMap)
	if err != nil {
		return nil, err
	}

	return takerFeeMap, nil
}

// ReadTokensMetadata reads the tokens meta data from disk at the given path and returns them.
func ReadTokensMetadata(tokensMetadataFileName string) (map[string]domain.Token, error) {
	tokensMetadataBytes, err := os.ReadFile(tokensMetadataFileName)
	if err != nil {
		return nil, err
	}

	tokensMetadata := map[string]domain.Token{}
	err = json.Unmarshal(tokensMetadataBytes, &tokensMetadata)
	if err != nil {
		return nil, err
	}

	return tokensMetadata, nil
}

// ReadPoolDenomsMetaData reads the pool denom meta data from disk at the given path and returns them.
func ReadPoolDenomsMetaData(poolDenomMetaData string) (domain.PoolDenomMetaDataMap, error) {
	tokensMetadataBytes, err := os.ReadFile(poolDenomMetaData)
	if err != nil {
		return nil, err
	}

	tokensMetadata := domain.PoolDenomMetaDataMap{}
	err = json.Unmarshal(tokensMetadataBytes, &tokensMetadata)
	if err != nil {
		return nil, err
	}

	return tokensMetadata, nil
}

// ReadCandidateRouteSearchData reads the candidate route search data from disk at the
func ReadCandidateRouteSearchData(candidateRouteSearchDataFile string) (map[string]domain.CandidateRouteDenomData, error) {
	candidateRouteSearchDataBytes, err := os.ReadFile(candidateRouteSearchDataFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	serialized := make([]candidateRouteSerializedData, 0)
	err = json.Unmarshal(candidateRouteSearchDataBytes, &serialized)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	candidateRouteSearchData := make(map[string]domain.CandidateRouteDenomData, len(serialized))

	for _, data := range serialized {
		pools := make([]sqsdomain.PoolI, 0, len(data.Pool))
		for _, poolData := range data.Pool {
			var serializedPool SerializedPool

			if err := json.Unmarshal(poolData, &serializedPool); err != nil {
				return nil, err
			}

			pool, err := UnmarshalPool(serializedPool)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal pool: %w", err)
			}

			pools = append(pools, pool)
		}

		orderbooks := make(map[string]sqsdomain.PoolI)
		for _, orderbookData := range data.Orderbooks {
			var serializedPool SerializedPool

			if err := json.Unmarshal(orderbookData.Orderbook, &serializedPool); err != nil {
				return nil, err
			}

			pool, err := UnmarshalPool(serializedPool)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal pool: %w", err)
			}

			orderbooks[orderbookData.PairDenom] = pool
		}

		candidateRouteSearchData[data.Denom] = domain.CandidateRouteDenomData{
			SortedPools:         pools,
			CanonicalOrderbooks: orderbooks,
		}
	}

	return candidateRouteSearchData, nil
}

// MarshalPool marshals a pool to JSON.
func MarshalPool(pool sqsdomain.PoolI) (json.RawMessage, error) {
	poolType := pool.GetType()

	underlyingPool := pool.GetUnderlyingPool()

	chainPoolBz, err := json.Marshal(underlyingPool)
	if err != nil {
		return nil, err
	}

	var tickModel *sqsdomain.TickModel
	if poolType == poolmanagertypes.Concentrated {
		tickModel, err = pool.GetTickModel()
		if err != nil {
			return nil, err
		}
	}

	serializedPool := SerializedPool{
		Type:      poolType,
		ChainPool: chainPoolBz,
		SQSModel:  pool.GetSQSPoolModel(),
		TickModel: tickModel,
	}

	poolData, err := json.Marshal(serializedPool)
	if err != nil {
		return nil, err
	}

	return poolData, nil
}

// UnmarshalPool unmarshals a pool from JSON.
func UnmarshalPool(serializedPool SerializedPool) (sqsdomain.PoolI, error) {
	var (
		chainModel poolmanagertypes.PoolI
	)

	switch serializedPool.Type {
	case poolmanagertypes.Concentrated:
		var concentratedPool concentratedmodel.Pool
		err := json.Unmarshal(serializedPool.ChainPool, &concentratedPool)
		if err != nil {
			return nil, err
		}
		chainModel = &concentratedPool
	case poolmanagertypes.CosmWasm:
		var transmuterPool cosmwasmpoolmodel.CosmWasmPool
		err := json.Unmarshal(serializedPool.ChainPool, &transmuterPool)
		if err != nil {
			return nil, err
		}
		chainModel = &transmuterPool
	case poolmanagertypes.Stableswap:
		var balancerPool stableswap.Pool
		err := json.Unmarshal(serializedPool.ChainPool, &balancerPool)
		if err != nil {
			return nil, err
		}
		chainModel = &balancerPool
	case poolmanagertypes.Balancer:
		var balancerPool balancer.Pool
		err := json.Unmarshal(serializedPool.ChainPool, &balancerPool)
		if err != nil {
			return nil, err
		}
		chainModel = &balancerPool
	default:
		return nil, domain.InvalidPoolTypeError{PoolType: int32(serializedPool.Type)}
	}

	poolWrapper := sqsdomain.PoolWrapper{
		ChainModel: chainModel,
		SQSModel:   serializedPool.SQSModel,
		TickModel:  serializedPool.TickModel,
	}

	return &poolWrapper, nil
}
