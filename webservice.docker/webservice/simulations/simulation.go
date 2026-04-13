package simulations

import (
	"net/http"
	"time"
)

const (
	jsTimeFormatString = "2006-01-02 15:04"
)

const (
	contTypeHeader  = "Content-Type"
	contTypeAppYAML = "application/yaml"
	contTypeAppJSON = "application/json"
	contTypeAppText = "application/text"
)

const (
	pathCalliope       = "/calliope"
	pathPyPSA          = "/pypsa"
	pathCSV2JSON       = "/csv2json"
	pathCharging       = "/charging"
	logFolderPath      = "logs"
	logFileCalliope    = "calliope.log"
	logFilePyPSA       = "pypsa.log"
	logFileCSV2JSON    = "csv2json.log"
	logFileCharging    = "charging.log"
	simFolderPath      = "sims"
	dataFolderPath     = "data"
	chargingFolderPath = "charging"
	scriptFolderPath   = "scripts"
	simFolderPrefix    = "sim_"
	simZipPostfix      = ".zip"
	dataCSVPostfix     = ".csv"
	cdFolderPath       = "cd"
	templFolder        = "templates"
	modelConfFolder    = "model_config"
	techFile           = "techs.yaml"
	//techConventionalFile = "techs_conventional.yaml"
	//techDemandFile       = "techs_demand.yaml"
	//techGeneralFile      = "techs_general_supply.yaml"
	//techRenewableFile    = "techs_renewable.yaml"
	//techStorageFile      = "techs_storage.yaml"
	//techTransmissionFile = "techs_transmission.yaml"
	modelFile     = "model.yaml"
	scenariosFile = "scenarios.yaml"
	locFile       = "locations.yaml"
	confFile      = "config.json"
	coefsFile     = "coefs.csv"
	durationsFile = "durations.csv"
	cmdCalliope   = "bash ./" + scriptFolderPath + "/calliope.sh"
	cmdPyPSA      = "bash ./" + scriptFolderPath + "/pypsa.sh"
)

const (
	pvSup       = "pv_supply"
	inPvSup     = "industry_pv_supply"
	batSto      = "battery_storage"
	inBatSto    = "industry_battery_storage"
	transfSup   = "transformer_supply"
	powerTrans  = "power_transmission"
	winSup      = "wind_onshore"
	biomassSup  = "biomass_supply"
)

const (
	templStrDemand           = "%s_demand"
	templStrDemandFile       = "file=%sB.csv:%d"
	templStrCustomDemandFile = "file=%s/%s/%s.csv:demand"
	templStrPVFile           = "file=pv_%g_%g.csv:capacity_factor"
	templStrOnshoreWindFile  = "file=wind_%g_%g.csv:capacity_factor"
	templStrBiomassFile      = "file=biomass_%g_%g.csv:capacity_factor"
	templStrGeothermalFile   = "file=geothermal_%g_%g.csv:capacity_factor"
	templStrConstraints      = "constraints"
	templStrCosts            = "costs"
	templStrMonetary         = "monetary"
	templStrResource         = "resource"
	templStrLink             = "%s,%s"
	templStrID               = "ID_%s"
	templStrYAML             = "%s"
	templStrJSON             = "%s"
	templStrLog              = "%s"
	templStrError            = "%s"
	templStrResponse         = "%s\n\n%s"
)

const (
	searchStrTrafo                    = "trafo"
	searchStrPv                       = "pv_"
	searchStronShoreWind              = "wind_"
	searchStrBiomass                  = "biomass_"
	searchStrGeothermalElectricity    = "geothermal_"
	searchStrConstraint               = "cont_"
	searchStrCost                     = "cost_"
	searchStrCD                       = "cd"
	searchStrZD                       = "zd"
	noAvArea                          = -2
	noDemand                          = "0"
)

const (
	readBodyErrMsg                  = "can't read body"
	readConfigDataErrMsg            = "can't read config data"
	readRequestDataErrMsg           = "can't read request data"
	readLocationsErrMsg             = "can't read locations"
	readConfigErrMsg                = "can't read config"
	readLogErrMsg                   = "can't read log file"
	createDirErrMsg                 = "can't create dir"
	createFileErrMsg                = "can't create file"
	sendZipErrMsg                   = "can't send zip file"
	zipFolderErrMsg                 = "can't zip folder"
	modelIDExistErrMsg              = "model ID already exists"
	modelIDNotExistErrMsg           = "model ID does not exist"
	IDNotExistErrMsg                = "ID does not exist"
	folderNotExistErrMsg            = "folder does not exist"
	zipNotExistErrMsg               = "zip does not exist"
	duplicateModelIDErrMsg          = "duplicate model ID"
	duplicateIDErrMsg               = "duplicate ID"
	executeCMDErrMsg                = "can't execute cmd"
	convertMapToJSONErrMsg          = "can't convert map to JSON"
	inconsistentTopologyErrMsg      = "inconsistent topology"
	validateSchemeErrMsg            = "can't validate JSON string"
	invalidJSONStringErrMsg         = "JSON string is not valid"
	tooLessRowsAndColumnsErrMsg     = "too less rows and columns"
	timeSeriesInconsistentErrMsg    = "time series is inconsistent"
	saveCustomDemandCSVFailedErrMsg = "failed to save custom demand csv files"
	marshalingJSONErrMsg            = "error marshaling Wind, biomass or PV Simulation request body"
	sendPvSimReqMsg                 = "sending pv simulation request"
	sendPvSimReqSuccessMsg          = "pV simulation request successful"
	parsePvSimResErrmsg             = "error parsing response from PV simulation"
	sendWindSimulationRequestion    = "sending wind simulation request"
	SendWindSimulationSuccess       = "sending wind simulation success"
	SendWindSimulationFailed        = "sending wind simulation failed"
	sendBiomassSimulationRequestion = "sending biomass simulation request"
	SendBiomassSimulationSuccess    = "sending biomass simulation success"
	SendBiomassSimulationFailed     = "sending biomass simulation failed"
	sendHydrogenSimulationRequest   = "sending hydrogen simulation request"
	SendHydrogenSimulationSuccess   = "sending hydrogen simulation success"
	SendHydrogenSimulationFailed    = "sending hydrogen simulation failed"
	createHttpReqErrMsg             = "error creating http request"
)

// simLogCalliope contains information about a single simulation
type simLogCalliope struct {
	ModelID   string
	SessionID string
	PyPSA     bool
	Start     time.Time
	Duration  time.Duration
}

// simLogPyPSA contains information about a single simulation
type simLogPyPSA struct {
	ModelID   string
	SessionID string
	PyPSA     bool
	Start     time.Time
	Duration  time.Duration
}

// simLogCSV2JSON contains information about a single simulation
type simLogCSV2JSON struct {
	ID       string
	Start    time.Time
	Duration time.Duration
}

// simLogChargingcontains information about a single simulation
type simLogCharging struct {
	ID       string
	Start    time.Time
	Duration time.Duration
}

// Simulation is a interface to handle all simulations
type Simulation interface {
	Path() string
	Configure() http.HandlerFunc
	Generate() http.HandlerFunc
	Start() http.HandlerFunc
	Show() http.HandlerFunc
	Log() http.HandlerFunc
	Finish() http.HandlerFunc
}
