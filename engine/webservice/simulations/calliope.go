package simulations

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"webservice/config"
	"webservice/conversion"
	models "webservice/models/calliope"
	"webservice/utils"
	"webservice/validator"

	"gopkg.in/yaml.v3"
)

// Semaphore for limiting concurrent simulations
var simSemaphore chan struct{}
var simSemaphoreOnce sync.Once

func getSimSemaphore() chan struct{} {
	simSemaphoreOnce.Do(func() {
		cfg := config.GetConfig()
		simSemaphore = make(chan struct{}, cfg.MaxConcurrentSims)
	})
	return simSemaphore
}

// Calliope YAMLbyte
type Calliope struct {
	mu                sync.Mutex
	activeSimulations map[string]simLogCalliope
}

// NewCalliope creates a new Calliope struct
func NewCalliope() *Calliope {
	return &Calliope{activeSimulations: make(map[string]simLogCalliope)}
}

func (c *Calliope) isModelIDDuplicate(modelID string) bool {
	// Caller must hold c.mu
	for _, l := range c.activeSimulations {
		if l.ModelID == modelID {
			return true
		}
	}

	return false
}

func (c *Calliope) newSimLog(modelID string, sessionID string, pyPSA bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isModelIDDuplicate(modelID) {
		return errors.New(duplicateModelIDErrMsg)
	}

	c.activeSimulations[modelID] = simLogCalliope{
		ModelID:   modelID,
		SessionID: sessionID,
		PyPSA:     pyPSA,
		Start:     time.Now(),
		Duration:  time.Duration(0),
	}

	return nil
}

func (c *Calliope) closeSimLog(modelID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	simLog, ok := c.activeSimulations[modelID]
	if !ok {
		return errors.New("Calliope | " + modelIDNotExistErrMsg)
	}

	simLog.Duration = time.Since(simLog.Start)
	c.activeSimulations[modelID] = simLog

	return nil
}

func (c *Calliope) removeSimLog(modelID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, ok := c.activeSimulations[modelID]
	if !ok {
		return errors.New("Calliope | " + modelIDNotExistErrMsg)
	}

	delete(c.activeSimulations, modelID)

	return nil
}

func (c *Calliope) showLogs() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, l := range c.activeSimulations {
		fmt.Printf("Calliope | MID: %s, SID %s, T %s, D %d \n", l.ModelID, l.SessionID, l.Start.String(), l.Duration.Milliseconds())
	}
}

// Path returns the path
func (c *Calliope) Path() string {
	return pathCalliope
}

// Configure prepares the simulation
func (c *Calliope) Configure() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}

// Generate creates a simulation
func (c *Calliope) Generate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}

// Start runs the simulation
func (c *Calliope) Start() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Acquire semaphore for concurrent simulation limit
		sem := getSimSemaphore()
		slotHeld := false
		select {
		case sem <- struct{}{}:
			slotHeld = true
			// Acquired slot — will be released by ExecCMDWithSemaphore when command finishes
		default:
			w.Header().Set(contTypeHeader, contTypeAppJSON)
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":"capacity","code":"SIM_AT_CAPACITY","retry_after_seconds":30,"message":"Server at capacity. Maximum concurrent simulations reached. Please try again later."}`)
			return
		}
		defer func() {
			if slotHeld {
				<-sem
			}
		}()

		body, err := ioutil.ReadAll(r.Body)

		if err != nil {
			utils.Log(err)
			http.Error(w, readBodyErrMsg, http.StatusBadRequest)
			return
		}

		validateResults, err := validator.ValidateJSON(string(body), pathCalliope)
		if err != nil {
			utils.Log(err)
			http.Error(w, validateSchemeErrMsg, http.StatusBadRequest)
			return
		}

		schemaValidateString := validator.PrintFormatResults(validateResults)

		if !validator.IsValid(validateResults) {
			utils.Log(errors.New(invalidJSONStringErrMsg))
			http.Error(w, schemaValidateString, http.StatusBadRequest)
			return
		}

		var config map[string]interface{}

		if err := json.Unmarshal(body, &config); err != nil {
			utils.Log(err)
			http.Error(w, readConfigDataErrMsg, http.StatusBadRequest)
			return
		}

		var request models.Request

		if err := json.Unmarshal(body, &request); err != nil {
			utils.Log(err)
			http.Error(w, readRequestDataErrMsg, http.StatusBadRequest)
			return
		}

		if strings.TrimSpace(request.Country) == "" {
			utils.Log(errors.New("country is required"))
			http.Error(w, "country is required", http.StatusBadRequest)
			return
		}

		if !isTopologyConsistent(request.Topology) {
			utils.Log(fmt.Errorf(templStrError, inconsistentTopologyErrMsg))
			http.Error(w, inconsistentTopologyErrMsg, http.StatusBadRequest)
			return
		}

		if err := saveCustomDemandCSV(request); err != nil {
			utils.Log(err)
			http.Error(w, saveCustomDemandCSVFailedErrMsg, http.StatusBadRequest)
			return
		}

		modelIDStr := utils.ToStr(request.ModelID)
		sessionIDStr := utils.ToStr(request.SessionID)

		// Check if PyPSA simulation is enabled
		pypsaEnabled := true
		if request.PyPSA == false {
			pypsaEnabled = false
		}

		if err := c.newSimLog(modelIDStr, sessionIDStr, pypsaEnabled); err != nil {
			utils.Log(err)
			http.Error(w, modelIDExistErrMsg, http.StatusBadRequest)
			return
		}

		c.showLogs()

		var locations models.LocationsConfig
		locations.Model.SubsetTime = getSubsetTime(request)
		locations.Model.Time = getResolution(request)
		locations.Locations = getLocations(request)
		locations.Links = getLinks(request)

		cleanAvailableArea(&locations)

		byteYAML, err := yaml.Marshal(&locations)
		if err != nil {
			utils.Log(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		byteJSON, err := json.MarshalIndent(&config, "", "\t")
		if err != nil {
			utils.Log(err)
			http.Error(w, readConfigErrMsg, http.StatusBadRequest)
			return
		}

		stringYAML := utils.CleanYAML(string(byteYAML))
		stringJSON := string(byteJSON)

		userID := utils.ToStr(request.UserID)
		sessionID := sessionIDStr
		modelID := modelIDStr
		lkr := request.Lkr
		callbackURL := request.CallbackURL

		folder := simFolderPath + "/" + simFolderPrefix + modelID

		if err := utils.CreateDir(folder); err != nil {
			utils.Log(err)
			http.Error(w, createDirErrMsg, http.StatusBadRequest)
			return
		}

		copyCalliopeTemplate(folder, modelID, request)

		// BuEM enrichment and renewables tech dispatch now happen upstream, in
		// the Orchestrator (see orchestrator.go), before this handler is invoked.
		// Calliope only builds and runs its own model here.

		if err := utils.CreateFile(folder+"/"+confFile, stringJSON); err != nil {
			utils.Log(err)
			http.Error(w, createFileErrMsg, http.StatusBadRequest)
			return
		}

		if err := utils.CreateFile(folder+"/"+modelConfFolder+"/"+locFile, stringYAML); err != nil {
			utils.Log(err)
			http.Error(w, createFileErrMsg, http.StatusBadRequest)
			return
		}

		// Pass pypsa flag to the shell script
		pypsaFlag := "true"
		if !pypsaEnabled {
			pypsaFlag = "false"
		}
		// Pass arguments directly as argv — NOT via "bash -c <string>" — so
		// values like lkr/callbackURL can't shell-escape. calliope.sh receives
		// $1..$7 either way, so the script itself does not need to change.
		calliopeArgs := []string{
			cmdCalliopeScript,
			userID,
			modelID,
			sessionID,
			lkr,
			callbackURL,
			pypsaFlag,
		}
		if err := utils.ExecArgvWithSemaphore(cmdCalliopeBin, calliopeArgs, sem); err != nil {
			utils.Log(err)
			http.Error(w, executeCMDErrMsg, http.StatusBadRequest)
			return
		}
		slotHeld = false

		w.Header().Set(contTypeHeader, contTypeAppYAML)

		fmt.Fprintf(w, templStrResponse, schemaValidateString, stringYAML)
	}
}

func isTopologyConsistent(topology models.Topology) bool {
	seen := make(map[string]models.Feature, len(topology)*2)

	for _, conn := range topology {
		for _, feature := range []models.Feature{conn.From, conn.To} {
			// Check custom demand has dimensions
			if strings.ToLower(feature.Properties.SLPClass) == searchStrCD &&
				feature.CustomDemand.Width == 0 &&
				feature.CustomDemand.Length == 0 {
				return false
			}

			// Check consistency: same ID must have same properties
			if prev, exists := seen[feature.ID]; exists {
				if !feature.Compare(prev) {
					return false
				}
			} else {
				seen[feature.ID] = feature
			}
		}
	}

	return true
}

func saveCustomDemandCSV(request models.Request) error {
	modelID := utils.ToStr(request.ModelID)

	for _, conn := range request.Topology {
		featureIDFrom := conn.From.ID
		featureIDTo := conn.To.ID
		customDemandFrom := conn.From.CustomDemand
		customDemandTo := conn.To.CustomDemand

		folder := dataFolderPath + "/" + cdFolderPath + "/" + modelID
		if err := utils.CreateDir(folder); err != nil {
			return err
		}

		if customDemandFrom.Length != 0 || customDemandFrom.Width != 0 {
			customDemandFromCSV, err := conversion.JSON2CSV(customDemandFrom)
			if err != nil {
				return err
			}

			file := folder + "/" + fmt.Sprintf("%s", featureIDFrom) + dataCSVPostfix
			if _, err := os.Stat(file); os.IsNotExist(err) {
				if err := utils.CreateFile(file, customDemandFromCSV); err != nil {
					return err
				}
			}
		}

		if customDemandTo.Length != 0 || customDemandTo.Width != 0 {
			customDemandToCSV, err := conversion.JSON2CSV(customDemandTo)
			if err != nil {
				return err
			}

			file := folder + "/" + fmt.Sprintf("%s", featureIDTo) + dataCSVPostfix
			if _, err := os.Stat(file); os.IsNotExist(err) {
				if err := utils.CreateFile(file, customDemandToCSV); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func copyCalliopeTemplate(dir string, modelID string, request models.Request) {
	utils.CreateDir(dir + "/" + modelConfFolder)
	utils.CopyTemplateFile(techFile, templFolder, dir+"/"+modelConfFolder)
	utils.CopyTemplateFile(modelFile, templFolder, dir)
	utils.CopyTemplateFile(scenariosFile, templFolder, dir)

	// Update timeseries_data_path to model-specific directory for concurrent output isolation
	modelYamlPath := dir + "/" + modelFile
	content, err := os.ReadFile(modelYamlPath)
	if err != nil {
		log.Printf("WARNING: could not read model.yaml for timeseries path update: %v\n", err)
		return
	}
	updated := strings.Replace(string(content), "timeseries_data_path: ../../data", "timeseries_data_path: ../../data/"+modelID, 1)

	// Choose solver method based on model size: IPM for large models, simplex for small
	timesteps := int(request.EndDate.Sub(request.StartDate).Minutes() / float64(request.Resolution))
	locations := len(request.Topology) * 2
	modelSize := timesteps * locations
	solverMethod := "simplex_strategy: 4"
	if modelSize > 50000 {
		solverMethod = "solver: ipm"
	}
	log.Printf("Model size: %d timesteps x %d locations = %d → solver method: %s\n", timesteps, locations, modelSize, solverMethod)
	updated = strings.Replace(updated, "SOLVER_METHOD_PLACEHOLDER", solverMethod, 1)
	if err := os.WriteFile(modelYamlPath, []byte(updated), 0644); err != nil {
		log.Printf("WARNING: could not update timeseries_data_path in model.yaml: %v\n", err)
		return
	}

	// Create the model-specific data directory and symlink all standard data files into it
	modelDataDir := dataFolderPath + "/" + modelID
	if err := os.MkdirAll(modelDataDir, 0755); err != nil {
		log.Printf("WARNING: could not create model data dir %s: %v\n", modelDataDir, err)
		return
	}
	entries, err := os.ReadDir(dataFolderPath)
	if err != nil {
		log.Printf("WARNING: could not read data dir: %v\n", err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		src, _ := filepath.Abs(dataFolderPath + "/" + entry.Name())
		dst := modelDataDir + "/" + entry.Name()
		if _, err := os.Lstat(dst); err == nil {
			continue // already exists
		}
		if err := os.Symlink(src, dst); err != nil {
			log.Printf("WARNING: could not symlink %s -> %s: %v\n", src, dst, err)
		}
	}
}

func getSubsetTime(request models.Request) []time.Time {
	return []time.Time{
		request.StartDate,
		request.EndDate,
	}
}

func getResolution(request models.Request) models.Time {
	resolution := fmt.Sprintf("%dmin", request.Resolution)
	function := "resample"

	functionOptions := make(map[string]interface{})
	functionOptions["resolution"] = resolution

	return models.Time{
		Function:       function,
		FunctionOption: functionOptions,
	}
}

func getLocations(request models.Request) map[string]models.Location {
	locations := make(map[string]models.Location)

	for _, conn := range request.Topology {
		var locationID string
		locationID = getID(conn.From.ID, conn.From.Properties.OSMID)
		locations[locationID] = getLocation(conn.From, request)
		locationID = getID(conn.To.ID, conn.To.Properties.OSMID)
		locations[locationID] = getLocation(conn.To, request)
	}

	return locations
}

func getLocation(feature models.Feature, request models.Request) models.Location {
	return models.Location{
		AvailableArea: feature.Properties.Area,
		Coordinates: models.Coordinates{
			X: feature.Geometry.Coordinates[0],
			Y: feature.Geometry.Coordinates[1],
		},
		Techs: getTechs(feature, request),
	}
}

func getTechs(feature models.Feature, request models.Request) map[string]interface{} {
	lkr := request.Lkr
	coords := feature.Geometry.Coordinates
	modelID := utils.ToStr(request.ModelID)
	featureID := feature.ID
	properties := feature.Properties
	activeTechs := feature.Techs
	techs := make(map[string]interface{})

	if isTrafo(properties.OSMID) {
		addTrafoSuppy(techs, properties.RatedPower)
		return techs
	}

	techClass, profileClass := resolveDemandClasses(properties)

	if isCustomDemand(techClass) {
		addCustomDemand(techs, techClass, modelID, featureID)
	} else if isZeroDemand(techClass) {
		addZeroDemand(techs, techClass)
	} else {
		addStdDemand(techs, techClass, profileClass, properties.DemandEnergy)
	}

	for techID, contAndCosts := range activeTechs {
		addTech(techs, techID, contAndCosts, lkr, coords)
	}

	return techs
}

// normalizeYAMLValue converts string "inf"/".inf" to math.Inf so yaml.Marshal
// outputs `.inf` which Python/Calliope correctly parses as float('inf').
func normalizeYAMLValue(v interface{}) interface{} {
	if s, ok := v.(string); ok {
		lower := strings.ToLower(strings.TrimSpace(s))
		if lower == "inf" || lower == ".inf" || lower == "+inf" || lower == "+.inf" {
			return math.Inf(1)
		}
		if lower == "-inf" || lower == "-.inf" {
			return math.Inf(-1)
		}
	}
	return v
}

func addTech(techs map[string]interface{}, techID string, contAndCosts models.ContAndCosts, lkr string, coords []float64) {
	constraints := make(map[string]interface{})
	costs := make(map[string]interface{})
	tech := make(map[string]interface{})

	for contAndCost, value := range contAndCosts {
		if value == nil {
			continue
		}
		if isConstraint(contAndCost) {
			contID := contAndCost[5:]
			constraints[contID] = normalizeYAMLValue(value)
		}
		if isCost(contAndCost) {
			costID := contAndCost[5:]
			costs[costID] = normalizeYAMLValue(value)
		}
	}

	if isPv(techID) {
		resource := fmt.Sprintf(templStrPVFile, coords[1], coords[0])
		constraints[templStrResource] = resource
	}

	if len(constraints) != 0 {
		tech[templStrConstraints] = constraints
	}

	if len(costs) != 0 {
		tech[templStrCosts] = map[string]interface{}{
			templStrMonetary: costs,
		}
	}

	if len(tech) == 0 {
		tech = nil
	}

	techs[techID] = tech
}

func isConstraint(key string) bool {
	return strings.Contains(strings.ToLower(key), searchStrConstraint)
}

func isCost(key string) bool {
	return strings.Contains(strings.ToLower(key), searchStrCost)
}

func isPv(key string) bool {
	return strings.Contains(strings.ToLower(key), searchStrPv)
}

func isCustomDemand(class string) bool {
	return strings.Contains(strings.ToLower(class), searchStrCD)
}

func isZeroDemand(class string) bool {
	return strings.Contains(strings.ToLower(class), searchStrZD)
}

func normalizeDemandToken(class string) string {
	v := strings.ToLower(strings.TrimSpace(class))
	v = strings.ReplaceAll(v, "-", "_")
	v = strings.ReplaceAll(v, " ", "_")
	for strings.Contains(v, "__") {
		v = strings.ReplaceAll(v, "__", "_")
	}
	return strings.Trim(v, "_")
}

func normalizeDemandTechClass(class string) string {
	c := normalizeDemandToken(class)
	switch c {
	case "sfh", "th", "mfh", "ab", "commercial", "public", "industrial", "agricultural", "cd", "zd":
		return c
	}

	switch {
	case strings.Contains(c, "apartment"), strings.Contains(c, "dormitory"), c == "apartments", c == "apartment":
		return "mfh"
	case strings.Contains(c, "terrace"), strings.Contains(c, "townhouse"), c == "semidetached_house":
		return "th"
	case strings.Contains(c, "house"), c == "detached", c == "residential", c == "bungalow", c == "farmhouse", c == "hut", c == "cabin":
		return "sfh"
	case strings.Contains(c, "farm"), strings.Contains(c, "agricultur"), c == "barn", c == "cowshed", c == "stable", c == "greenhouse":
		return "agricultural"
	case strings.Contains(c, "factory"), strings.Contains(c, "industrial"), strings.Contains(c, "warehouse"),
		strings.Contains(c, "workshop"), strings.Contains(c, "manufacture"), strings.Contains(c, "station"),
		strings.Contains(c, "power"), strings.Contains(c, "utility"), strings.Contains(c, "substation"),
		strings.Contains(c, "data_center"):
		return "industrial"
	case strings.Contains(c, "school"), strings.Contains(c, "kindergarten"), strings.Contains(c, "hospital"),
		strings.Contains(c, "university"), strings.Contains(c, "church"), strings.Contains(c, "museum"),
		strings.Contains(c, "theatre"), strings.Contains(c, "government"), strings.Contains(c, "library"),
		strings.Contains(c, "public"), strings.Contains(c, "civic"), strings.Contains(c, "community"):
		return "public"
	default:
		return "commercial"
	}
}

func resolveDemandClasses(properties models.Properties) (string, string) {
	slpClass := normalizeDemandToken(properties.SLPClass)
	demandProfile := normalizeDemandToken(properties.DemandProfile)
	fClass := normalizeDemandToken(properties.FClass)

	// Keep explicit custom/zero demand markers untouched.
	if slpClass != "" && isCustomDemand(slpClass) {
		return "cd", "cd"
	}
	if slpClass != "" && isZeroDemand(slpClass) {
		return "zd", "zd"
	}

	if slpClass != "" {
		techClass := normalizeDemandTechClass(slpClass)
		if demandProfile == "" {
			demandProfile = slpClass
		}
		return techClass, demandProfile
	}

	if demandProfile != "" {
		return normalizeDemandTechClass(demandProfile), demandProfile
	}

	if fClass != "" {
		return normalizeDemandTechClass(fClass), fClass
	}

	return "commercial", "commercial"
}

// isBuildingType checks if the class is a building type (pylovo classification)
func isBuildingType(class string) bool {
	buildingTypes := map[string]bool{
		"sfh": true, "th": true, "mfh": true, "ab": true,
		"commercial": true, "public": true, "industrial": true, "agricultural": true,
	}
	return buildingTypes[strings.ToLower(class)]
}

// getBuildingTypeCSVName returns the CSV file name for a building type
// Building types use direct names (SFH.csv), legacy SLP uses suffix (H1B.csv)
func getBuildingTypeCSVName(class string) string {
	lowerClass := strings.ToLower(class)

	// Building type mapping to CSV names
	buildingTypeMap := map[string]string{
		"sfh":          "SFH",
		"th":           "TH",
		"mfh":          "MFH",
		"ab":           "AB",
		"commercial":   "Commercial",
		"public":       "Public",
		"industrial":   "Industrial",
		"agricultural": "Agricultural",
	}

	if csvName, ok := buildingTypeMap[lowerClass]; ok {
		return csvName
	}

	// Safe fallback to avoid invalid file references.
	return "Commercial"
}

func addStdDemand(techs map[string]interface{}, techClass string, profileClass string, demand int64) {
	normalizedTechClass := normalizeDemandTechClass(techClass)
	if normalizedTechClass == "" {
		normalizedTechClass = "commercial"
	}
	techID := fmt.Sprintf(templStrDemand, normalizedTechClass)

	profileKey := normalizeDemandToken(profileClass)
	if profileKey == "" {
		profileKey = normalizedTechClass
	}

	// Resolve lookup key via demand index.
	columnMapper := utils.GetColumnMapper()
	lookupKey := columnMapper.ResolveDemandLookupKey(profileKey)
	if !columnMapper.HasBuildingType(lookupKey) {
		if columnMapper.HasBuildingType(normalizedTechClass) {
			lookupKey = normalizedTechClass
		} else if columnMapper.HasBuildingType("commercial") {
			lookupKey = "commercial"
		}
	}

	nearestLevel := columnMapper.FindNearestDemandLevel(lookupKey, demand)

	// Log if we're using a different level than requested
	if nearestLevel != demand {
		log.Printf("Demand %d not found for profile %s (tech=%s), using nearest level: %d", demand, lookupKey, normalizedTechClass, nearestLevel)
	}

	fileName := columnMapper.GetFile(lookupKey)
	if fileName == "" {
		// Backward-compatible fallback when index is not loaded.
		fileName = fmt.Sprintf("%s.csv", getBuildingTypeCSVName(normalizedTechClass))
	}

	// Calliope 0.6.x CSV timeseries syntax: file=<filename>:<column>.
	resource := fmt.Sprintf("file=%s:%d", fileName, nearestLevel)

	techs[techID] = map[string]interface{}{
		templStrConstraints: map[string]interface{}{
			templStrResource: resource,
		},
	}
}

func addZeroDemand(techs map[string]interface{}, class string) {
	techID := fmt.Sprintf(templStrDemand, strings.ToLower(class))
	resource := noDemand

	techs[techID] = map[string]interface{}{
		templStrConstraints: map[string]interface{}{
			templStrResource: resource,
		},
	}
}

func addCustomDemand(techs map[string]interface{}, class string, modelID string, featureID string) {
	techID := fmt.Sprintf(templStrDemand, strings.ToLower(class))
	resource := fmt.Sprintf(templStrCustomDemandFile, strings.ToLower(class), modelID, featureID)

	techs[techID] = map[string]interface{}{
		templStrConstraints: map[string]interface{}{
			templStrResource: resource,
		},
	}
}

func addTrafoSuppy(techs map[string]interface{}, ratedPower float64) {
	techID := transfSup
	if ratedPower > 0 {
		techs[techID] = map[string]interface{}{
			templStrConstraints: map[string]interface{}{
				"energy_cap_max": ratedPower,
			},
		}
	} else {
		techs[techID] = nil
	}
}

func getLinks(request models.Request) map[string]models.Link {
	links := make(map[string]models.Link)

	for _, conn := range request.Topology {
		linkID := getLinkID(conn)
		link := links[linkID]
		link.Techs = map[string]interface{}{powerTrans: nil}
		links[linkID] = link
	}

	return links
}

func getLinkID(conn models.Connection) string {
	fromID := getID(conn.From.ID, conn.From.Properties.OSMID)
	toID := getID(conn.To.ID, conn.To.Properties.OSMID)

	return fmt.Sprintf(templStrLink, fromID, toID)
}

func getID(ID string, OSMID string) string {
	if isTrafo(OSMID) {
		return OSMID
	}

	return fmt.Sprintf(templStrID, ID)
}

func isTrafo(OSMID string) bool {
	return strings.Contains(strings.ToLower(OSMID), searchStrTrafo)
}

func cleanAvailableArea(locations *models.LocationsConfig) {
	locs := locations.Locations
	for key, loc := range locs {
		if hasPV(loc) {
			continue
		}
		locs[key] = models.Location{
			AvailableArea: noAvArea,
			Coordinates:   loc.Coordinates,
			Techs:         loc.Techs,
		}
	}
	locations.Locations = locs
}

func hasPV(location models.Location) bool {
	pvOK := false
	for techID := range location.Techs {
		if pvOK = isPv(techID); pvOK {
			break
		}
	}
	return pvOK
}

// Show gives information about the progress of the simulation
func (c *Calliope) Show() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		activeSimsJSON, err := json.Marshal(c.activeSimulations)
		c.mu.Unlock()
		if err != nil {
			utils.Log(err)
			http.Error(w, convertMapToJSONErrMsg, http.StatusBadRequest)
			return
		}

		w.Header().Set(contTypeHeader, contTypeAppJSON)

		fmt.Fprintf(w, templStrJSON, activeSimsJSON)
	}
}

// Log gives you the log file content
func (c *Calliope) Log() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		log, err := utils.ReadFile(logFolderPath + "/" + logFileCalliope)
		if err != nil {
			utils.Log(err)
			http.Error(w, readLogErrMsg, http.StatusBadRequest)
			return
		}

		w.Header().Set(contTypeHeader, contTypeAppText)

		fmt.Fprintf(w, templStrLog, log)
	}
}

// Finish completes the simulation
func (c *Calliope) Finish() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		body, err := ioutil.ReadAll(r.Body)

		if err != nil {
			utils.Log(err)
			http.Error(w, readBodyErrMsg, http.StatusBadRequest)
			return
		}

		var request models.Request

		if err := json.Unmarshal(body, &request); err != nil {
			utils.Log(err)
			http.Error(w, readRequestDataErrMsg, http.StatusBadRequest)
			return
		}

		modelIDStr := utils.ToStr(request.ModelID)
		if err := c.closeSimLog(modelIDStr); err != nil {
			utils.Log(err)
			http.Error(w, modelIDNotExistErrMsg, http.StatusBadRequest)
			return
		}

		c.showLogs()

		// If Calliope failed, send failure callback to the platform
		// so the model status is updated to "failed" instead of staying "calculating"
		if request.CalliopeExit != 0 {
			callbackURL := request.CallbackURL
			userID := utils.ToStr(request.UserID)
			modelID := modelIDStr
			errorMsg := fmt.Sprintf("Calliope simulation failed (exit code %d)", request.CalliopeExit)

			cfg := config.GetConfig()
			fmt.Printf("[Calliope Finish] Calliope failed (exit=%d), sending failure callback to %s\n", request.CalliopeExit, callbackURL)
			if err := utils.SendFailureCallback(callbackURL, userID, modelID, errorMsg, cfg.CallbackSecret); err != nil {
				fmt.Printf("[Calliope Finish] ERROR: Failed to send failure callback: %v\n", err)
				utils.Log(err)
			} else {
				fmt.Printf("[Calliope Finish] Failure callback sent successfully\n")
			}

			// Clean up simulation folder
			folder := simFolderPath + "/" + simFolderPrefix + modelID
			if err := utils.RemoveDir(folder); err != nil {
				utils.Log(err)
			}
		}

		// Clean up the simulation entry to prevent memory leak
		if err := c.removeSimLog(modelIDStr); err != nil {
			utils.Log(err)
		}

		// Clean up model-specific data directory (PV/wind/biomass output + symlinks)
		modelDataDir := dataFolderPath + "/" + modelIDStr
		if err := utils.RemoveDir(modelDataDir); err != nil {
			// Non-fatal: calliope.sh may have already cleaned this up.
			if !os.IsNotExist(err) {
				log.Printf("Note: model data dir cleanup: %v\n", err)
			}
		}

		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}
