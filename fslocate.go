package main

import (
	"flag"
	. "fmt"
	"os"
	"strings"
	"fslocate/boyer"
	"fslocate/mboyer"
	"fslocate/postgres"
)

const DEFAULT_NUM_INDEXERS = 3

var numIndexers int
var verbose bool
var doIndexing bool
var implType string = "mboyer"

type FsLocate interface {
	Search(s string)
	Index(numIndexes int, verbose bool)
}

func init() {
	flag.IntVar(&numIndexers, "n", DEFAULT_NUM_INDEXERS, "specify num indexers")
	flag.BoolVar(&verbose, "v", false, "verbose")
	flag.BoolVar(&doIndexing, "i", false, "index the config dirs (not search)")
	flag.StringVar(&implType, "t", "boyer", "type of fslocate: postgres, or boyer")
}

//
// invoke with:
// fslocate search-term
// fslocate index
// 
func main() {
	checkArgs()
	flag.Parse()

	if !doIndexing && numIndexers != DEFAULT_NUM_INDEXERS {
		Fprintf(os.Stderr, "ERROR: cannot specify -t without the -i (indexing) flag\n")
		os.Exit(1)
	}

	fslocate := getImpl(implType)
	
	if doIndexing {
		fslocate.Index(numIndexers, verbose)
	} else {
		fslocate.Search(getSearchTerm(os.Args[1:]))
	}
}

func getImpl(fstype string) FsLocate {
	switch fstype {
	case "boyer":
		return boyer.BoyerFsLocate{}
	case "mboyer":
		return mboyer.MBoyerFsLocate{}
	case "postgresql":
		return postgres.PgFsLocate{}
	}
	panic("No matching type for " + fstype)
	return nil
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
	Println("Usage: [-hv] [-t NUM] fslocate search-term | -i")
	Println("  fslocate <search-term>")
	Println("  fslocate -i  (run the indexer)")
	Println("     -t NUM : specify number of indexer threads (default=3)")
	Println("     -v     : verbose mode")
	Println("     -h     : show help")
}
