package mocks

import (
	"context"

	"cosmossdk.io/math"

	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/repository"
	routerredisrepo "github.com/osmosis-labs/sqs/sqsdomain/repository/redis/router"
)

type RedisRouterRepositoryMock struct {
	TakerFees sqsdomain.TakerFeeMap
	Routes    map[sqsdomain.DenomPair]sqsdomain.CandidateRoutes
}

// GetAllTakerFees implements domain.RouterRepository.
func (r *RedisRouterRepositoryMock) GetAllTakerFees(ctx context.Context) (sqsdomain.TakerFeeMap, error) {
	return r.TakerFees, nil
}

// GetRoutes implements domain.RouterRepository.
func (r *RedisRouterRepositoryMock) GetRoutes(ctx context.Context, denom0 string, denom1 string) (sqsdomain.CandidateRoutes, error) {
	// Ensure increasing lexicographic order.
	if denom1 < denom0 {
		denom0, denom1 = denom1, denom0
	}

	routes := r.Routes[sqsdomain.DenomPair{Denom0: denom0, Denom1: denom1}]
	return routes, nil
}

// GetTakerFee implements domain.RouterRepository.
func (r *RedisRouterRepositoryMock) GetTakerFee(ctx context.Context, denom0 string, denom1 string) (math.LegacyDec, error) {
	// Ensure increasing lexicographic order.
	if denom1 < denom0 {
		denom0, denom1 = denom1, denom0
	}

	return r.TakerFees[sqsdomain.DenomPair{Denom0: denom0, Denom1: denom1}], nil
}

// SetRoutes implements domain.RouterRepository.
func (r *RedisRouterRepositoryMock) SetRoutes(ctx context.Context, denom0 string, denom1 string, routes sqsdomain.CandidateRoutes) error {
	// Ensure increasing lexicographic order.
	if denom1 < denom0 {
		denom0, denom1 = denom1, denom0
	}

	r.Routes[sqsdomain.DenomPair{Denom0: denom0, Denom1: denom1}] = routes
	return nil
}

// SetRoutesTx implements domain.RouterRepository.
func (r *RedisRouterRepositoryMock) SetRoutesTx(ctx context.Context, tx repository.Tx, denom0 string, denom1 string, routes sqsdomain.CandidateRoutes) error {
	// Ensure increasing lexicographic order.
	if denom1 < denom0 {
		denom0, denom1 = denom1, denom0
	}

	r.Routes[sqsdomain.DenomPair{Denom0: denom0, Denom1: denom1}] = routes
	return nil
}

// SetTakerFee implements domain.RouterRepository.
func (r *RedisRouterRepositoryMock) SetTakerFee(ctx context.Context, tx repository.Tx, denom0 string, denom1 string, takerFee math.LegacyDec) error {
	// Ensure increasing lexicographic order.
	if denom1 < denom0 {
		denom0, denom1 = denom1, denom0
	}

	r.TakerFees[sqsdomain.DenomPair{Denom0: denom0, Denom1: denom1}] = takerFee
	return nil
}

var _ routerredisrepo.RouterRepository = &RedisRouterRepositoryMock{}
