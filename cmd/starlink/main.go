package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

func main() {
	url := "https://celestrak.org/NORAD/elements/gp.php?GROUP=starlink&FORMAT=tle"

	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("request failed:", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("reading body failed:", err)
		return
	}

	lines := strings.Split(string(body), "\n")
	fmt.Println("Status:", resp.Status)
	fmt.Println("Satellites:", len(lines)/3)

	for _, line := range lines[:min(6, len(lines))] {
		fmt.Println(line)
	}
}
