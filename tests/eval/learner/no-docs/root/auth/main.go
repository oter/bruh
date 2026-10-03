// Command auth signs in users.
package main

import (
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("POST /login", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	log.Fatal(http.ListenAndServe(":8081", nil))
}
