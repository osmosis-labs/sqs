package passthroughdomain

// PoolFee represents the fees data of a pool.
type PoolFee struct {
	PoolID         string  `json:"pool_id"`
	Volume24h      float64 `json:"volume_24h"`
	Volume7d       float64 `json:"volume_7d"`
	FeesSpent24h   float64 `json:"fees_spent_24h"`
	FeesSpent7d    float64 `json:"fees_spent_7d"`
	FeesPercentage string  `json:"fees_percentage"`
}

// PoolFees represents the fees data of the pools.
type PoolFees struct {
	LastUpdateAt int64     `json:"last_update_at"`
	Data         []PoolFee `json:"data"`
}
