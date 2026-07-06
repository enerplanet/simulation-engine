package pypsa

// Request holds the request data
type Request struct {
	UserID       interface{} `json:"user_id" yaml:"user_id"`
	ModelID      interface{} `json:"model_id" yaml:"model_id"`
	SessionID    interface{} `json:"session_id" yaml:"session_id"`
	Lkr          string      `json:"lkr" yaml:"lkr"`
	CallbackURL  string      `json:"callback_url" yaml:"callback_url"`
	CustomDemand interface{} `json:"custom_demand_time_series" yaml:"custom_demand_time_series"`
	PyPSA        interface{} `json:"pypsa" yaml:"pypsa"`
	Topology     Topology    `json:"topology" yaml:"topology"`
}

// TechClasses holds all classes with their techs
type TechClasses struct {
	ZD          TechClass `json:"zd" yaml:"zd"`
	SFH         TechClass `json:"sfh" yaml:"sfh"`
	TH          TechClass `json:"th" yaml:"th"`
	MFH         TechClass `json:"mfh" yaml:"mfh"`
	AB          TechClass `json:"ab" yaml:"ab"`
	Commercial  TechClass `json:"commercial" yaml:"commercial"`
	Public      TechClass `json:"public" yaml:"public"`
	Industrial  TechClass `json:"industrial" yaml:"industrial"`
	Agricultural TechClass `json:"agricultural" yaml:"agricultural"`
}

// TechClass holds the data of techs for a class
type TechClass struct {
	PVSupply               bool `json:"pv_supply" yaml:"pv_supply"`
	IndustryPVSupply       bool `json:"industry_pv_supply" yaml:"industry_pv_supply"`
	BatteryStorage         bool `json:"battery_storage" yaml:"battery_storage"`
	IndustryBatteryStorage bool `json:"industry_battery_storage" yaml:"industry_battery_storage"`
	WindOnshore            bool `json:"wind_onshore" yaml:"wind_onshore"`
	BiomassSupply          bool `json:"biomass_supply" yaml:"biomass_supply"`
	geothermalSupply       bool `json:"geothermal_supply" yaml:"geothermal_supply"`
}
