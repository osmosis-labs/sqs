package domain_test

import (
	"fmt"
	"testing"

	"github.com/osmosis-labs/sqs/domain"
)

// Note: test cases are code-generated as sanity checks. If extension is needed,
// might make sense to revamp them completely.
func TestValidateDynamicMinLiquidityCapDesc(t *testing.T) {
	tests := []struct {
		name    string
		values  []domain.DynamicMinLiquidityCapFilterEntry
		wantErr error
	}{
		{
			name:    "empty slice",
			values:  []domain.DynamicMinLiquidityCapFilterEntry{},
			wantErr: nil,
		},
		{
			name: "valid descending order",
			values: []domain.DynamicMinLiquidityCapFilterEntry{
				{MinTokensCap: 500, FilterValue: 50},
				{MinTokensCap: 300, FilterValue: 30},
				{MinTokensCap: 100, FilterValue: 10},
			},
			wantErr: nil,
		},
		{
			name: "invalid min_tokens_cap order",
			values: []domain.DynamicMinLiquidityCapFilterEntry{
				{MinTokensCap: 300, FilterValue: 30},
				{MinTokensCap: 500, FilterValue: 50}, // out of order
				{MinTokensCap: 100, FilterValue: 10},
			},
			wantErr: fmt.Errorf("min_tokens_cap must be in descending order"),
		},
		{
			name: "invalid filter_value order",
			values: []domain.DynamicMinLiquidityCapFilterEntry{
				{MinTokensCap: 500, FilterValue: 50},
				{MinTokensCap: 300, FilterValue: 60}, // out of order
				{MinTokensCap: 100, FilterValue: 10},
			},
			wantErr: fmt.Errorf("filter_value must be in descending order"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := domain.ValidateDynamicMinLiquidityCapDesc(tt.values)

			if (err != nil && tt.wantErr == nil) || (err == nil && tt.wantErr != nil) || (err != nil && tt.wantErr != nil && err.Error() != tt.wantErr.Error()) {
				t.Errorf("validateDynamicMinLiquidityCapDesc() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
