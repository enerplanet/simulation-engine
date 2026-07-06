package simulations

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"webservice/config"
	"webservice/models/buem"
	"webservice/services"
	"webservice/utils"
)

const (
	pathBUEM          = "/buem"
	logFileBUEM       = "buem.log"
	buemDataDirEnv    = "BUEM_DATA_DIR"
	buemResultsDirEnv = "BUEM_RESULTS_DIR"
	buemAPIProcess    = "/api/process"
	buemResultsDir    = "results"
	buemFilesPrefix   = "/api/files/"
)

var (
	buemSemaphore     chan struct{}
	buemSemaphoreOnce sync.Once
)

func getBUEMSemaphore() chan struct{} {
	buemSemaphoreOnce.Do(func() {
		maxConcurrent := config.GetConfig().MaxConcurrentSims
		if maxConcurrent <= 0 {
			maxConcurrent = 4
		}
		buemSemaphore = make(chan struct{}, maxConcurrent)
	})
	return buemSemaphore
}

// buemTask is a single building to process.
type buemTask struct {
	nodeID     string
	lat, lon   float64
	year       int
	modelID    string          // EnerPlanET model ID — used to isolate CSV output per model
	rawFeature json.RawMessage // BuEM spec Feature, ready to send
}

// buemOutcome is the result of one BuEM run.
type buemOutcome struct {
	nodeID                 string
	buemBlock              json.RawMessage // enriched properties.buem to merge back
	errMsg                 string
	wallDuration           time.Duration
	csvWriteDuration       time.Duration
	modelProcessingSeconds float64
}

type buemRunMetrics struct {
	wallDuration           time.Duration
	csvWriteDuration       time.Duration
	modelProcessingSeconds float64
}

// BUEM fans out building-level requests to the BuEM Flask microservice and
// writes heating and cooling CSV profiles to the shared Docker volume for
// downstream consumption by Calliope and PyPSA.
type BUEM struct{}

// NewBUEM creates a new BUEM handler.
func NewBUEM() *BUEM { return &BUEM{} }

// Path returns the URL base path for this simulation.
func (b *BUEM) Path() string { return pathBUEM }

// Configure is a no-op stub satisfying the Simulation interface.
func (b *BUEM) Configure() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}

// Generate is a no-op stub satisfying the Simulation interface.
func (b *BUEM) Generate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}

// Start receives an EnerPlanET config.json, extracts buildings that carry a
// "buem" block in their properties, sends each building to the BuEM service
// concurrently, writes heating and cooling CSV profiles to the shared Docker
// volume, and returns the original config enriched with BuEM results under
// each building node's properties.buem.
func (b *BUEM) Start() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestStart := time.Now()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			utils.Log(err)
			http.Error(w, readBodyErrMsg, http.StatusBadRequest)
			return
		}

		// Keep the full config as a raw map so all non-buem fields pass through
		// unchanged in the response.
		var rawConfig map[string]json.RawMessage
		if err := json.Unmarshal(body, &rawConfig); err != nil {
			utils.Log(err)
			http.Error(w, readRequestDataErrMsg, http.StatusBadRequest)
			return
		}

		var startDate, endDate, modelID string
		var resolution int
		json.Unmarshal(rawConfig["start_date"], &startDate)
		json.Unmarshal(rawConfig["end_date"], &endDate)
		json.Unmarshal(rawConfig["resolution"], &resolution)
		json.Unmarshal(rawConfig["model_id"], &modelID)

		tasks, err := extractBUEMTasks(rawConfig["topology"], startDate, endDate, resolution, modelID)
		if err != nil {
			utils.Log(err)
			http.Error(w, "failed to parse topology", http.StatusBadRequest)
			return
		}

		if len(tasks) == 0 {
			w.Header().Set(contTypeHeader, contTypeAppJSON)
			w.Write(body)
			return
		}

		log.Printf("BUEM | model=%s found %d buildings with buem block", modelID, len(tasks))

		sem := getBUEMSemaphore()
		ch := make(chan buemOutcome, len(tasks))
		var wg sync.WaitGroup

		for _, task := range tasks {
			wg.Add(1)
			go func(t buemTask) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				block, metrics, err := runBUEMFeature(t)
				if err != nil {
					ch <- buemOutcome{nodeID: t.nodeID, errMsg: err.Error()}
				} else {
					ch <- buemOutcome{
						nodeID:                 t.nodeID,
						buemBlock:              block,
						wallDuration:           metrics.wallDuration,
						csvWriteDuration:       metrics.csvWriteDuration,
						modelProcessingSeconds: metrics.modelProcessingSeconds,
					}
				}
			}(task)
		}

		wg.Wait()
		close(ch)

		results := make(map[string]buemOutcome, len(tasks))
		failed := 0
		var totalWallDuration time.Duration
		var totalCSVWriteDuration time.Duration
		var totalModelProcessingSeconds float64
		successful := 0
		for o := range ch {
			results[o.nodeID] = o
			if o.errMsg != "" {
				failed++
				log.Printf("BUEM | node=%s error: %s", o.nodeID, o.errMsg)
				continue
			}
			successful++
			totalWallDuration += o.wallDuration
			totalCSVWriteDuration += o.csvWriteDuration
			totalModelProcessingSeconds += o.modelProcessingSeconds
		}

		enrichedTopology, err := mergeIntoTopology(rawConfig["topology"], results)
		if err != nil {
			utils.Log(err)
			http.Error(w, "failed to merge BUEM results into topology", http.StatusInternalServerError)
			return
		}
		rawConfig["topology"] = enrichedTopology

		requestDuration := time.Since(requestStart)
		logBUEMBatchSummary(successful, failed, requestDuration, totalWallDuration, totalCSVWriteDuration, totalModelProcessingSeconds)
		w.Header().Set(contTypeHeader, contTypeAppJSON)
		json.NewEncoder(w).Encode(rawConfig)
	}
}

// EnrichTopologyWithBuem is the callable core of the BUEM pre-processing step.
// It is called by the Calliope handler before launching the energy simulation so
// that heating_file / cooling_file paths are injected into the topology before
// Calliope reads it.
//
// If no topology nodes carry a buem block the function returns (rawTopology, false, nil)
// and the caller may proceed unchanged. Errors are non-fatal from the Calliope
// perspective — the caller should log and continue without heating profiles.
func EnrichTopologyWithBuem(rawTopology json.RawMessage, startDate, endDate, modelID string, resolution int) (json.RawMessage, bool, error) {
	tasks, err := extractBUEMTasks(rawTopology, startDate, endDate, resolution, modelID)
	if err != nil {
		return rawTopology, false, fmt.Errorf("parse buem tasks: %w", err)
	}
	if len(tasks) == 0 {
		return rawTopology, false, nil
	}

	log.Printf("BUEM | model=%s found %d buildings with buem block (calliope pre-process)", modelID, len(tasks))

	sem := getBUEMSemaphore()
	ch := make(chan buemOutcome, len(tasks))
	var wg sync.WaitGroup

	for _, task := range tasks {
		wg.Add(1)
		go func(t buemTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			block, _, err := runBUEMFeature(t)
			if err != nil {
				ch <- buemOutcome{nodeID: t.nodeID, errMsg: err.Error()}
			} else {
				ch <- buemOutcome{nodeID: t.nodeID, buemBlock: block}
			}
		}(task)
	}

	wg.Wait()
	close(ch)

	results := make(map[string]buemOutcome, len(tasks))
	for o := range ch {
		results[o.nodeID] = o
		if o.errMsg != "" {
			log.Printf("BUEM | node=%s error: %s", o.nodeID, o.errMsg)
		}
	}

	enriched, err := mergeIntoTopology(rawTopology, results)
	if err != nil {
		return rawTopology, true, fmt.Errorf("merge buem results: %w", err)
	}
	return enriched, true, nil
}

// extractBUEMTasks walks the topology edges, finds BasePOI nodes that carry a
// "buem" block, deduplicates by node ID, and returns one task per building.
func extractBUEMTasks(rawTopology json.RawMessage, startDate, endDate string, resolution int, modelID string) ([]buemTask, error) {
	var rawEdges []json.RawMessage
	if err := json.Unmarshal(rawTopology, &rawEdges); err != nil {
		return nil, fmt.Errorf("parse topology edges: %w", err)
	}

	var tasks []buemTask
	seen := make(map[string]bool)

	for _, rawEdge := range rawEdges {
		var edge struct {
			From json.RawMessage `json:"from"`
			To   json.RawMessage `json:"to"`
		}
		if err := json.Unmarshal(rawEdge, &edge); err != nil {
			continue
		}
		for _, rawNode := range []json.RawMessage{edge.From, edge.To} {
			task, ok := nodeToTask(rawNode, startDate, endDate, resolution, modelID)
			if !ok || seen[task.nodeID] {
				continue
			}
			seen[task.nodeID] = true
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

// nodeToTask checks whether a topology node is a building with a buem block
// and, if so, builds the BuEM spec Feature that will be sent to the service.
func nodeToTask(rawNode json.RawMessage, startDate, endDate string, resolution int, modelID string) (buemTask, bool) {
	var node struct {
		ID       string `json:"id"`
		Geometry struct {
			Coordinates []float64 `json:"coordinates"`
		} `json:"geometry"`
		Properties struct {
			FeatureType string          `json:"feature_type"`
			BUEM        json.RawMessage `json:"buem"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(rawNode, &node); err != nil {
		return buemTask{}, false
	}
	if node.Properties.FeatureType != "BasePOI" {
		return buemTask{}, false
	}
	if len(node.Properties.BUEM) == 0 || string(node.Properties.BUEM) == "null" {
		return buemTask{}, false
	}
	if len(node.Geometry.Coordinates) < 2 {
		return buemTask{}, false
	}

	lon := node.Geometry.Coordinates[0]
	lat := node.Geometry.Coordinates[1]

	year, err := yearFromStartTime(startDate)
	if err != nil {
		return buemTask{}, false
	}

	// Build a BuEM spec Feature. The buem block is forwarded as-is.
	rawFeat, err := json.Marshal(map[string]interface{}{
		"type": "Feature",
		"id":   node.ID,
		"geometry": map[string]interface{}{
			"type":        "Point",
			"coordinates": node.Geometry.Coordinates,
		},
		"properties": map[string]interface{}{
			"start_time":      startDate,
			"end_time":        endDate,
			"resolution":      strconv.Itoa(resolution),
			"resolution_unit": "minutes",
			"buem":            node.Properties.BUEM,
		},
	})
	if err != nil {
		return buemTask{}, false
	}

	return buemTask{
		nodeID:     node.ID,
		lat:        lat,
		lon:        lon,
		year:       year,
		modelID:    modelID,
		rawFeature: rawFeat,
	}, true
}

// runBUEMFeature sends one BuEM spec Feature to the BuEM service, writes
// heating and cooling CSV profiles to the shared volume, and returns the
// enriched buem block (timeseries retained, file paths injected).
func runBUEMFeature(task buemTask) (json.RawMessage, buemRunMetrics, error) {
	wallStart := time.Now()
	singleFC := buem.FeatureCollection{
		Type:     "FeatureCollection",
		Features: []json.RawMessage{task.rawFeature},
	}

	cfg := config.GetConfig()
	url := cfg.BUEMService.URL(buemAPIProcess) + "?include_timeseries=true"
	client := services.NewResilientHTTPClient()

	var respFC buem.ResponseFeatureCollection
	if err := client.PostJSONAndDecode(url, singleFC, &respFC); err != nil {
		return nil, buemRunMetrics{}, fmt.Errorf("BUEM request: %w", err)
	}
	if len(respFC.Features) == 0 {
		return nil, buemRunMetrics{}, fmt.Errorf("BUEM response has no features")
	}

	buemBlock := &respFC.Features[0].Properties.BUEM
	modelSecs := buemBlock.ModelMetadata.ProcessingTime.Value

	enriched, csvWriteDuration, err := extractWriteAndAnnotate(buemBlock, task.lat, task.lon, task.year, task.modelID)
	if err != nil {
		return nil, buemRunMetrics{}, err
	}

	metrics := buemRunMetrics{
		wallDuration:           time.Since(wallStart),
		csvWriteDuration:       csvWriteDuration,
		modelProcessingSeconds: modelSecs,
	}
	log.Printf(
		"BUEM | node=%s lat=%.6f lon=%.6f year=%d wall=%s model=%.3fs csv=%s",
		task.nodeID,
		task.lat,
		task.lon,
		task.year,
		metrics.wallDuration.Round(time.Millisecond),
		metrics.modelProcessingSeconds,
		metrics.csvWriteDuration.Round(time.Millisecond),
	)
	return enriched, metrics, nil
}

// extractWriteAndAnnotate writes heating, cooling, and electricity CSVs to the
// shared volume under {BUEM_DATA_DIR}/{modelID}/, injects the file paths into
// the buem block, and returns it as raw JSON. The timeseries arrays are retained.
// If modelID is empty, CSVs are written directly to BUEM_DATA_DIR (test fallback).
func extractWriteAndAnnotate(block *buem.BuEMResponseBlock, lat, lon float64, year int, modelID string) (json.RawMessage, time.Duration, error) {
	ts := block.ThermalLoadProfile.Timeseries
	if ts == nil {
		return nil, 0, fmt.Errorf("BuEM response missing timeseries (include_timeseries=true was requested)")
	}
	if len(ts.Heating) == 0 {
		return nil, 0, fmt.Errorf("heating timeseries is empty")
	}

	latStr := fmt.Sprintf("%.6f", lat)
	lonStr := fmt.Sprintf("%.6f", lon)
	yearStr := strconv.Itoa(year)
	baseDir := os.Getenv(buemDataDirEnv)
	if baseDir == "" {
		baseDir = buemResultsDir
	}
	resultsDir := baseDir
	if modelID != "" {
		resultsDir = filepath.Join(baseDir, modelID)
	}

	csvWriteStart := time.Now()

	heatingFile := fmt.Sprintf("heating_%s_%s_%s.csv", latStr, lonStr, yearStr)
	heatingPath := filepath.Join(resultsDir, heatingFile)
	if err := writeProfileCSV(heatingPath, ts.Heating); err != nil {
		return nil, 0, fmt.Errorf("write heating CSV: %w", err)
	}
	block.ThermalLoadProfile.HeatingFile = heatingPath

	if len(ts.Cooling) > 0 {
		coolingFile := fmt.Sprintf("cooling_%s_%s_%s.csv", latStr, lonStr, yearStr)
		coolingPath := filepath.Join(resultsDir, coolingFile)
		if err := writeProfileCSV(coolingPath, ts.Cooling); err != nil {
			return nil, 0, fmt.Errorf("write cooling CSV: %w", err)
		}
		block.ThermalLoadProfile.CoolingFile = coolingPath
	}

	if len(ts.Electricity) > 0 {
		electricityFile := fmt.Sprintf("electricity_%s_%s_%s.csv", latStr, lonStr, yearStr)
		electricityPath := filepath.Join(resultsDir, electricityFile)
		if err := writeProfileCSV(electricityPath, ts.Electricity); err != nil {
			return nil, 0, fmt.Errorf("write electricity CSV: %w", err)
		}
		block.ThermalLoadProfile.ElectricityFile = electricityPath
	}

	deleteSourceTimeseries(&block.ThermalLoadProfile)

	enriched, err := json.Marshal(block)
	if err != nil {
		return nil, 0, err
	}
	return enriched, time.Since(csvWriteStart), nil
}

// deleteSourceTimeseries removes the .json.gz file that the BuEM Flask service
// saves to the shared Docker volume when include_timeseries=true is requested.
// The timeseries has already been written to CSV, so the gz file is redundant.
// Failures are logged but do not fail the request.
func deleteSourceTimeseries(tlp *buem.ThermalLoadProfile) {
	if !strings.HasPrefix(tlp.TimeseriesFile, buemFilesPrefix) {
		return
	}
	fname := tlp.TimeseriesFile[len(buemFilesPrefix):]
	if fname == "" {
		return
	}

	resultsDir := os.Getenv(buemResultsDirEnv)
	if resultsDir == "" {
		return
	}
	fullPath := filepath.Join(resultsDir, fname)

	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		log.Printf("BUEM | warning: failed to delete source timeseries file %s: %v", fullPath, err)
	} else {
		log.Printf("BUEM | deleted source timeseries file: %s", fullPath)
	}
}

func logBUEMBatchSummary(successful, failed int, requestDuration, totalWallDuration, totalCSVWriteDuration time.Duration, totalModelProcessingSeconds float64) {
	throughput := 0.0
	if requestDuration > 0 {
		throughput = float64(successful) / requestDuration.Seconds()
	}
	avgWall := time.Duration(0)
	avgCSV := time.Duration(0)
	avgModel := 0.0
	if successful > 0 {
		avgWall = totalWallDuration / time.Duration(successful)
		avgCSV = totalCSVWriteDuration / time.Duration(successful)
		avgModel = totalModelProcessingSeconds / float64(successful)
	}
	log.Printf(
		"BUEM | processed=%d failed=%d total=%s avg_wall=%s avg_model=%.3fs avg_csv=%s throughput=%.2f buildings/s",
		successful,
		failed,
		requestDuration.Round(time.Millisecond),
		avgWall.Round(time.Millisecond),
		avgModel,
		avgCSV.Round(time.Millisecond),
		throughput,
	)
}

// mergeIntoTopology injects the enriched buem block into each matching
// topology node, leaving all other node fields unchanged.
func mergeIntoTopology(rawTopology json.RawMessage, results map[string]buemOutcome) (json.RawMessage, error) {
	var edges []interface{}
	if err := json.Unmarshal(rawTopology, &edges); err != nil {
		return nil, fmt.Errorf("parse topology: %w", err)
	}

	for _, edge := range edges {
		edgeMap, ok := edge.(map[string]interface{})
		if !ok {
			continue
		}
		for _, side := range []string{"from", "to"} {
			nodeMap, ok := edgeMap[side].(map[string]interface{})
			if !ok {
				continue
			}
			propsMap, ok := nodeMap["properties"].(map[string]interface{})
			if !ok {
				continue
			}
			nodeID := fmt.Sprintf("%v", nodeMap["id"])
			outcome, found := results[nodeID]
			if !found || outcome.errMsg != "" {
				continue
			}
			var buemData interface{}
			json.Unmarshal(outcome.buemBlock, &buemData)
			propsMap["buem"] = buemData
		}
	}

	return json.Marshal(edges)
}

// writeProfileCSV writes float64 values to a CSV file with a single "demand"
// header column. The format matches Calliope's timeseries_data_path convention.
// Parent directories are created if they do not exist.
func writeProfileCSV(path string, values []float64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var sb strings.Builder
	sb.WriteString("demand\n")
	for _, v := range values {
		sb.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
		sb.WriteByte('\n')
	}
	_, err = fmt.Fprint(f, sb.String())
	return err
}

// yearFromStartTime parses the 4-digit year from an ISO 8601 timestamp string.
func yearFromStartTime(s string) (int, error) {
	if len(s) < 4 {
		return 0, fmt.Errorf("start_time too short: %q", s)
	}
	return strconv.Atoi(s[:4])
}

// Show returns an empty JSON object — BuEM does not track active simulations.
func (b *BUEM) Show() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
		w.Write([]byte("{}"))
	}
}

// Log returns the contents of the BuEM log file.
func (b *BUEM) Log() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		content, err := utils.ReadFile(logFolderPath + "/" + logFileBUEM)
		if err != nil {
			utils.Log(err)
			http.Error(w, readLogErrMsg, http.StatusBadRequest)
			return
		}
		w.Header().Set(contTypeHeader, contTypeAppText)
		fmt.Fprintf(w, templStrLog, content)
	}
}

// Finish is a no-op stub satisfying the Simulation interface.
func (b *BUEM) Finish() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}
