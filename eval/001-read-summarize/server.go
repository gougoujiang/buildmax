// Package main is a minimal HTTP server that serves static files.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	dir := flag.String("dir", ".", "directory to serve")
	flag.Parse()

	if _, err := os.Stat(*dir); err != nil {
		fmt.Fprintf(os.Stderr, "directory %q does not exist\n", *dir)
		os.Exit(1)
	}

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("serving %s on %s", *dir, addr)
	if err := http.ListenAndServe(addr, http.FileServer(http.Dir(*dir))); err != nil {
		log.Fatal(err)
	}
}
