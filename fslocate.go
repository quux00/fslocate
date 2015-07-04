package main

import (
	"flag"
	. "fmt"
	"fslocate/boyer"
	"log"
	"os"
	"runtime/pprof"
	"strings"
)

var verbose bool
var doIndexing bool
var implType string = "boyer"
var cpuprofile string

//
// FsLocate defines the interface that all implementations
// must provide to the fslocate program.
//
type FsLocate interface {
	Search(s string)
	Index(numIndexes int, verbose bool)
}

func init() {
	flag.BoolVar(&verbose, "v", false, "verbose")
	flag.BoolVar(&doIndexing, "i", false, "index the config dirs (not search)")
	flag.StringVar(&cpuprofile, "cpuprofile", "", "write cpu profile to file")
}

//
// To search existing db, invoke with:
//   fslocate search-term
//
// To rebuild db:
//   fslocate -i
//
// To see full usage, see the help function.
//
func main() {
	checkArgs()
	flag.Parse()

	fslocate := getImpl(implType)

	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	if doIndexing {
		fslocate.Index(1, verbose)
	} else {
		fslocate.Search(getSearchTerm(os.Args[1:]))
	}
}

func getImpl(fstype string) FsLocate {
	return boyer.BoyerFsLocate{}
}

func getSearchTerm(args []string) string {
	nonflagArgs := removeFlags(args)
	if len(nonflagArgs) == 0 {
		Fprintln(os.Stderr, "ERROR: No search term provided")
		os.Exit(1)
	}
	return nonflagArgs[len(nonflagArgs)-1]
}

func removeFlags(args []string) []string {
	var nonflags []string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			nonflags = append(nonflags, arg)
		}
	}
	return nonflags
}

func checkArgs() {
	if len(os.Args) < 2 {
		Println("ERROR: no command line args provided")
		help()
		os.Exit(-1)
	}
	if os.Args[1] == "-h" {
		help()
		os.Exit(0)
	}
}

func help() {
	Println("Usage: [-hv] fslocate search-term | -i")
	Println("  fslocate <search-term>")
	Println("  fslocate -i  (run the indexer)")
	Println("     -v     : verbose mode")
	Println("     -h     : show help")
}
