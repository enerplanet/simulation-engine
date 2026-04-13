package simulations

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"
	models "webservice/models/charging"
	"webservice/validator"
	"webservice/utils"
)

// Calliope YAMLbyte
type Charging struct {
	coefs             []models.Data
	durations         []models.Data
	tmResM            int64
	tmRampupM         float64
	activeSimulations map[string]simLogCharging
}

// NewCharging creates a new Charging struct
func NewCharging() *Charging {
	return &Charging{
		coefs:             parseCSV(utils.ReadCSVFile(dataFolderPath + "/" + chargingFolderPath + "/" + coefsFile)),
		durations:         parseCSV(utils.ReadCSVFile(dataFolderPath + "/" + chargingFolderPath + "/" + durationsFile)),
		tmResM:            60,
		tmRampupM:         1,
		activeSimulations: make(map[string]simLogCharging),
	}
}

func parseCSV(data [][]string) []models.Data {
	var recs []models.Data
	for i, line := range data {
		if i >= 0 {
			var rec models.Data
			var vals []float64
			for j, field := range line {
				if j == 0 {
					rec.Category = field
				} else {
					val, _ := strconv.ParseFloat(field, 64)
					vals = append(vals, val)
					rec.Values = vals
				}
			}
			recs = append(recs, rec)
		}
	}
	return recs
}

func (c *Charging) isModelIDDuplicate(ID string) bool {
	for _, l := range c.activeSimulations {
		if l.ID == ID {
			return true
		}
	}

	return false
}

func (c *Charging) newSimLog(ID string) error {
	if c.isModelIDDuplicate(ID) {
		return errors.New(duplicateModelIDErrMsg)
	}

	c.activeSimulations[ID] = simLogCharging{
		ID:       ID,
		Start:    time.Now(),
		Duration: time.Duration(0),
	}

	return nil
}

func (c *Charging) closeSimLog(ID string) error {
	_, ok := c.activeSimulations[ID]
	if !ok {
		return errors.New("Charging | " + IDNotExistErrMsg)
	}

	simLog := c.activeSimulations[ID]
	simLog.Duration = time.Since(simLog.Start)
	c.activeSimulations[ID] = simLog

	return nil
}

func (c *Charging) removeSimLog(ID string) error {
	_, ok := c.activeSimulations[ID]
	if !ok {
		return errors.New("Charging | " + IDNotExistErrMsg)
	}

	delete(c.activeSimulations, ID)

	return nil
}

func (c *Charging) showLogs() {
	for _, l := range c.activeSimulations {
		fmt.Printf("Charging | ID: %s, T %s, D %d \n", l.ID, l.Start.String(), l.Duration.Milliseconds())
	}
}

// Path returns the path
func (c Charging) Path() string {
	return pathCharging
}

// Configure prepares the simulation
func (c Charging) Configure() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}

// Generate creates a simulation
func (c Charging) Generate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}

// Start runs the simulation
func (c Charging) Start() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		body, err := ioutil.ReadAll(r.Body)

		if err != nil {
			utils.Log(err)
			http.Error(w, readBodyErrMsg, http.StatusBadRequest)
			return
		}

		validateResults, err := validator.ValidateJSON(string(body), pathCharging)
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

		var request models.Request

		if err := json.Unmarshal(body, &request); err != nil {
			utils.Log(err)
			http.Error(w, readRequestDataErrMsg, http.StatusBadRequest)
			return
		}

		if err := c.newSimLog(request.ID); err != nil {
			utils.Log(err)
			http.Error(w, modelIDExistErrMsg, http.StatusBadRequest)
			return
		}

		selectedChargingStations, warnings := readSelectedCategories(&request)
		chargingProfiles, warnings, err := createChargingProfiles(selectedChargingStations, warnings, &c, &request)
		if err != nil {
			utils.Log(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

		result := models.Response{
			ID:               request.ID,
			Warnings:         warnings,
			ChargingProfiles: chargingProfiles,
		}

		if err := c.closeSimLog(request.ID); err != nil {
			utils.Log(err)
			http.Error(w, modelIDNotExistErrMsg, http.StatusBadRequest)
			return
		}

		if err := c.removeSimLog(request.ID); err != nil {
			utils.Log(err)
			http.Error(w, modelIDNotExistErrMsg, http.StatusBadRequest)
			return
		}

		stringJSON, err := json.MarshalIndent(&result, "", "\t")
		if err != nil {
			utils.Log(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		c.showLogs()

		w.Header().Set(contTypeHeader, contTypeAppJSON)

		fmt.Fprintf(w, templStrResponse, schemaValidateString, stringJSON)
	}
}

func getStationTypeValues(data []models.Data, stationType string) []float64 {
	for i := range data {
		if data[i].Category == stationType {
			return data[i].Values
		}
	}
	return nil
}

func getChargingStationType(powerKw float64) (string, string) {
	var class string
	var warning = ""
	if powerKw <= 4 {
		class = "P<=4"
	} else if 4 < powerKw && powerKw <= 12 {
		class = "4<P<=12"
	} else if 12 < powerKw && powerKw <= 25 {
		class = "12<P<=25"
	} else if 25 < powerKw && powerKw <= 100 {
		class = "25<P<=100"
	} else if 100 < powerKw {
		class = "100<P"
	} else {
		warning = "unspecified charging power"
		class = ""
	}
	return class, warning
}

func getAllOfType(chargingStationType string, request *models.Request) ([]models.ChargingStation, string) {
	var selectedStations []models.ChargingStation
	warning := ""
	for _, cs := range request.ChargingStations {
		csType, warn := getChargingStationType(cs.PowerKw)
		warning = warn
		if chargingStationType == csType {
			tCs := new(models.ChargingStation)
			tCs.PoiID = cs.PoiID
			tCs.Location = cs.Location
			tCs.PowerKw = cs.PowerKw
			selectedStations = append(selectedStations, *tCs)
		}
	}
	return selectedStations, warning
}

func readSelectedCategories(request *models.Request) (map[string]int, []string) {
	selectedCategory := map[string]int{}
	warnings := []string{}
	warning := ""
	for _, cs := range request.ChargingStations {
		powerKw := int(cs.PowerKw)
		if powerKw <= 4 {
			_, exists := selectedCategory["P<=4"]
			if !exists {
				selectedCategory["P<=4"] = 1
			} else {
				selectedCategory["P<=4"] = selectedCategory["P<=4"] + 1
			}
		} else if 4 < powerKw && powerKw <= 12 {
			_, exists := selectedCategory["4<P<=12"]
			if !exists {
				selectedCategory["4<P<=12"] = 1
			} else {
				selectedCategory["4<P<=12"] = selectedCategory["4<P<=12"] + 1
			}
		} else if 12 < powerKw && powerKw <= 25 {
			_, exists := selectedCategory["12<P<=25"]
			if !exists {
				selectedCategory["12<P<=25"] = 1
			} else {
				selectedCategory["12<P<=25"] = selectedCategory["12<P<=25"] + 1
			}
		} else if 25 < powerKw && powerKw <= 100 {
			_, exists := selectedCategory["25<P<=100"]
			if !exists {
				selectedCategory["25<P<=100"] = 1
			} else {
				selectedCategory["25<P<=100"] = selectedCategory["25<P<=100"] + 1
			}
		} else if 100 < powerKw {
			_, exists := selectedCategory["100<P"]
			if !exists {
				selectedCategory["100<P"] = 1
			} else {
				selectedCategory["100<P"] = selectedCategory["100<P"] + 1
			}
		} else {
			warning = "unspecified charging power"
		}
	}
	if warning != "" {
		warnings = append(warnings, warning)
	}
	return selectedCategory, warnings
}

func checkXRange(x float64) error {
	if 0 > x || x > 1.0 {
		return fmt.Errorf("x outside allowed range [0,1[")
	}
	return nil
}

func getOccupancyProb(coefs []models.Data, stationType string, x float64) (float64, error) {
	if err := checkXRange(x); err != nil {
		return 0, err
	}
	result := 0.0
	expo := float64(len(getStationTypeValues(coefs, stationType)) - 1)
	coefficients := getStationTypeValues(coefs, stationType)
	for i := len(coefficients) - 1; i >= 0; i-- {
		factor := math.Pow(x, expo)
		result += coefficients[i] * factor
	}
	return result, nil
}

func getStationNumberPerType(coefs []models.Data, stationType string, numSelected int, x float64) (int, error) {
	prob, err := getOccupancyProb(coefs, stationType, x)
	if err != nil {
		return 0, err
	}
	return int(math.Floor(float64(numSelected) * prob)), nil
}

func getOccDurProb(durations []models.Data, stationType string, period int) float64 {
	return getStationTypeValues(durations, stationType)[period]
}

func occupDurationM(durations []models.Data, stationType string) int {
	cnt := 0
	for i := range getStationTypeValues(durations, stationType) {
		occDurProb := getOccDurProb(durations, stationType, i)
		rnd := float64(rand.Float64() * 100)
		if rnd <= occDurProb {
			cnt += 1
		}
	}
	return cnt * 20
}

func timeToNormtime(hours int, minutes int) float64 {
	return (float64(hours) + float64(minutes)/60.0) / 24.0
}

func pauseKept(tailSlice []float64, pauseSlice []float64) bool {
	pauseKept := true
	for i := 0; i < len(tailSlice); i++ {

		if pauseSlice[i] != tailSlice[i] {
			pauseKept = false
			break
		}
	}
	return pauseKept
}

func prepareDemandProfile(durations []models.Data, tmResM int64, tmRampupM float64, cst string, durM int, chargePowerKw float64) ([]float64, []float64, []float64) {

	//Create a charging interval according to the duration
	//Total number of time slices of the charging process of duration dur_m
	totalChargingSlices := int(math.Ceil(float64(durM) / float64(tmResM)))

	//Number of padding slices to fill up the charging profile according to the number of slices considered in the durations statistics
	padSlices, _ := math.Modf((float64((len(getStationTypeValues(durations, cst))))*20 - float64(durM)) / float64(tmResM))

	//After a charging process we consider that a charging station might be not used for a certain period of time slices
	lenPauseSlice := int(math.Floor((rand.Float64()*4+1)*20) / float64(tmResM))
	var pauseSlice []float64
	for i := 0; i < lenPauseSlice; i++ {
		pauseSlice = append(pauseSlice, 0.0)
	}

	//Creation of a rampup profile according to the charging process energy consumption model
	//we need to consider that a rampup profile could take longer then a single slice. We consider a linearly increasing takeup of charging power and select the max charging power demand per each slice
	var rampupProfile []float64
	for i := int64(1); i <= int64(math.Ceil(float64(tmRampupM)/float64(tmResM)))+1; i++ {
		if tmRampupM/float64(tmResM) < 1 {
			rampupProfile = append(rampupProfile, chargePowerKw)
		} else {
			rampupProfile = append(rampupProfile, chargePowerKw/float64(tmRampupM*(float64(i))))
		}
	}

	//Create the charging profile starting without considering rampup, padding or pause
	var demandProfile []float64
	for i := 0; i < totalChargingSlices; i++ {
		demandProfile = append(demandProfile, chargePowerKw)
	}

	//Appending the pad_slices at the end of the charging demand profile
	for i := 0; i < int(padSlices); i++ {
		demandProfile = append(demandProfile, 0.0)
	}

	return rampupProfile, demandProfile, pauseSlice
}

func createChargingProfiles(selectedCsCats map[string]int, warnings []string, charging *Charging, request *models.Request) (map[string][]float64, []string, error) {
	tsFrom := request.StartDate
	tsTo := request.EndDate

	if tsTo.Before(tsFrom) {
		return nil, warnings, fmt.Errorf("end date must be later than start date")
	}

	ts0 := tsFrom.Unix()
	ts1 := tsTo.Unix()

	var slices []int64
	for i := ts0; i <= ts1; i += charging.tmResM * 60 {
		slices = append(slices, i)
	}

	chargingProfiles := map[string][]float64{}

	// should be selectedCsCats
	for cst, numCst := range selectedCsCats {
		for sl, ts := range slices {
			// log.Print("Charging station", cst, sl)
			var tsIntMid = time.Unix(ts+60*charging.tmResM/2, 0)
			// cs_type := get_cs_type(cst.PowerKw)
			numStations, err := getStationNumberPerType(charging.coefs, cst, numCst, timeToNormtime(tsIntMid.Hour(), tsIntMid.Minute()))
			if err != nil {
				return nil, warnings, err
			}
			stationsOfType, warn := getAllOfType(cst, request)
			if warn != "" {
				warnings = append(warnings, warn)
			}
			for j := 0; j <= numStations; j++ {

				//estimate occup_durattion
				durM := occupDurationM(charging.durations, cst)
				if durM == 0 {
					warnings = append(warnings, "charing duration is considered as 0 lenght. Continue to next Charging station")
					continue
				}

				//randomly select one of the selected stations of considered type
				randIdx := rand.Intn(len(stationsOfType))
				//extract paras of the selected charging station
				chargePowerKw := float64(stationsOfType[randIdx].PowerKw)
				stationId := stationsOfType[randIdx].PoiID
				//remove the selected station from the list
				stationsOfType = append(stationsOfType[:randIdx], stationsOfType[randIdx+1:]...)
				//get potentially already existing charging profile for the selected charging station
				existingProfile, exists := chargingProfiles[stationId]

				rampupProfile, demandProfile, pauseSlice := prepareDemandProfile(charging.durations, charging.tmResM, charging.tmRampupM, cst, durM, chargePowerKw)

				if !exists { //There is currently no profile existing for the selected charging station (new profile)
					//we need to check of the rampup profile
					if len(demandProfile) >= len(rampupProfile) {
						demandProfileCp := make([]float64, len(demandProfile))
						copy(demandProfileCp, rampupProfile)
						demandProfile = append(demandProfileCp, demandProfile[len(rampupProfile)+1:]...)
					} else {
						for i := 0; i < len(demandProfile); i++ {
							demandProfile[i] = rampupProfile[i]
						}

						var fill []float64
						for i := 0; i <= sl-1; i++ {
							fill = append(fill, 0.0)
						}
						demandProfile = append(fill, demandProfile...)

						var pause_slice []float64
						for i := 1; i <= len(pause_slice); i++ {
							pause_slice = append(pause_slice, float64(0))
						}
						demandProfile = append(demandProfile, pause_slice...)
						demandProfile = append(existingProfile, demandProfile...)
					}
					chargingProfiles[stationId] = demandProfile
				} else if sl <= len(existingProfile) { //If the current time slice in smaller as the existing profile - should be the standard case

					//First we need to check if the break between consequitive charging processes is kept
					if len(pauseSlice) > 0 {
						tail := existingProfile[len(existingProfile)-len(pauseSlice):]
						if !pauseKept(tail, pauseSlice) {
							warnings = append(warnings, "pause not kept for the selected charging station, continue with next")
							continue
						}
					}

					//Adding the rampup profile
					if len(demandProfile) >= len(rampupProfile) {
						demandProfileCp := make([]float64, len(demandProfile))
						copy(demandProfileCp, rampupProfile)
						demandProfile = append(demandProfileCp, demandProfile[len(rampupProfile)+1:]...)
					} else {
						for i := 0; i < len(demandProfile); i++ {
							demandProfile[i] = rampupProfile[i]
						}
					}

					var pause_slice []float64
					for i := 1; i <= len(pause_slice); i++ {
						pause_slice = append(pause_slice, float64(0))
					}
					demandProfile = append(demandProfile, pause_slice...)
					demandProfile = append(existingProfile, demandProfile...)
					chargingProfiles[stationId] = demandProfile

				} else {
					warnings = append(warnings, "may not happen")
					return nil, warnings, fmt.Errorf("may not happen")
				}
			}
		}
	}
	return chargingProfiles, warnings, nil
}

// Show gives information about the progress of the simulation
func (c Charging) Show() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		activeSimsJSON, err := json.Marshal(c.activeSimulations)
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
func (c Charging) Log() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		log, err := utils.ReadFile(logFolderPath + "/" + logFileCharging)
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
func (c Charging) Finish() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.showLogs()
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}
