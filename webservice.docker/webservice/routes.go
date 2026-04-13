package main

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"webservice/simulations"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

const (
	pathConfigure = "/configure"
	pathGenerate  = "/generate"
	pathStart     = "/start"
	pathShow      = "/show"
	pathLog       = "/log"
	pathFinish    = "/finish"
	pathHealth    = "/health"
	pathStatus    = "/status"
)

const (
	originsCORS     = "*"
	xReqWithCORS    = "X-Requested-With"
	contentTypeCORS = "Content-Type"
	originCORS      = "Origin"
	acceptCORS      = "Accept"
	tokenCORS       = "Token"
)

func getRouter() *mux.Router {
	router := mux.NewRouter()

	setupHandleFunc(router)
	setupCORS(router)

	return router
}

func setupHandleFunc(router *mux.Router) {
	// Health check endpoint for service discovery
	router.HandleFunc(pathHealth, healthCheck()).Methods(http.MethodGet)
	router.HandleFunc(pathStatus, statusCheck()).Methods(http.MethodGet)

	simulations := []simulations.Simulation{
		simulations.NewCalliope(),
		simulations.NewPyPSA(),
		simulations.NewCSV2JSON(),
		simulations.NewCharging(),
	}

	for _, sim := range simulations {
		router.HandleFunc(sim.Path()+pathConfigure, sim.Configure()).Methods(http.MethodPost)
		router.HandleFunc(sim.Path()+pathGenerate, sim.Generate()).Methods(http.MethodPost)
		router.HandleFunc(sim.Path()+pathStart, sim.Start()).Methods(http.MethodPost)
		router.HandleFunc(sim.Path()+pathShow, sim.Show()).Methods(http.MethodGet)
		router.HandleFunc(sim.Path()+pathLog, sim.Log()).Methods(http.MethodGet)
		router.HandleFunc(sim.Path()+pathFinish, sim.Finish()).Methods(http.MethodPost)
	}
}

// getSystemMetrics returns CPU and memory usage percentages
func getSystemMetrics() (cpuUsage float64, memoryUsage float64) {
	// Read actual system memory from /proc/meminfo
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var totalKB, availableKB uint64
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			switch {
			case strings.HasPrefix(line, "MemTotal:"):
				if val, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
					totalKB = val
				}
			case strings.HasPrefix(line, "MemAvailable:"):
				if val, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
					availableKB = val
				}
			}
		}
		if totalKB > 0 && availableKB > 0 {
			memoryUsage = float64(totalKB-availableKB) / float64(totalKB) * 100
		}
	}

	// Read CPU usage from /proc/stat
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 1 {
			if load, err := strconv.ParseFloat(fields[0], 64); err == nil {
				numCPU := float64(runtime.NumCPU())
				cpuUsage = (load / numCPU) * 100
				if cpuUsage > 100 {
					cpuUsage = 100
				}
			}
		}
	}

	return cpuUsage, memoryUsage
}

func healthCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cpuUsage, memoryUsage := getSystemMetrics()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":       "healthy",
			"service":      "simulation-engine",
			"cpu_usage":    cpuUsage,
			"memory_usage": memoryUsage,
		})
	}
}

func statusCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cpuUsage, memoryUsage := getSystemMetrics()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Return status compatible with platform-core check
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":       "available",
			"jobs":         []interface{}{},
			"active":       true,
			"cpu_usage":    cpuUsage,
			"memory_usage": memoryUsage,
		})
	}
}

func setupCORS(router *mux.Router) {
	origins := []string{originsCORS}
	headers := []string{xReqWithCORS, contentTypeCORS, originCORS, acceptCORS, tokenCORS}
	methods := []string{http.MethodGet, http.MethodPost, http.MethodOptions}

	cors := handlers.CORS(
		handlers.AllowedOrigins(origins),
		handlers.AllowedHeaders(headers),
		handlers.AllowedMethods(methods),
		handlers.AllowCredentials(),
	)

	router.Use(cors)
}
