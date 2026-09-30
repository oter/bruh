package main

import (
	"log"
	"os"
)

func main() {
	srv := NewServer("bruh", "0.1.0-dev", AllTools())
	if err := srv.Serve(EnvFromOS(), os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
