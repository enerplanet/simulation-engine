package simulations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"webservice/config"
	models "webservice/models/calliope"
	"webservice/services"
	"webservice/utils"
)

// Orchestrator sequences the pre-processing steps that used to be tangled
// inside Calliope.Start(): BuEM heating/cooling enrichment, then dispatching
// to the renewables techs (currently just PV — wind/biomass/geothermal were
// dropped). Each step goes through that service's own clean entrypoint
// (EnrichTopologyWithBuem, sendTechSimRequest over HTTP), the same shape
// Ignis already used standing alone. Calliope itself is invoked directly
// in-process, since it lives in the same binary — no network hop needed for
// a same-process call.
//
// The Calliope -> PyPSA handoff is intentionally NOT chained here: the
// architecture diagram marks that link as still undecided.
type Orchestrator struct {
	calliope *Calliope
}

// NewOrchestrator creates a new Orchestrator wrapping the given Calliope handler.
func NewOrchestrator(calliope *Calliope) *Orchestrator {
	return &Orchestrator{calliope: calliope}
}

// Path returns the URL base path for this simulation.
func (o *Orchestrator) Path() string { return pathOrchestrator }

// Configure is a no-op stub satisfying the Simulation interface.
func (o *Orchestrator) Configure() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}

// Generate is a no-op stub satisfying the Simulation interface.
func (o *Orchestrator) Generate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}

// Start enriches the incoming config with BuEM heating/cooling profiles,
// dispatches PV, waits for its generated resource files, then forwards the
// enriched config to Calliope's own /calliope/start handler.
func (o *Orchestrator) Start() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			utils.Log(err)
			http.Error(w, readBodyErrMsg, http.StatusBadRequest)
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

		modelID := utils.ToStr(request.ModelID)

		// Step 1: enrich topology with BuEM heating/cooling profiles.
		if rawTopo, err := json.Marshal(config["topology"]); err == nil {
			startDateStr := request.StartDate.Format("2006-01-02T15:04:05.000Z")
			endDateStr := request.EndDate.Format("2006-01-02T15:04:05.000Z")
			enriched, hasBuem, buemErr := EnrichTopologyWithBuem(rawTopo, startDateStr, endDateStr, modelID, int(request.Resolution))
			if buemErr != nil {
				log.Printf("Orchestrator | WARN: BUEM enrichment failed (continuing without heating profiles): %v", buemErr)
			} else if hasBuem {
				var enrichedTopo interface{}
				if json.Unmarshal(enriched, &enrichedTopo) == nil {
					config["topology"] = enrichedTopo
				}
			}
		}

		// Step 2: dispatch PV (the only renewables tech left) and wait for its output.
		if isTechAvailable(pvSup, config) {
			log.Printf("Orchestrator | Attempting to send %s simulation request\n", pvSup)
			if err := sendTechSimRequest(pvSup, config); err != nil {
				errMsg := fmt.Sprintf("%s simulation request failed: %v", pvSup, err)
				log.Printf("Orchestrator | ERROR: %s\n", errMsg)
				http.Error(w, errMsg, http.StatusBadGateway)
				return
			}
		} else {
			log.Printf("Orchestrator | Tech %s is not available in the config\n", pvSup)
		}

		if err := waitForGeneratedTechResources(request, modelID, 120*time.Second); err != nil {
			errMsg := fmt.Sprintf("generated tech resource files are not ready: %v", err)
			log.Printf("Orchestrator | ERROR: %s\n", errMsg)
			http.Error(w, errMsg, http.StatusBadGateway)
			return
		}

		// Step 3: forward the (possibly enriched) config to Calliope's own handler.
		enrichedBody, err := json.Marshal(config)
		if err != nil {
			utils.Log(err)
			http.Error(w, readConfigErrMsg, http.StatusInternalServerError)
			return
		}

		forwardReq, err := http.NewRequest(r.Method, r.URL.String(), bytes.NewReader(enrichedBody))
		if err != nil {
			utils.Log(err)
			http.Error(w, readRequestDataErrMsg, http.StatusInternalServerError)
			return
		}
		forwardReq.Header = r.Header

		o.calliope.Start()(w, forwardReq)
	}
}

// Show delegates to Calliope's own active-simulations view.
func (o *Orchestrator) Show() http.HandlerFunc { return o.calliope.Show() }

// Log delegates to Calliope's own log file.
func (o *Orchestrator) Log() http.HandlerFunc { return o.calliope.Log() }

// Finish delegates to Calliope's own completion handler.
func (o *Orchestrator) Finish() http.HandlerFunc { return o.calliope.Finish() }

// --- Orchestration helpers (moved out of calliope.go — these dispatch to
// external tech services and wait for their output, they are not part of
// building the Calliope model itself) ---

func isTechAvailable(techName string, config map[string]interface{}) bool {
	topology, topologyExists := config["topology"].([]interface{})
	if !topologyExists {
		return false
	}
	for _, element := range topology {
		elementMap, elementIsMap := element.(map[string]interface{})
		if !elementIsMap {
			continue
		}
		// Check both "from" and "to" sides for the tech
		for _, side := range []string{"from", "to"} {
			sideMap, sideExists := elementMap[side].(map[string]interface{})
			if !sideExists {
				continue
			}
			techs, techsExist := sideMap["techs"].(map[string]interface{})
			if !techsExist {
				continue
			}
			if _, techExists := techs[techName]; techExists {
				return true
			}
		}
	}
	return false
}

func sendTechSimRequest(techName string, body map[string]interface{}) error {
	cfg := config.GetConfig()
	serviceConfig, exists := cfg.GetTechServiceConfig(techName)
	if !exists {
		log.Printf("No service config found for tech: %s (skipping)\n", techName)
		return nil
	}

	apiUrl := serviceConfig.URL("/start")
	httpClient := services.NewResilientHTTPClient()

	var responseMap map[string]interface{}
	err := httpClient.PostJSONAndDecode(apiUrl, body, &responseMap)
	if err != nil {
		return fmt.Errorf("service %s request failed: %w", techName, err)
	}

	message, ok := responseMap["message"].(string)
	if !ok {
		message = "No message provided"
	}

	log.Printf("%s simulation completed. Message: %s\n", techName, message)
	return nil
}

func waitForGeneratedTechResources(request models.Request, modelID string, timeout time.Duration) error {
	expectedFiles := collectExpectedResourceFiles(request)
	if len(expectedFiles) == 0 {
		return nil
	}

	deadline := time.Now().Add(timeout)
	var missing []string
	for {
		missing = missing[:0]
		for _, fileName := range expectedFiles {
			if !ensureModelResourceFile(modelID, fileName) {
				missing = append(missing, fileName)
			}
		}

		if len(missing) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			break
		}

		time.Sleep(2 * time.Second)
	}

	preview := missing
	const previewLimit = 12
	if len(preview) > previewLimit {
		preview = preview[:previewLimit]
	}

	return fmt.Errorf("timeout after %s waiting for %d files; missing %d (sample: %s)", timeout, len(expectedFiles), len(missing), strings.Join(preview, ", "))
}

func collectExpectedResourceFiles(request models.Request) []string {
	fileSet := make(map[string]struct{})

	for _, conn := range request.Topology {
		addExpectedResourceFiles(fileSet, conn.From)
		addExpectedResourceFiles(fileSet, conn.To)
	}

	files := make([]string, 0, len(fileSet))
	for fileName := range fileSet {
		files = append(files, fileName)
	}
	sort.Strings(files)

	return files
}

func addExpectedResourceFiles(fileSet map[string]struct{}, feature models.Feature) {
	coords := feature.Geometry.Coordinates
	if len(coords) < 2 {
		return
	}

	for techID := range feature.Techs {
		if fileName, ok := resourceFileNameForTech(techID, coords); ok {
			fileSet[fileName] = struct{}{}
		}
	}
}

func resourceFileNameForTech(techID string, coords []float64) (string, bool) {
	if len(coords) < 2 {
		return "", false
	}

	lat := coords[1]
	lon := coords[0]

	if isPv(techID) {
		return fmt.Sprintf("pv_%g_%g.csv", lat, lon), true
	}
	return "", false
}

func ensureModelResourceFile(modelID string, fileName string) bool {
	modelPath := filepath.Join(dataFolderPath, modelID, fileName)
	if info, err := os.Stat(modelPath); err == nil && info.Size() > 0 {
		return true
	}

	legacyPath := filepath.Join(dataFolderPath, fileName)
	legacyInfo, err := os.Stat(legacyPath)
	if err != nil || legacyInfo.Size() <= 0 {
		return false
	}

	if _, err := os.Lstat(modelPath); os.IsNotExist(err) {
		absLegacyPath, absErr := filepath.Abs(legacyPath)
		if absErr != nil {
			return false
		}

		if symlinkErr := os.Symlink(absLegacyPath, modelPath); symlinkErr != nil && !os.IsExist(symlinkErr) {
			return false
		}
	}

	info, err := os.Stat(modelPath)
	return err == nil && info.Size() > 0
}
