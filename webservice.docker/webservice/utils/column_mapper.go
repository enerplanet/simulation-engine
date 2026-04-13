package utils

import (
	"encoding/json"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
)

// DemandIndex represents the JSON index file structure
type DemandIndex struct {
	Version       string                     `json:"version"`
	Format        string                     `json:"format"`
	BuildingTypes map[string]BuildingProfile `json:"building_types"`
}

// BuildingProfile contains demand level info for a building type
type BuildingProfile struct {
	File         string  `json:"file"`
	BDEWProfile  string  `json:"bdew_profile"`
	DemandLevels []int64 `json:"demand_levels"`
	Hours        int     `json:"hours"`
}

// ColumnMapper holds the available demand levels for each building type
type ColumnMapper struct {
	demandLevels map[string][]int64 // maps building type to sorted list of demand levels
	files        map[string]string  // maps building type to NetCDF filename
	format       string             // "netcdf4" or "csv"
	mu           sync.RWMutex
}

// Prefer canonical singular keys for some f_class values even if plural entries exist.
// This keeps profile file selection stable when duplicate/generated aliases are present.
var preferredDemandLookupAliases = map[string]string{
	"apartments": "apartment",
}

func normalizeBuildingTypeKey(buildingType string) string {
	return strings.ToLower(strings.TrimSpace(buildingType))
}

func demandLookupCandidates(buildingType string) []string {
	key := normalizeBuildingTypeKey(buildingType)
	if key == "" {
		return nil
	}

	candidates := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	add := func(candidate string) {
		candidate = normalizeBuildingTypeKey(candidate)
		if candidate == "" {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}

	if alias, ok := preferredDemandLookupAliases[key]; ok {
		add(alias)
	}
	add(key)

	// Generic singular fallbacks only after exact match to avoid surprising overrides.
	if strings.HasSuffix(key, "ies") && len(key) > 3 {
		add(strings.TrimSuffix(key, "ies") + "y")
	}
	if strings.HasSuffix(key, "s") && len(key) > 1 {
		add(strings.TrimSuffix(key, "s"))
	}

	return candidates
}

var (
	columnMapperInstance *ColumnMapper
	columnMapperOnce     sync.Once
)

// GetColumnMapper returns the singleton ColumnMapper instance
func GetColumnMapper() *ColumnMapper {
	columnMapperOnce.Do(func() {
		columnMapperInstance = &ColumnMapper{
			demandLevels: make(map[string][]int64),
			files:        make(map[string]string),
			format:       "netcdf4",
		}
	})
	return columnMapperInstance
}

// LoadFromIndex loads demand levels from the JSON index file
func (cm *ColumnMapper) LoadFromIndex(indexPath string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	file, err := os.Open(indexPath)
	if err != nil {
		return err
	}
	defer file.Close()

	var index DemandIndex
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&index); err != nil {
		return err
	}

	cm.format = index.Format
	cm.demandLevels = make(map[string][]int64)
	cm.files = make(map[string]string)

	for buildingType, profile := range index.BuildingTypes {
		key := normalizeBuildingTypeKey(buildingType)
		if key == "" {
			continue
		}

		// Ensure demand levels are sorted
		levels := make([]int64, len(profile.DemandLevels))
		copy(levels, profile.DemandLevels)
		sort.Slice(levels, func(i, j int) bool {
			return levels[i] < levels[j]
		})

		cm.demandLevels[key] = levels
		cm.files[key] = profile.File
		log.Printf("Loaded %d demand levels for %s from %s", len(levels), key, profile.File)
	}

	log.Printf("Demand index loaded: format=%s, building_types=%d", cm.format, len(cm.demandLevels))
	return nil
}

// FindNearestDemandLevel finds the nearest available demand level for a given demand value
func (cm *ColumnMapper) FindNearestDemandLevel(buildingType string, demand int64) int64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	key := normalizeBuildingTypeKey(buildingType)
	levels, exists := cm.demandLevels[key]
	if !exists || len(levels) == 0 {
		// If building type not loaded, return original demand
		return demand
	}

	// Handle edge cases
	if demand <= levels[0] {
		return levels[0]
	}
	if demand >= levels[len(levels)-1] {
		return levels[len(levels)-1]
	}

	// Binary search to find the nearest level
	idx := sort.Search(len(levels), func(i int) bool {
		return levels[i] >= demand
	})

	// Check which is closer: levels[idx-1] or levels[idx]
	if idx == 0 {
		return levels[0]
	}
	if idx == len(levels) {
		return levels[len(levels)-1]
	}

	// Compare distances to find nearest
	lowerDiff := demand - levels[idx-1]
	upperDiff := levels[idx] - demand

	if lowerDiff <= upperDiff {
		return levels[idx-1]
	}
	return levels[idx]
}

// FindNearestColumn is an alias for FindNearestDemandLevel (for backward compatibility)
func (cm *ColumnMapper) FindNearestColumn(buildingType string, demand int64) int64 {
	return cm.FindNearestDemandLevel(buildingType, demand)
}

// GetFile returns the NetCDF filename for a building type
func (cm *ColumnMapper) GetFile(buildingType string) string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.files[normalizeBuildingTypeKey(buildingType)]
}

// GetFormat returns the data format (netcdf4 or csv)
func (cm *ColumnMapper) GetFormat() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.format
}

// HasExactDemandLevel checks if an exact demand level exists
func (cm *ColumnMapper) HasExactDemandLevel(buildingType string, demand int64) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	key := normalizeBuildingTypeKey(buildingType)
	levels, exists := cm.demandLevels[key]
	if !exists || len(levels) == 0 {
		return false
	}

	idx := sort.Search(len(levels), func(i int) bool {
		return levels[i] >= demand
	})

	return idx < len(levels) && levels[idx] == demand
}

// HasExactColumn is an alias for HasExactDemandLevel (for backward compatibility)
func (cm *ColumnMapper) HasExactColumn(buildingType string, demand int64) bool {
	return cm.HasExactDemandLevel(buildingType, demand)
}

// HasBuildingType checks whether the index contains a demand mapping for the given key.
func (cm *ColumnMapper) HasBuildingType(buildingType string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	_, exists := cm.demandLevels[normalizeBuildingTypeKey(buildingType)]
	return exists
}

// ResolveDemandLookupKey returns the best available lookup key for a profile/f_class.
// It prefers canonical aliases (e.g. apartments -> apartment) and then falls back to
// exact or singularized keys if available in the loaded demand index.
func (cm *ColumnMapper) ResolveDemandLookupKey(buildingType string) string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, candidate := range demandLookupCandidates(buildingType) {
		if _, ok := cm.demandLevels[candidate]; ok {
			return candidate
		}
	}

	return normalizeBuildingTypeKey(buildingType)
}

// InitializeColumnMappings loads demand levels from the index file
func (cm *ColumnMapper) InitializeColumnMappings(dataPath string) {
	indexPath := dataPath + "/demand_index.json"

	if err := cm.LoadFromIndex(indexPath); err != nil {
		log.Printf("Warning: Could not load demand index from %s: %v", indexPath, err)
		log.Printf("Demand profile lookup may not work correctly")
	}
}
