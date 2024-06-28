package routerrepo

import (
	"sort"
	"strconv"
	"sync"

	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

type candidateRouteDataHolder struct {
	candidateRouteData sync.Map

	sortedMinLiquidityCapValuesDesc []uint64
}

var _ mvc.CandidateRouteSearchDataHolder = &candidateRouteDataHolder{}

// NewCandidateRouteDataHolder creates a new candidate route data holder.
func NewCandidateRouteDataHolder(uniqueMinLiquidityCapValues map[uint64]struct{}) *candidateRouteDataHolder {
	// Sort the unique min liquidity cap values in descending order
	sortedMinLiquidityCapValuesDesc := make([]uint64, 0, len(uniqueMinLiquidityCapValues))
	for minLiquidityCap := range uniqueMinLiquidityCapValues {
		sortedMinLiquidityCapValuesDesc = append(sortedMinLiquidityCapValuesDesc, minLiquidityCap)
	}

	// Sort the unique min liquidity cap values in descending order
	sort.Slice(sortedMinLiquidityCapValuesDesc, func(i, j int) bool {
		return sortedMinLiquidityCapValuesDesc[i] > sortedMinLiquidityCapValuesDesc[j]
	})

	return &candidateRouteDataHolder{

		sortedMinLiquidityCapValuesDesc: sortedMinLiquidityCapValuesDesc,

		candidateRouteData: sync.Map{},
	}
}

// GetCandidateRouteSearchData implements mvc.CandidateRouteSearchDataHolder.
func (c *candidateRouteDataHolder) GetCandidateRouteSearchData() map[string][]sqsdomain.PoolI {
	panic("unimplemented")
}

// GetSortedPoolsByDenom implements mvc.CandidateRouteSearchDataHolder.
func (c *candidateRouteDataHolder) GetSortedPoolsByDenom(denom string, minLiquidityCap uint64) ([]sqsdomain.PoolI, bool) {
	candidateRouteData, ok := c.candidateRouteData.Load(denom)
	if !ok {
		return nil, false
	}

	minLiqudityCapDescPools, ok := candidateRouteData.([][]sqsdomain.PoolI)
	if !ok {
		return nil, false
	}

	if len(minLiqudityCapDescPools) == 0 {
		return nil, false
	}

	for i, minLiquidityCapValue := range c.sortedMinLiquidityCapValuesDesc {
		if minLiquidityCapValue == minLiquidityCap {
			return minLiqudityCapDescPools[i], true
		}
	}

	return nil, false
}

// SetCandidateRouteSearchData implements mvc.CandidateRouteSearchDataHolder.
func (c *candidateRouteDataHolder) SetCandidateRouteSearchData(candidateRouteSearchData map[string][]sqsdomain.PoolI) {

	for denom, allDenomPools := range candidateRouteSearchData {

		if len(allDenomPools) == 0 {
			continue
		}

		denomCandidateRouteData := make([][]sqsdomain.PoolI, len(c.sortedMinLiquidityCapValuesDesc))

		// From top liquidity to lowest
		for j, pool := range allDenomPools {

			poolLiquidityCap := pool.GetPoolLiquidityCap().Uint64()

			// From top liquidity cap to lowest
			for i := 0; i < len(c.sortedMinLiquidityCapValuesDesc); i++ {

				// Pre-allocate
				if j == 0 && i == 0 {
					denomCandidateRouteData[i] = make([]sqsdomain.PoolI, 0)
				}

				minLiquidityCap := c.sortedMinLiquidityCapValuesDesc[i]

				if poolLiquidityCap >= minLiquidityCap {
					denomCandidateRouteData[i] = append(denomCandidateRouteData[i], pool)
				}

				if poolLiquidityCap == minLiquidityCap {
					// Break and continue to next pool.
					break
				}
			}

			// Since pools are ordered by liquidity, we can early exit if a pool is observed to
			// have less than the min liquidity cap.
			if poolLiquidityCap < c.sortedMinLiquidityCapValuesDesc[len(c.sortedMinLiquidityCapValuesDesc)-1] {
				break
			}
		}

		c.candidateRouteData.Store(denom, denomCandidateRouteData)
	}
}

func formatPrefix(denom string, minLiquidityCap uint64) string {
	return denom + "_" + strconv.FormatUint(minLiquidityCap, 10)
}
