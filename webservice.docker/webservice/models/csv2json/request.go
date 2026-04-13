package csv2json

// Request holds the request data
type Request struct {
	ID string `json:"id" yaml:"id"`
}

// Response holds the response data
type Response struct {
	ID        string `json:"id" yaml:"id"`
	StartTime string `json:"start_time" yaml:"start_time"`
	DeltaTime int64  `json:"delta_time" yaml:"delta_time"`
	EndTime   string `json:"end_time" yaml:"end_time"`
	Values    Values `json:"values" yaml:"values"`
	Width     int64  `json:"width" yaml:"width"`
	Length    int64  `json:"length" yaml:"length"`
}

// Values holds the time series values
type Values map[string][]float64
