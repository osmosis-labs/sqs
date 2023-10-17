package redis_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	domain "github.com/osmosis-labs/sqs/domain"
)

var poolRepo domain.PoolsRepository

func TestRedis(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Redis Suite")
}
