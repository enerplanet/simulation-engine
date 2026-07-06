package main

import (
	"fmt"
	"log"
	"net/http"
	"webservice/utils"

	"github.com/subosito/gotenv"
)

var config utils.Config

func init() {
	gotenv.Load()
	config.Load()

	// Initialize column mapper with demand CSV files
	log.Println("Initializing column mapper for demand CSV files...")
	columnMapper := utils.GetColumnMapper()
	columnMapper.InitializeColumnMappings("data")
	log.Println("Column mapper initialized successfully")
}

func main() {
	fmt.Println(config)
	log.Fatal(http.ListenAndServe(config.Address(), getRouter()))
}
