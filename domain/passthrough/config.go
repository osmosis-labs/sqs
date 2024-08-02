package passthroughdomain

type PassthroughConfig struct {
	// The url of the Numia data source.
	NumiaURL string `mapstructure:"numia-url"`
	// The interval at which the APR data is fetched.
	APRFetchIntervalMinutes int `mapstructure:"apr-fetch-interval-minutes"`
}
