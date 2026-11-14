package iwst

import "time"

// ContactWindow describes a scheduled satellite pass and its link capacity.
type ContactWindow struct {
	PassID        string `json:"pass_id"`
	SatelliteName string `json:"satellite_name"`
	StartTime     int64  `json:"start_time"`
	EndTime       int64  `json:"end_time"`
	BandwidthBps  int64  `json:"bandwidth_bps"`
	MaxBytes      int64  `json:"max_bytes"`
}

// PassLog records the outcome of a satellite contact window.
type PassLog struct {
	PassID           string `json:"pass_id"`
	SatelliteName    string `json:"satellite_name"`
	BytesTransferred int64  `json:"bytes_transferred"`
	BundlesDelivered int    `json:"bundles_delivered"`
	Interrupted      bool   `json:"interrupted"`
	CompletedAt      int64  `json:"completed_at"`
}

// GroundStationAdapter is the hardware abstraction layer for authorized satellite ground stations.
type GroundStationAdapter interface {
	Name() string
	LinkCapacityBps() int64
	IsPassActive(now time.Time) bool
	MaxBytesPerPass() int64
}
