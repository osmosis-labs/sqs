package domain

import (
	"fmt"
	"slices"
	"sync"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	sqsatomic "github.com/osmosis-labs/sqs/domain/sync/atomic"
)

// TransmuterInactiveError is returned when a swap goes through a transmuter that is paused.
type TransmuterInactiveError struct {
	PoolID uint64
}

func (e TransmuterInactiveError) Error() string {
	return fmt.Sprintf("transmuter pool %d is not active", e.PoolID)
}

// TransmuterCorruptedDenomError is returned when a swap would increase a corrupted asset of a transmuter,
// which the contract rejects.
type TransmuterCorruptedDenomError struct {
	PoolID uint64
	Denom  string
}

func (e TransmuterCorruptedDenomError) Error() string {
	return fmt.Sprintf("transmuter pool %d does not allow swaps that increase corrupted denom %s", e.PoolID, e.Denom)
}

// transmuterStatus is what is known about one transmuter. The zero value is "active, nothing corrupted",
// which is also what an unknown pool is treated as.
type transmuterStatus struct {
	inactive        bool
	corruptedDenoms []string
}

// TransmuterStatuses holds the active flag and corrupted denoms that the contracts report, keyed by pool ID.
// A pool without an entry, or with a value that failed to load, is routed as active with no corrupted denoms:
// the chain still rejects a swap that is not allowed, and excluded-pool-ids stays the operator override.
// All methods are safe for concurrent use and safe to call on a nil receiver.
//
// Reads are lock-free: candidate route search and every transmuter quote read this on their hot path,
// many goroutines at once, while writes happen only when a status query returns.
type TransmuterStatuses struct {
	// writeMu serialises the read-modify-write of one entry, so that SetActive and SetCorruptedDenoms
	// on the same pool cannot drop each other's update.
	writeMu  sync.Mutex
	statuses sqsatomic.Map[uint64, transmuterStatus]
}

func NewTransmuterStatuses() *TransmuterStatuses {
	return &TransmuterStatuses{}
}

// SetActive records the is_active value reported by the pool's contract.
func (s *TransmuterStatuses) SetActive(poolID uint64, active bool) {
	if s == nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	status, _ := s.statuses.Get(poolID)
	status.inactive = !active
	s.statuses.Set(poolID, status)
}

// SetCorruptedDenoms records the get_corrupted_denoms value reported by the pool's contract.
func (s *TransmuterStatuses) SetCorruptedDenoms(poolID uint64, denoms []string) {
	if s == nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	status, _ := s.statuses.Get(poolID)
	status.corruptedDenoms = slices.Clone(denoms)
	s.statuses.Set(poolID, status)
}

// IsInactive returns true if the pool's contract reported that it is paused.
func (s *TransmuterStatuses) IsInactive(poolID uint64) bool {
	if s == nil {
		return false
	}
	status, _ := s.statuses.Get(poolID)
	return status.inactive
}

// CheckSwap returns an error if the transmuter does not allow swapping tokenInDenom for tokenOutDenom.
// alloyedDenom is the alloyed asset of an alloyed transmuter, empty for a v1 transmuter.
//
// tokenOutAmount is the raw token out amount and balances the pool's raw balances. Both are used only when
// alloyedDenom is set.
//
// The contract never lets a corrupted asset increase in amount or weight, which means:
//   - a corrupted denom cannot be swapped in;
//   - swapping healthy denoms for each other, or a healthy denom for the alloyed asset, never raises a
//     corrupted asset's weight, so it is allowed;
//   - an alloyed asset swapped in burns shares and removes the token out, which raises the weight of every
//     corrupted asset that still holds a balance other than the token out. So it is rejected if and only if
//     some corrupted denom other than the token out has a positive balance. A corrupted asset with a zero
//     balance has weight zero before and after, so it does not block the swap; this matters because balances
//     are updated synchronously by ingest while the corrupted list is refreshed asynchronously, so the list
//     can still name a denom that has already been drained. The exception is the contract's forced
//     redemption of a corrupted token out, which skips the check: a swap that withdraws the token out's
//     entire pool balance (raw amount against the pool asset's raw amount) is always allowed. That exception
//     never applies to a healthy token out.
func (s *TransmuterStatuses) CheckSwap(poolID uint64, tokenInDenom, tokenOutDenom, alloyedDenom string, tokenOutAmount math.Int, balances sdk.Coins) error {
	if s == nil {
		return nil
	}
	status, _ := s.statuses.Get(poolID)

	if status.inactive {
		return TransmuterInactiveError{PoolID: poolID}
	}

	if len(status.corruptedDenoms) == 0 {
		return nil
	}

	if slices.Contains(status.corruptedDenoms, tokenInDenom) {
		return TransmuterCorruptedDenomError{PoolID: poolID, Denom: tokenInDenom}
	}

	if alloyedDenom == "" || tokenInDenom != alloyedDenom {
		return nil
	}

	tokenOutCorrupted := slices.Contains(status.corruptedDenoms, tokenOutDenom)

	// A full withdrawal of the corrupted token out is the contract's forced redemption.
	if tokenOutCorrupted && !tokenOutAmount.IsNil() && tokenOutAmount.Equal(balances.AmountOf(tokenOutDenom)) {
		return nil
	}

	for _, denom := range status.corruptedDenoms {
		if denom != tokenOutDenom && balances.AmountOf(denom).IsPositive() {
			return TransmuterCorruptedDenomError{PoolID: poolID, Denom: denom}
		}
	}

	return nil
}
