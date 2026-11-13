package domain

import (
	"fmt"
	"slices"
	"sync"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
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
type TransmuterStatuses struct {
	mu       sync.RWMutex
	statuses map[uint64]transmuterStatus
}

func NewTransmuterStatuses() *TransmuterStatuses {
	return &TransmuterStatuses{statuses: make(map[uint64]transmuterStatus)}
}

// SetActive records the is_active value reported by the pool's contract.
func (s *TransmuterStatuses) SetActive(poolID uint64, active bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.statuses[poolID]
	status.inactive = !active
	s.statuses[poolID] = status
}

// SetCorruptedDenoms records the get_corrupted_denoms value reported by the pool's contract.
func (s *TransmuterStatuses) SetCorruptedDenoms(poolID uint64, denoms []string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.statuses[poolID]
	status.corruptedDenoms = slices.Clone(denoms)
	s.statuses[poolID] = status
}

// IsInactive returns true if the pool's contract reported that it is paused.
func (s *TransmuterStatuses) IsInactive(poolID uint64) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.statuses[poolID].inactive
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
//     corrupted asset that still holds a balance other than the token out. So it is allowed only when the
//     token out is corrupted and either no other corrupted asset holds a balance, or the swap withdraws the
//     token out's entire pool balance (the contract's forced redemption of a corrupted asset, which skips
//     the check; it compares the raw amount against the pool asset's raw amount).
func (s *TransmuterStatuses) CheckSwap(poolID uint64, tokenInDenom, tokenOutDenom, alloyedDenom string, tokenOutAmount math.Int, balances sdk.Coins) error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	status := s.statuses[poolID]
	s.mu.RUnlock()

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

	if !slices.Contains(status.corruptedDenoms, tokenOutDenom) {
		return TransmuterCorruptedDenomError{PoolID: poolID, Denom: status.corruptedDenoms[0]}
	}

	// A full withdrawal of the corrupted token out is the contract's forced redemption.
	if !tokenOutAmount.IsNil() && tokenOutAmount.Equal(balances.AmountOf(tokenOutDenom)) {
		return nil
	}

	for _, denom := range status.corruptedDenoms {
		if denom != tokenOutDenom && balances.AmountOf(denom).IsPositive() {
			return TransmuterCorruptedDenomError{PoolID: poolID, Denom: denom}
		}
	}

	return nil
}
