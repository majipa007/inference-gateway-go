package main

import (
	"log"
	"net/http"

	"inference-gateway-go/handlers"

	"github.com/joho/godotenv"
)

func main() {
	// Set log flags so logs include date and time.
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, using system environment")
	}

	http.HandleFunc("/predict", handlers.Predict)
	http.HandleFunc("/health", handlers.Health)

	log.Println("INFO: listening on localhost:8080")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("ERROR: server failed to start: %v", err)
	}
}
