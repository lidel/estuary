package config

import "time"

// MinSize - minimum staging bucket size before it can be aggregated
// MaxSize - maximum staging bucket size before it can be aggregated
// AggregateInterval - interval to aggregate staging contents
type StagingBucket struct {
	Enabled                 bool          `json:"enabled"`
	MinSize                 int64         `json:"min_size"`
	MaxSize                 int64         `json:"max_size"`
	IndividualDealThreshold int64         `json:"individual_deal_threshold"`
	AggregateInterval       time.Duration `json:"aggregate_interval"`
}
