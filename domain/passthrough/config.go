package passthroughdomain

type PassthroughConfig struct {
	// The url of the Numia data source.
	NumiaURL string `mapstructure:"numia-url"`
	// The url of the timeseries data stack.
	TimeseriesURL string `mapstructure:"timeseries-url"`
	// The interval at which the APR data is fetched.
	APRFetchIntervalMinutes int `mapstructure:"apr-fetch-interval-minutes"`
	// The interval at which the pool fees data is fetched.
	PoolFeesFetchIntervalMinutes int `mapstructure:"pool-fees-fetch-interval-minutes"`
}
