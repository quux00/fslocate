package main

import (
	"fmt"
	// "log"
	"os"
)


// invoke with:
// fslocate search-term
// fslocate index
func main() {
	checkArgs()

	switch os.Args[1] {
	case "-i":
		Index(os.Args[2:])
	default:
		Search(os.Args[1])
	}
}

func checkArgs()  {
	if len(os.Args) < 2 {
		fmt.Println("ERROR: no command line args provided")
		help()
		os.Exit(-1)
	}
	if os.Args[1] == "-h" {
		help()
		os.Exit(0)
	}
}


func help() {
	println("Usage: [-h] fslocate search-term|-i")
	println("  fslocate <search-term>")
	println("  fslocate -i  (run the indexer)")
}
