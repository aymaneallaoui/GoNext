package main

import (
	"fmt"
	"net/http"
)

func helloHandler(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "text/plain")

	fmt.Fprintf(w, "Hello, World!")
}

func main() {

	http.HandleFunc("/", helloHandler)

	fmt.Println("Server starting on port 5000...")
	err := http.ListenAndServe(":5000", nil)
	if err != nil {
		fmt.Println("Error starting server: ", err)
	}
}
