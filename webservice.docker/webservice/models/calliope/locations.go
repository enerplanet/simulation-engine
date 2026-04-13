package calliope

import "time"

// LocationsConfig holds the data for the locations.yaml config file
type LocationsConfig struct {
	Model     Model               `json:"model" yaml:"model"`
	Locations map[string]Location `json:"locations" yaml:"locations"`
	Links     map[string]Link     `json:"links" yaml:"links"`
}

// Location holds the data of a location
type Location struct {
	AvailableArea float64                `json:"available_area" yaml:"available_area"`
	Coordinates   Coordinates            `json:"coordinates" yaml:"coordinates"`
	Techs         map[string]interface{} `json:"techs" yaml:"techs"`
}

// Link hold the data of a link
type Link struct {
	Techs map[string]interface{} `json:"techs" yaml:"techs"`
}

// Coordinates holds the x and y coordinates
type Coordinates struct {
	X float64 `json:"x" yaml:"x"`
	Y float64 `json:"y" yaml:"y"`
}

// Model holds the config override properties
type Model struct {
	SubsetTime []time.Time `json:"subset_time" yaml:"subset_time"`
	Time       Time        `json:"time" yaml:"time"`
}

type Time struct {
	Function       string                 `json:"function" yaml:"function"`
	FunctionOption map[string]interface{} `json:"function_options" yaml:"function_options"`
}
