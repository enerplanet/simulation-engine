package simulations

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
	models "webservice/models/csv2json"
	"webservice/utils"
)

// CSV2JSON
type CSV2JSON struct {
	activeSimulations map[string]simLogCSV2JSON
}

// CSV2JSON creates a new Calliope struct
func NewCSV2JSON() *CSV2JSON {
	return &CSV2JSON{make(map[string]simLogCSV2JSON)}
}

// func (c *CSV2JSON) isModelIDDuplicate(ID int64) bool {
// 	for _, l := range c.activeSimulations {
// 		if l.ID == ID {
// 			return true
// 		}
// 	}

// 	return false
// }

// func (c *CSV2JSON) newSimLog(ID int64) error {
// 	if c.isModelIDDuplicate(ID) {
// 		return errors.New(duplicateIDErrMsg)
// 	}

// 	c.activeSimulations[ID] = simLogCSV2JSON{
// 		ID:       ID,
// 		Start:    time.Now(),
// 		Duration: time.Duration(0),
// 	}

// 	return nil
// }

// func (c *CSV2JSON) closeSimLog(ID int64) error {
// 	_, ok := c.activeSimulations[ID]
// 	if !ok {
// 		return errors.New("CSV2JSON | " + IDNotExistErrMsg)
// 	}

// 	simLog := c.activeSimulations[ID]
// 	simLog.Duration = time.Since(simLog.Start)
// 	c.activeSimulations[ID] = simLog

// 	return nil
// }

// func (c *CSV2JSON) removeSimLog(ID int64) error {
// 	_, ok := c.activeSimulations[ID]
// 	if !ok {
// 		return errors.New("CSV2JSON | " + IDNotExistErrMsg)
// 	}

// 	delete(c.activeSimulations, ID)

// 	return nil
// }

func (c CSV2JSON) showLogs() {
	for _, l := range c.activeSimulations {
		fmt.Printf("CSV2JSON | ID: %s, T %s, D %d \n", l.ID, l.Start.String(), l.Duration.Milliseconds())
	}
}

// Path returns the path
func (c CSV2JSON) Path() string {
	return pathCSV2JSON
}

// Configure prepares the simulation
func (c CSV2JSON) Configure() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}

// Generate creates a simulation
func (c CSV2JSON) Generate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}

// Start runs the simulation
func (c CSV2JSON) Start() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		body, err := ioutil.ReadAll(r.Body)

		if err != nil {
			utils.Log(err)
			http.Error(w, readBodyErrMsg, http.StatusBadRequest)
			return
		}

		csvRecords := csv.NewReader(strings.NewReader(string(body)))

		arrRecords, err := csvRecords.ReadAll()
		if err != nil {
			utils.Log(err)
			http.Error(w, "can't read CSV records", http.StatusBadRequest)
			return
		}

		rows, columns, err := checkRowsAndColumn(arrRecords)
		if err != nil {
			utils.Log(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		startTime := getStartTime(arrRecords)
		endTime := getEndTime(arrRecords)

		allValues, err := getAllValues(arrRecords)
		if err != nil {
			utils.Log(err)
			http.Error(w, "can't read values", http.StatusBadRequest)
			return
		}

		delta, err := getDeltaTime(arrRecords)
		if err != nil {
			utils.Log(err)
			http.Error(w, "can't calculate delta time", http.StatusBadRequest)
			return
		}

		responseJSON := models.Response{
			StartTime: startTime,
			DeltaTime: delta,
			EndTime:   endTime,
			Values:    allValues,
			Width:     columns,
			Length:    rows,
		}

		stringJSON, err := json.MarshalIndent(&responseJSON, "", "\t")
		if err != nil {
			utils.Log(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set(contTypeHeader, contTypeAppJSON)

		fmt.Fprintf(w, templStrJSON, stringJSON)
	}
}

func checkRowsAndColumn(records [][]string) (int64, int64, error) {
	rows := len(records)
	columns := len(records[0])
	firstHeader := strings.TrimSpace(records[0][0])
	if rows < 2 && columns < 2 && firstHeader != "time" {
		return int64(0), int64(0), fmt.Errorf(tooLessRowsAndColumnsErrMsg)
	}

	return int64(rows), int64(columns), nil
}

func getStartTime(records [][]string) string {
	return strings.TrimSpace(records[1][0])
}

func getEndTime(records [][]string) string {
	return strings.TrimSpace(records[len(records)-1][0])
}

func getDeltaTime(records [][]string) (int64, error) {
	previousTime, err := time.Parse(jsTimeFormatString, strings.TrimSpace(records[1][0]))
	if err != nil {
		return int64(0), err
	}

	previousDelta := time.Duration(0)
	for nr, row := range records {
		if nr < 2 {
			continue
		}

		currentTime, err := time.Parse(jsTimeFormatString, strings.TrimSpace(row[0]))
		if err != nil {
			return int64(0), err
		}

		delta := currentTime.Sub(previousTime)
		if delta != previousDelta && previousDelta != time.Duration(0) {
			return int64(0), fmt.Errorf(timeSeriesInconsistentErrMsg)
		}

		previousDelta = delta
		previousTime = currentTime
	}

	return int64(previousDelta.Minutes()), nil
}

func getAllValues(records [][]string) (map[string][]float64, error) {
	allValues := make(map[string][]float64)

	headers := getHeaders(records)
	for column, header := range headers {
		if column == 0 {
			continue
		}
		values, err := getValues(records, column)
		if err != nil {
			return nil, err
		}
		allValues[header] = values
	}

	return allValues, nil
}

func getHeaders(records [][]string) []string {
	headers := []string{}
	for _, header := range records[0] {
		headers = append(headers, strings.TrimSpace(header))
	}
	return headers
}

func getValues(records [][]string, column int) ([]float64, error) {
	var values []float64
	for nr, row := range records {
		if nr == 0 {
			continue
		}
		val, err := strconv.ParseFloat(strings.TrimSpace(row[column]), 64)
		if err != nil {
			return nil, err
		}

		values = append(values, val)
	}

	return values, nil
}

// Show gives information about the progress of the simulation
func (c CSV2JSON) Show() http.HandlerFunc {
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
func (c CSV2JSON) Log() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		log, err := utils.ReadFile(logFolderPath + "/" + logFileCSV2JSON)
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
func (c CSV2JSON) Finish() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.showLogs()
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}
