package pypsa

// Topology holds the topology data
type Topology []Connection

// Connection holds the link data
type Connection struct {
	From   Feature `json:"from" yaml:"from"`
	To     Feature `json:"to" yaml:"to"`
	Length float64 `json:"length" yaml:"length"`
	Pipe   string  `json:"pipe" yaml:"pipe"`
}

// Compare compares two connections
func (c Connection) Compare(other Connection) bool {
	return c.From.Compare(other.From) &&
		c.To.Compare(other.To) &&
		c.Length == other.Length &&
		c.Pipe == other.Pipe
}

// Feature holds the feature data
type Feature struct {
	ID         string     `json:"id" yaml:"id"`
	Type       string     `json:"type" yaml:"type"`
	Geometry   Geometry   `json:"geometry" yaml:"geometry"`
	Properties Properties `json:"properties" yaml:"properties"`
	Techs      Techs      `json:"techs" yaml:"techs"`
}

// Compare compares two features
func (f Feature) Compare(other Feature) bool {
	return f.ID == other.ID &&
		f.Type == other.Type &&
		f.Geometry.Compare(other.Geometry) &&
		f.Properties.Compare(other.Properties) &&
		f.Techs.Compare(other.Techs)
}

// Techs hold the active techs
type Techs map[string]ContAndCosts

// Compare compares two techs
func (t Techs) Compare(other Techs) bool {
	techOk := true
	techOk = techOk && len(t) == len(other)

	for key, tVal := range t {
		if oVal, ok := other[key]; ok {
			techOk = techOk && tVal.Compare(oVal)
		}
	}

	return techOk
}

// ConstAndCosts holds the constrains and consts for a tech
type ContAndCosts map[string]interface{}

// Compare compares two constraints and costs
func (c ContAndCosts) Compare(other ContAndCosts) bool {
	contAndCostsOk := true
	contAndCostsOk = contAndCostsOk && len(c) == len(other)

	for key, tVal := range c {
		oVal, ok := other[key]
		contAndCostsOk = contAndCostsOk && ok && compareTypesAndVals(tVal, oVal)
	}

	return contAndCostsOk
}

func compareTypesAndVals(tVal interface{}, oVal interface{}) bool {
	valsOk := true

	switch tVal := tVal.(type) {
	case int:
		oVal, ok := oVal.(int)
		valsOk = valsOk && ok && tVal == oVal
	case float64:
		oVal, ok := oVal.(float64)
		valsOk = valsOk && ok && tVal == oVal
	case string:
		oVal, ok := oVal.(string)
		valsOk = valsOk && ok && tVal == oVal
	default:
		valsOk = false
	}

	return valsOk
}

// Geometry holds the geometry data
type Geometry struct {
	Type        string `json:"type" yaml:"type"`
	Coordinates Coors  `json:"coordinates" yaml:"coordinates"`
}

// Compare compares two geometries
func (g Geometry) Compare(other Geometry) bool {
	return g.Type == other.Type &&
		g.Coordinates.Compare(other.Coordinates)
}

// Coors holds the coordinate data as a float64 array
type Coors []float64

func (c Coors) Compare(other Coors) bool {
	coorsOk := true
	coorsOk = coorsOk && len(c) == len(other)

	for i := 0; i < len(other); i++ {
		coorsOk = coorsOk && c[i] == other[i]
	}

	return coorsOk
}

// Properties holds the property data
type Properties struct {
	OSMID        string  `json:"osm_id" yaml:"osm_id"`
	Area         float64 `json:"area" yaml:"area"`
	HexID        string  `json:"hex_id" yaml:"hex_id"`
	Class        string  `json:"class" yaml:"class"`
	Demand       int64   `json:"demand" yaml:"demand"`
	DemandEnergy int64   `json:"demand_energy" yaml:"demand_energy"`
	DemandGas    int64   `json:"demand_gas" yaml:"demand_gas"`
	DemandHeat   int64   `json:"demand_heat" yaml:"demand_heat"`
}

// Compare compares two properties
func (p Properties) Compare(other Properties) bool {
	return p.OSMID == other.OSMID &&
		p.Area == other.Area &&
		p.HexID == other.HexID &&
		p.Class == other.Class &&
		p.Demand == other.Demand &&
		p.DemandEnergy == other.DemandEnergy &&
		p.DemandGas == other.DemandGas &&
		p.DemandHeat == other.DemandHeat
}
