package simulations

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"

	"webservice/config"
	"webservice/services"
	"webservice/utils"
)

const (
	pathIgnis    = "/ignis"
	logFileIgnis = "ignis.log"
	ignisAPICalc = "/api/v1/calculate"
	ignisMaxConc = 8 // Ignis is pure in-memory arithmetic — no I/O bound
)

var (
	ignisSemaphore     chan struct{}
	ignisSemaphoreOnce sync.Once
)

func getIgnisSemaphore() chan struct{} {
	ignisSemaphoreOnce.Do(func() {
		ignisSemaphore = make(chan struct{}, ignisMaxConc)
	})
	return ignisSemaphore
}

// ignisTask is a single building to calculate.
type ignisTask struct {
	nodeID      string
	variantCode string
	aRef        *float64 // optional floor area override (m²)
}

// ignisOutcome is the result of one Ignis run.
type ignisOutcome struct {
	nodeID string
	qHND   float64 // kWh/(m²·a) annual heating demand
	errMsg string
}

// Ignis fans out heating demand estimation requests to the Ignis service and
// merges the annual q_h_nd result back into each building node's properties.ignis.
type Ignis struct {
	mu                sync.Mutex
	activeSimulations map[string]simLogCalliope
}

// NewIgnis creates a new Ignis handler.
func NewIgnis() *Ignis {
	return &Ignis{activeSimulations: make(map[string]simLogCalliope)}
}

// Path returns the URL base path for this simulation.
func (h *Ignis) Path() string {
	return pathIgnis
}

// Configure is a no-op stub satisfying the Simulation interface.
func (h *Ignis) Configure() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}

// Generate is a no-op stub satisfying the Simulation interface.
func (h *Ignis) Generate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}

// Start receives an EnerPlanET config.json, extracts building nodes that carry
// an "ignis" block with a "variant_code" field, sends each building to the Ignis
// service concurrently, and returns the original config enriched with
// properties.ignis.q_h_nd for each processed node.
func (h *Ignis) Start() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			utils.Log(err)
			http.Error(w, readBodyErrMsg, http.StatusBadRequest)
			return
		}

		var rawConfig map[string]json.RawMessage
		if err := json.Unmarshal(body, &rawConfig); err != nil {
			utils.Log(err)
			http.Error(w, readRequestDataErrMsg, http.StatusBadRequest)
			return
		}

		tasks, err := extractIgnisTasks(rawConfig["topology"])
		if err != nil {
			utils.Log(err)
			http.Error(w, "failed to parse topology", http.StatusBadRequest)
			return
		}

		if len(tasks) == 0 {
			// Nothing to process — return the config unchanged.
			w.Header().Set(contTypeHeader, contTypeAppJSON)
			w.Write(body)
			return
		}

		sem := getIgnisSemaphore()
		ch := make(chan ignisOutcome, len(tasks))
		var wg sync.WaitGroup

		for _, task := range tasks {
			wg.Add(1)
			go func(t ignisTask) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				qHND, err := callIgnisService(t)
				if err != nil {
					ch <- ignisOutcome{nodeID: t.nodeID, errMsg: err.Error()}
				} else {
					ch <- ignisOutcome{nodeID: t.nodeID, qHND: qHND}
				}
			}(task)
		}

		wg.Wait()
		close(ch)

		results := make(map[string]ignisOutcome, len(tasks))
		failed := 0
		for o := range ch {
			results[o.nodeID] = o
			if o.errMsg != "" {
				failed++
				log.Printf("Ignis | node=%s error: %s", o.nodeID, o.errMsg)
			}
		}

		enrichedTopology, err := mergeIgnisIntoTopology(rawConfig["topology"], results)
		if err != nil {
			utils.Log(err)
			http.Error(w, "failed to merge Ignis results into topology", http.StatusInternalServerError)
			return
		}
		rawConfig["topology"] = enrichedTopology

		log.Printf("Ignis | processed=%d failed=%d", len(tasks)-failed, failed)
		w.Header().Set(contTypeHeader, contTypeAppJSON)
		json.NewEncoder(w).Encode(rawConfig)
	}
}

// extractIgnisTasks walks the topology edges and collects one task per unique
// BasePOI node that carries an ignis block with a non-empty variant_code.
func extractIgnisTasks(rawTopology json.RawMessage) ([]ignisTask, error) {
	var rawEdges []json.RawMessage
	if err := json.Unmarshal(rawTopology, &rawEdges); err != nil {
		return nil, fmt.Errorf("parse topology edges: %w", err)
	}

	var tasks []ignisTask
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
			task, ok := ignisNodeToTask(rawNode)
			if !ok || seen[task.nodeID] {
				continue
			}
			seen[task.nodeID] = true
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

// ignisNodeToTask extracts the Ignis task from a topology node.
// Returns false if the node is not a BasePOI or has no valid ignis.variant_code.
func ignisNodeToTask(rawNode json.RawMessage) (ignisTask, bool) {
	var node struct {
		ID         string `json:"id"`
		Properties struct {
			FeatureType string `json:"feature_type"`
			Ignis       struct {
				VariantCode string   `json:"variant_code"`
				ARef        *float64 `json:"A_ref"`
			} `json:"ignis"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(rawNode, &node); err != nil {
		return ignisTask{}, false
	}
	if node.Properties.FeatureType != "BasePOI" {
		return ignisTask{}, false
	}
	if node.Properties.Ignis.VariantCode == "" {
		return ignisTask{}, false
	}
	return ignisTask{
		nodeID:      node.ID,
		variantCode: node.Properties.Ignis.VariantCode,
		aRef:        node.Properties.Ignis.ARef,
	}, true
}

// callIgnisService POSTs to Ignis and returns q_h_nd.
func callIgnisService(task ignisTask) (float64, error) {
	cfg := config.GetConfig()
	url := cfg.IgnisService.URL(fmt.Sprintf("%s/%s", ignisAPICalc, task.variantCode))
	client := services.NewResilientHTTPClient()

	// Always send a JSON object so the handler receives a parseable body.
	// When no override is needed, send an empty object {}.
	body := map[string]interface{}{}
	if task.aRef != nil {
		body["A_ref"] = *task.aRef
	}

	var result struct {
		QhNd float64 `json:"q_h_nd"`
	}
	if err := client.PostJSONAndDecode(url, body, &result); err != nil {
		return 0, fmt.Errorf("Ignis request: %w", err)
	}

	log.Printf("Ignis | node=%s variant=%s q_h_nd=%.2f kWh/(m2.a)", task.nodeID, task.variantCode, result.QhNd)
	return result.QhNd, nil
}

// mergeIgnisIntoTopology writes q_h_nd into each matching topology node's
// properties.ignis block, leaving all other fields unchanged.
func mergeIgnisIntoTopology(rawTopology json.RawMessage, results map[string]ignisOutcome) (json.RawMessage, error) {
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
			nodeID := fmt.Sprintf("%v", propsMap["id"])
			outcome, found := results[nodeID]
			if !found || outcome.errMsg != "" {
				continue
			}
			// Merge q_h_nd into the existing ignis block.
			ignis, _ := propsMap["ignis"].(map[string]interface{})
			if ignis == nil {
				ignis = make(map[string]interface{})
			}
			ignis["q_h_nd"] = outcome.qHND
			ignis["q_h_nd_unit"] = "kWh/(m2.a)"
			propsMap["ignis"] = ignis
		}
	}

	return json.Marshal(edges)
}

// Show returns a JSON object listing active Ignis simulations.
func (h *Ignis) Show() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		w.Header().Set(contTypeHeader, contTypeAppJSON)
		json.NewEncoder(w).Encode(h.activeSimulations)
	}
}

// Log returns the contents of the Ignis log file.
func (h *Ignis) Log() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		content, err := utils.ReadFile(logFolderPath + "/" + logFileIgnis)
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
func (h *Ignis) Finish() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contTypeHeader, contTypeAppJSON)
	}
}
