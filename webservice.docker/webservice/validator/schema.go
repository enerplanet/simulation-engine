package validator

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/xeipuuv/gojsonschema"
)

const (
	schemaFolderPath  = "schemas"
	schemaFilePostfix = ".json"
)

const (
	templStrResultHeader      = "# ---- Schema Validation Results ----"
	templStrValidJSONString   = "| The JSON string is %s\n"
	templStrResultErrorString = "|\t %000d: %s\n"
	templStrResultFooter      = "# --- End ---"
	templStrValid             = "is valid."
	templStrInvalid           = "is not valid. See errors:"
)

func loadJSONSchema(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	byteValue, err := ioutil.ReadAll(file)
	if err != nil {
		return "", err
	}

	return string(byteValue), nil
}

// ValidateJSON validates the JSON string against the JSON validator
func ValidateJSON(JSON string, pathSim string) (*gojsonschema.Result, error) {
	path := schemaFolderPath + pathSim + schemaFilePostfix

	schemaString, err := loadJSONSchema(path)
	if err != nil {
		return nil, err
	}

	schemaLoader := gojsonschema.NewStringLoader(schemaString)
	JSONLoader := gojsonschema.NewStringLoader(JSON)

	return gojsonschema.Validate(schemaLoader, JSONLoader)

}

// IsValid checks whether the results are valid
func IsValid(result *gojsonschema.Result) bool {
	return result != nil && result.Valid()
}

func printTempalte(result *gojsonschema.Result, valid string) string {
	resultString := fmt.Sprintln(templStrResultHeader)

	resultString += fmt.Sprintf(templStrValidJSONString, valid)
	for nr, desc := range result.Errors() {
		resultString += fmt.Sprintf(templStrResultErrorString, nr, desc)
	}

	resultString += fmt.Sprintln(templStrResultFooter)

	return resultString
}

// PrintFormatResults prints the results in a predefined format
func PrintFormatResults(result *gojsonschema.Result) string {
	if IsValid(result) {
		return printTempalte(result, templStrValid)
	}

	return printTempalte(result, templStrInvalid)
}
