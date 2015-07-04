package main

import (
	"flag"
	. "fmt"
	"fslocate/boyer"
	"fslocate/mboyer"
	"fslocate/postgres"
	"fslocate/sqlite"
	"log"
	"os"
	"runtime/pprof"
	"strings"
)

const DefaultNumIndexers = 3

var numIndexers int
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
	flag.IntVar(&numIndexers, "n", DefaultNumIndexers, "specify num indexers")
	flag.BoolVar(&verbose, "v", false, "verbose")
	flag.BoolVar(&doIndexing, "i", false, "index the config dirs (not search)")
	flag.StringVar(&implType, "t", "mboyer", "type of fslocate: postgres, sqlite, or boyer")
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

	if !doIndexing && numIndexers != DefaultNumIndexers {
		Fprintf(os.Stderr, "ERROR: cannot specify -t without the -i (indexing) flag\n")
		os.Exit(1)
	}

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
	case "sqlite":
		return sqlite.SqliteFsLocate{}
	case "postgresql":
		return postgres.PgFsLocate{}
	}
	panic("No matching type for " + fstype)
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
	Println("     -n NUM : specify number of indexer threads (default=3)")
	Println("     -t TYP : specify type of indexing ('boyer' is default)")
	Println("     -v     : verbose mode")
	Println("     -h     : show help")
}
