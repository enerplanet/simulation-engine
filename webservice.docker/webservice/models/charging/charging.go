package charging

import "time"

type Request struct {
	ID               string            `json:"id"`
	ChargingStations []ChargingStation `json:"charging_stations"`
	StartDate        time.Time         `json:"start_date"`
	EndDate          time.Time         `json:"end_date"`
}

type ChargingStation struct {
	PoiID    string  `json:"poi_id"`
	Location string  `json:"location"`
	PowerKw  float64 `json:"power_kw"`
}

type Data struct {
	Category string
	Values   []float64
}

type Response struct {
	ID               string              `json:"id"`
	Warnings         []string            `json:"warnings"`
	ChargingProfiles map[string][]float64 `json:"charging_profiles"`
}
