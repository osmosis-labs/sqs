package v1beta1

import "testing"

func TestPaginationRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request PaginationRequest
		wantErr error
	}{
		{
			name:    "Valid request",
			request: PaginationRequest{Page: 1, Limit: 10},
			wantErr: nil,
		},
		{
			name:    "Page is zero",
			request: PaginationRequest{Page: 0, Limit: 10},
			wantErr: ErrPageNotValid,
		},
		{
			name:    "Limit is zero",
			request: PaginationRequest{Page: 1, Limit: 0},
			wantErr: ErrLimitNotValid,
		},
		{
			name:    "Page exceeds maximum",
			request: PaginationRequest{Page: MaxPage + 1, Limit: 10},
			wantErr: ErrPageTooLarge,
		},
		{
			name:    "Limit exceeds maximum",
			request: PaginationRequest{Page: 1, Limit: MaxLimit + 1},
			wantErr: ErrLimitTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if err != tt.wantErr {
				t.Errorf("PaginationRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
