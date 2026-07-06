package main

import (
	"log"
	"net/http"
	"webservice/config"
	"webservice/utils"

	"github.com/subosito/gotenv"
)

func init() {
	gotenv.Load()

	// Initialize column mapper with demand CSV files
	log.Println("Initializing column mapper for demand CSV files...")
	columnMapper := utils.GetColumnMapper()
	columnMapper.InitializeColumnMappings("data")
	log.Println("Column mapper initialized successfully")
}

func main() {
	cfg := config.GetConfig()
	log.Printf("Starting webservice on %s", cfg.ServerAddr())
	log.Fatal(http.ListenAndServe(cfg.ServerAddr(), getRouter()))
}
