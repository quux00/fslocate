package main

import (
	"fmt"
	"log"
	"os"
)


// invoke with:
// fslocate search search-term
// fslocate index
func main() {
	checkArgs()

	switch os.Args[1] {
	case "search":
		Search(os.Args[2])
	case "index":
		Index(os.Args[2:])
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
	switch os.Args[1] {
	case "search":
		if len(os.Args) != 3 { log.Fatal("ERROR: No search term provided") }
	case "index":
		fmt.Print("")  // TODO: I don't know how to "do nothing" in a case stmt
	default:
		fmt.Printf("ERROR: term '%s' not recognized\n", os.Args[1])
		help()
		os.Exit(-1)
	}
}


func help() {
	println("Usage: [-h] fslocate search|index")
	println("  fslocate search <search-term>")
	println("  fslocate index")
}
