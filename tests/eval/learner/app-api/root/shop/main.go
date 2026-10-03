// Command shop serves an HTTP API of items.
package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("GET /api/v1/items", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]string{"item"})
	})
	log.Fatal(http.ListenAndServe(":8080", nil))
}
