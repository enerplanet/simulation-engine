package simulations

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
	"webservice/config"
	models "webservice/models/pypsa"
	"webservice/utils"
)

// PyPSA manages PyPSA simulation state
type PyPSA struct {
	mu                sync.Mutex
	activeSimulations map[string]simLogPyPSA
}

// NewPyPSA creates a new PyPSA struct
func NewPyPSA() *PyPSA {
	return &PyPSA{activeSimulations: make(map[string]simLogPyPSA)}
}

func (p *PyPSA) isModelIDDuplicate(modelID string) bool {
	// Caller must hold p.mu
	for _, l := range p.activeSimulations {
		if l.ModelID == modelID {
			return true
		}
	}

	return false
}

func (p *PyPSA) newSimLog(modelID string, sessionID string, pyPSA bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isModelIDDuplicate(modelID) {
		return errors.New(duplicateModelIDErrMsg)
	}

	p.activeSimulations[modelID] = simLogPyPSA{
		ModelID:   modelID,
		SessionID: sessionID,
		PyPSA:     pyPSA,
		Start:     time.Now(),
		Duration:  time.Duration(0),
	}

	return nil
}

func (p *PyPSA) closeSimLog(modelID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	simLog, ok := p.activeSimulations[modelID]
	if !ok {
		return errors.New("PyPSA | " + modelIDNotExistErrMsg)
	}

	simLog.Duration = time.Since(simLog.Start)
	p.activeSimulations[modelID] = simLog

	return nil
}

func (p *PyPSA) removeSimLog(modelID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, ok := p.activeSimulations[modelID]
	if !ok {
		return errors.New("PyPSA | " + modelIDNotExistErrMsg)
	}

	delete(p.activeSimulations, modelID)

	return nil
}

func (p *PyPSA) showLogs() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, l := range p.activeSimulations {
		fmt.Printf("PyPSA | MID: %s, SID %s, T %s, D %d \n", l.ModelID, l.SessionID, l.Start.String(), l.Duration.Milliseconds())
	}
}

// Path returns the path
func (p *PyPSA) Path() string {
	return pathPyPSA
}

// Configure prepares the simulation
func (p *PyPSA) Configure() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}

// Generate creates a simulation
func (p *PyPSA) Generate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}

// Start runs the simulation
func (p *PyPSA) Start() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		body, err := ioutil.ReadAll(r.Body)

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

		modelIDStr := utils.ToStr(request.ModelID)
		sessionIDStr := utils.ToStr(request.SessionID)

		if err := p.newSimLog(modelIDStr, sessionIDStr, true); err != nil {
			utils.Log(err)
			http.Error(w, modelIDExistErrMsg, http.StatusBadRequest)
			return
		}

		p.showLogs()

		userID := utils.ToStr(request.UserID)
		sessionID := sessionIDStr
		modelID := modelIDStr
		lkr := request.Lkr
		callbackURL := request.CallbackURL

		cmd := fmt.Sprintf("%s %s %s %s '%s' '%s'", cmdPyPSA, userID, modelID, sessionID, lkr, callbackURL)
		if err := utils.ExecCMD(cmd); err != nil {
			utils.Log(err)
			http.Error(w, executeCMDErrMsg, http.StatusBadRequest)
			return
		}

		w.Header().Set(contTypeHeader, contTypeAppYAML)
	}
}

// Show gives information about the progress of the simulation
func (p *PyPSA) Show() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		activeSimsJSON, err := json.Marshal(p.activeSimulations)
		p.mu.Unlock()
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
func (p *PyPSA) Log() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		log, err := utils.ReadFile(logFolderPath + "/" + logFilePyPSA)
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
func (p *PyPSA) Finish() http.HandlerFunc {
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

		if err := p.closeSimLog(modelIDStr); err != nil {
			utils.Log(err)
			http.Error(w, modelIDNotExistErrMsg, http.StatusBadRequest)
			return
		}

		p.showLogs()

		userID := utils.ToStr(request.UserID)
		sessionID := utils.ToStr(request.SessionID)
		modelID := modelIDStr
		callbackURL := request.CallbackURL

		folderSim := simFolderPath + "/" + simFolderPrefix + modelID
		folderData := dataFolderPath + "/" + cdFolderPath + "/" + modelID
		zipped := folderSim + simZipPostfix

		fmt.Printf("[PyPSA Finish] Creating zip file: %s from folder: %s\n", zipped, folderSim)
		if err := utils.ZipFolder(folderSim, zipped); err != nil {
			fmt.Printf("[PyPSA Finish] ERROR: Failed to create zip: %v\n", err)
			utils.Log(err)
			http.Error(w, zipFolderErrMsg, http.StatusBadRequest)
			return
		}
		fmt.Printf("[PyPSA Finish] Zip file created successfully\n")

		// Determine PyPSA success status from the request (set by pypsa.sh based on exit code)
		pypsaStatus := "true"
		if request.PyPSA == false || request.PyPSA == "false" {
			pypsaStatus = "false"
		}

		fmt.Printf("[PyPSA Finish] Sending zip to callback URL: %s (pypsa=%s)\n", callbackURL, pypsaStatus)
		fmt.Printf("[PyPSA Finish]   session_id: %s, user_id: %s, model_id: %s\n", sessionID, userID, modelID)
		cfg := config.GetConfig()
		if err := utils.SendZip(callbackURL, sessionID, userID, modelID, pypsaStatus, zipped, cfg.CallbackSecret); err != nil {
			fmt.Printf("[PyPSA Finish] ERROR: Failed to send zip to callback: %v\n", err)
			utils.Log(err)
			http.Error(w, fmt.Sprintf("%s: %v", sendZipErrMsg, err), http.StatusBadGateway)
			return
		}
		fmt.Printf("[PyPSA Finish] Zip file sent successfully to callback URL\n")

		// The platform has already accepted the result at this point.
		// Keep local cleanup best-effort so transient filesystem issues
		// do not turn a successful callback into an apparent failure.
		cleanupTargets := []struct {
			label string
			path  string
		}{
			{label: "simulation folder", path: folderSim},
			{label: "custom demand folder", path: folderData},
			{label: "zip file", path: zipped},
		}

		for _, target := range cleanupTargets {
			if err := utils.RemoveDir(target.path); err != nil {
				fmt.Printf("[PyPSA Finish] WARNING: Failed to remove %s %s: %v\n", target.label, target.path, err)
			}
		}

		// Clean up the simulation entry to prevent memory leak
		if err := p.removeSimLog(modelIDStr); err != nil {
			utils.Log(err)
		}

		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}
