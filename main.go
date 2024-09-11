package main

import (
	"log"
	"os"

	"github.com/aymaneallaoui/GoNext/cmd"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	if err := cmd.RootCmd.Execute(); err != nil {
		log.Fatalf("Error executing the command: %v", err)
		os.Exit(1)
	}
}
