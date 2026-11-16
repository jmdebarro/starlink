package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/jmdebarro/starlink/internal/tle"
)

// dataFile is where the downloaded TLEs are saved, relative to where you run
// the program (run it from the project root: go run ./cmd/starlink).
const dataFile = "data/starlink.tle"

func main() {
	data, source, err := tle.Load(dataFile)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	fmt.Println("Source:", source, "("+dataFile+")")
	fmt.Println("Satellites:", len(lines)/3)

	for _, line := range lines[:min(6, len(lines))] {
		fmt.Println(line)
	}
}
