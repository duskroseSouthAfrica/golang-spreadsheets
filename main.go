// main.go
package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	// Serve static files
	fs := http.FileServer(http.Dir("./"))
	http.Handle("/upload.css", http.StripPrefix("/", fs))
	http.Handle("/display.css", http.StripPrefix("/", fs))
	http.Handle("/results.css", http.StripPrefix("/", fs))
	http.Handle("/duskrose.woff2", http.StripPrefix("/", fs))

	// App endpoints
	http.HandleFunc("/", uploadHandler)
	http.HandleFunc("/display", displayHandler)
	http.HandleFunc("/calculate", calculateHandler)
	http.HandleFunc("/api/validate", validateFileHandler)
	http.HandleFunc("/health", healthHandler)

	fmt.Println("🚀 Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}