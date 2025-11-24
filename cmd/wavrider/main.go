package main

import (
	"fmt"
	"os"
	"wavrider/internal/decoder"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: wavrider <wav-file>")
		os.Exit(1)
	}

	filename := os.Args[1]
	outfile := "output.bin"
	if len(os.Args) > 2 {
		outfile = os.Args[2]
	}

	fmt.Printf("Processing %s...\n", filename)

	data, err := decoder.Decode(filename)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if len(data) > 0 {
		if err := os.WriteFile(outfile, data, 0644); err != nil {
			fmt.Printf("Error writing output: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Decoded %d bytes. Written to %s\n", len(data), outfile)
	} else {
		fmt.Println("No data decoded.")
	}
}
