package main

import (
	"log"
	"net/http"

	"inference-gateway-go/routes"
)

func main() {
	// Set log flags so logs include date and time.
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	log.Println("INFO: listening on localhost:8080")

	mux := routes.Register()

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("ERROR: server failed to start: %v", err)
	}
}
