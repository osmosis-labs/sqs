package usecase

import (
	"github.com/osmosis-labs/sqs/domain/mvc"
)

type StateDumper struct {
	RUsecase mvc.RouterUsecase
	TUsecase mvc.TokensUsecase
}

func NewStateDumper(routerUsecase mvc.RouterUsecase, tokenUsecase mvc.TokensUsecase) *StateDumper {
	return &StateDumper{
		RUsecase: routerUsecase,
		TUsecase: tokenUsecase,
	}
}

func (s *StateDumper) DumpAll() error {
	if err := s.RUsecase.StoreRouterStateFiles(); err != nil {
		return err
	}

	if err := s.TUsecase.StoreTokensStateFiles(); err != nil {
		return err
	}

	return nil
}
