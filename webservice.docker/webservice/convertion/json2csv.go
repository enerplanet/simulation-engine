package convertion

import (
	"fmt"
	"strings"
	"time"
	csv2json "webservice/models/csv2json"
)

const (
	seperator = ","
	lineBreak = "\n"
)

func JSON2CSV(json csv2json.Response) (string, error) {
	startTime := json.StartTime
	deltaTime := json.DeltaTime
	endTime := json.EndTime
	values := json.Values
	width := json.Width
	length := json.Length

	timeStamps, err := getTimeStamps(startTime, deltaTime, endTime, length)
	if err != nil {
		return "", err
	}
	header := getHeader(json.Values)
	valueRows := getValueRows(timeStamps, length, values)
	if len(header) != int(width) || len(valueRows) != int(length)-1 ||
		len(valueRows[0]) != int(width)-1 || len(timeStamps) != int(length)-1 {
		return "", fmt.Errorf("rows and columns do not match!")
	}
	CSVString := getCSVString(timeStamps, header, valueRows)
	return CSVString, nil
}

func getCSVString(timeStamps []string, header []string, rows [][]string) string {
	CSVString := ""
	for colNr, col := range header {
		if colNr == 0 {
			CSVString += col
		} else {
			CSVString += seperator + col
		}
	}
	CSVString += lineBreak
	for rowNr, row := range rows {
		CSVString += timeStamps[rowNr]
		for _, col := range row {
			CSVString += seperator + col
		}
		CSVString += lineBreak
	}
	return CSVString
}

func getTimeStamps(startTime string, deltaTime int64, endTime string, length int64) ([]string, error) {
	start, err := time.Parse("2006-01-02 15:04", startTime)
	if err != nil {
		return nil, err
	}
	timeStamps := []string{}
	var rowNr int64
	for rowNr = 0; rowNr < length-1; rowNr++ {
		current := start.Add(time.Duration(deltaTime*rowNr) * time.Minute)
		timeStamps = append(timeStamps, current.Format("2006-01-02 15:04"))
	}
	if timeStamps[len(timeStamps)-1] != endTime {
		return nil, fmt.Errorf("Last timestamp does not match!")
	}
	return timeStamps, nil
}

func getHeader(values csv2json.Values) []string {
	header := []string{"time"}
	for key := range values {
		header = append(header, strings.ToLower(key))
	}
	return header
}

func getValueRows(timeStamps []string, length int64, values csv2json.Values) [][]string {
	rows := [][]string{}
	var rowNr int64
	for rowNr = 0; rowNr < length-1; rowNr++ {
		rows = append(rows, getValueRow(rowNr, values))
	}
	return rows
}

func getValueRow(rowNr int64, values csv2json.Values) []string {
	row := []string{}
	for key := range values {
		row = append(row, fmt.Sprintf("%f", values[key][rowNr]))
	}
	return row
}
