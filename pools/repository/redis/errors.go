package redis

import (
	"fmt"

	poolmanagertypes "github.com/osmosis-labs/osmosis/v20/x/poolmanager/types"
)

type InvalidPoolTypeError struct {
	PoolType int32
}

func (e InvalidPoolTypeError) Error() string {
	return fmt.Sprintf("invalid pool type %s", poolmanagertypes.PoolType_name[e.PoolType])
}
