// TODO: move this to its own subpackage
package main

import (
	"bufio"
	"database/sql"
	"fmt"
	_ "github.com/bmizerany/pq"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var verbose bool
var nindexers int
var db *sql.DB  // safe for concurrent use by multiple goroutines

const (
	DIR_TYPE  = "d"
	FILE_TYPE = "f"
)

// Index needs to be documented
func Index(args []string) {
	parseArgs(args)
	prf("Using %v indexers", nindexers)

	var ignorePatterns StringSet = readInIgnorePatterns()
	var indexDirs []string = readInIndexDirs()

	// DEBUG
	// fmt.Printf("nindexers: %v\n", nindexers)
	// prf("%T: %v\n", args, args)
	fmt.Printf("%v\n", ignorePatterns)
	fmt.Printf("%v\n", indexDirs)
	// END DEBUG

	err := initDb()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Cannot established connection to fslocate db: %v\n", err)
		return
	}
	// TODO: where does this go? -> some shutdown/quit notifcation when the goroutines all finish
	defer db.Close()

	// TODO: what is this for?
	dbHandler()  // TODO: call as own goroutine

	// TODO: put this in a goroutine
	// TODO: split up the indexDirs into nindexers equal sized pieces
	doIndex(indexDirs, ignorePatterns)
}


func initDb() error {
	// TODO: need to parameterize
	var err error
	db, err = sql.Open("postgres", "user=midpeter444 password=jiffylube dbname=fslocate sslmode=disable")
	if err != nil {
		return err
	}
	return nil
}

// filter removes all pathNames where the basename (filepath.Base)
// matches an "ignore" pattern in the ignorePatterns StringSet
// create and returns a new []string; it does not modify the pathNames
// slice passed in
func filter(pathNames []string, ignorePatterns StringSet) []string {
	keepers := make([]string, 0, len(pathNames))
	for _, path := range pathNames {
		basepath := filepath.Base(path)
		if ! ignorePatterns.Contains(basepath) {
			keepers = append(keepers, path)
		}
	}
	return keepers
}

func syncWithDatabase(fileNames []string, dirNames []string) {
	filesToInsert, dirsToInsert, err := findEntriesNotInDb(fileNames, dirNames)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
		return
	}
	insertIntoDb(filesToInsert, FILE_TYPE)
	insertIntoDb(dirsToInsert, DIR_TYPE)
	deleteFromDb() // TODO: how will this work?
}

func findEntriesNotInDb(filePaths []string, dirPaths []string) (filesToInsert []string, dirsToInsert []string, err error) {
	var (
		stmt *sql.Stmt
		count int
	)
	filesToInsert = make([]string, 0, len(filePaths))
	dirsToInsert  = make([]string, 0, len(dirPaths))

	stmt, err = db.Prepare("SELECT count(path) FROM files WHERE path = $1")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: unable to prepare stmt for db query in indexer: %v\n", err)
		return
	}
	defer stmt.Close()

	// TODO: make this its own function => local anon fn?
	for _, path := range filePaths {
		err = stmt.QueryRow(path).Scan(&count)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: unable to query db in indexer: %v\n", err)
			return
		}
		if count == 0 {
			filesToInsert = append(filesToInsert, path)
		}
	}
	
	for _, path := range dirPaths {
		err = stmt.QueryRow(path).Scan(&count)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: unable to query db in indexer: %v\n", err)
			return
		}
		if count == 0 {
			dirsToInsert = append(dirsToInsert, path)
		}
	}
	
	return filesToInsert, dirsToInsert, err
}

func insertIntoDb(pathsToInsert []string, entryType string) error {
	var (
		stmt *sql.Stmt
		res sql.Result
		err error
	)

	stmt, err = db.Prepare("INSERT INTO files(path, type) values($1, $2)")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: unable to prepare insert stmt: %v\n", err)
		return err
	}

	for _, path := range pathsToInsert {
		prf("Inserting: %v\n", path)
		
		res, err = stmt.Exec(path, entryType)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: unable to insert: %v\n", err)
			return err
		}
		rowCnt, err := res.RowsAffected()
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: error checking rows affected: %v\n", err)
			return err
		}
		if rowCnt != 1 {
			return fmt.Errorf("Number of rows affected was not 1. Was: %d", rowCnt);
		}
	}
	
	return nil
}

func deleteFromDb() {
	// TODO: impl
}

func syncWithDatabase2(pathNames []string) {
	// TODO: future impl should match clj version: push onto delete channel, insert channel or query channel

	// for now doing all in line
	fmt.Printf("Would now syncWithDatabase: %v\n", pathNames)

	// TMP: just do query and insert for now => TODO: deal with deletes later

	var stmt *sql.Stmt
	var err error

	// TODO: should we also pull the type => what if a filename changed into a dirname or vice versa?
	//       both have to match
	// LEFT OFF: no, disagree with the above => just need to know whether it is there => count(*) may be better query
	stmt, err = db.Prepare("select count(path) from files where path = $1")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: unable to prepare stmt for db query in indexer: %v\n", err)
		return
	}
	defer stmt.Close()

	
	// var rows *sql.Rows
	
	for _, path := range pathNames {
		var count int
		err = stmt.QueryRow(path).Scan(&count)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: unable to query database db query in indexer: %v\n", err)
			return
		}
		// DEBUG
		fmt.Printf(">>>========\n Database output %v\n<<<<===========\n", count)
		// END DEBUG
		
		// for rows.Next() {
		// 	var rt string
		// 	rows.Scan(&rt)
		// }
		// rows.Close()
	}

	// LEFT OFF
	
}

// scanDir looks at all the entries in the specified directory.
// It returns a slice of files (full path) and a slice of subdirs (full path)
// It does not recurse into subdirectories.
func scanDir(dirpath string) (files []string, subdirs []string) {
	var finfolist []os.FileInfo
	var err error

	finfolist, err = ioutil.ReadDir(dirpath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: Unable to fully read entries in dir: %v\n", dirpath)
		return files, subdirs
	}

	for _, finfo := range finfolist {
		if finfo.IsDir() {
			// TODO: do I need to worry about type of file separator on Windows?
			subdirs = append(subdirs, dirpath + "/" + finfo.Name())
		} else {
			files = append(files, dirpath + "/" + finfo.Name())
		}
	}
	return files, subdirs
}

// doIndex is the main logic controller for the indexing
// >>> MORE HERE <<<
func doIndex(indexDirs []string, ignorePatterns StringSet) {
	for _, dir := range filter(indexDirs, ignorePatterns) {
		prf("Searching: %v\n", dir)
		files, subdirs := scanDir(dir)
		syncWithDatabase( filter(files, ignorePatterns), filter(subdirs, ignorePatterns) )
		doIndex(subdirs, ignorePatterns)
	}
}

// readInIndexDirs reads in from the fslocate config file that lists
// all the root directories to search and index.  It returns a list of
// strings - each a path to search.  The config file is assumed to have
// one path entry per line.
// If the config file cannot be found or read, a warning is printed to STDERR
// and an empty string slice is returned
func readInIndexDirs() []string {
	indexDirsPath := "conf/fslocate.indexlist"
	indexDirs := make([]string, 0)

	if ! fileExists(indexDirsPath) {
		fmt.Fprintf(os.Stderr,
			"WARN: Unable to find file listing dirs to index: %v\n", indexDirsPath)
		return indexDirs
	}

    file, err := os.Open(indexDirsPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "WARN: Unable to open file for reading: %v\n", indexDirsPath)
        return indexDirs
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ln := strings.TrimSpace(scanner.Text())
		if len(ln) != 0 {
			indexDirs = append(indexDirs, ln)
		}
	}

	if err = scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: Error reading in %v: %v\n", indexDirsPath, err)
	}

	return indexDirs
}

// TODO: implement
func dbHandler() {

}

func fileExists(fpath string) bool {
	_, err := os.Stat(fpath)
	return err == nil
}

func readInIgnorePatterns() StringSet {
	ignoreFilePath := "conf/fslocate.ignore"
	ignorePatterns := StringSet{}

	if ! fileExists(ignoreFilePath) {
		fmt.Fprintf(os.Stderr,
			"WARN: Unable to find ignore patterns file: %v\n", ignoreFilePath)
		return ignorePatterns
	}

    file, err := os.Open(ignoreFilePath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "WARN: Unable to open file for reading: %v\n", ignoreFilePath)
        return ignorePatterns
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        ln := strings.TrimSpace(scanner.Text())
        if len(ln) != 0 {
            ignorePatterns.Add(ln)
        }
    }

    if err = scanner.Err(); err != nil {
        fmt.Fprintf(os.Stderr, "WARN: Error reading in %v: %v\n", ignoreFilePath, err)
    }
	return ignorePatterns
}

func parseArgs(args []string) {
	var intExpected bool
	var sawDashT bool
	for _, a := range args {
		if intExpected {
			if n, err := strconv.Atoi(a); err == nil {
				nindexers = n
				intExpected = false
			} else {
				log.Fatalf("ERROR: Number of indexers specified is not a number: %v\n", a)
			}
		} else if a == "-v" {
			verbose = true
		} else if a == "-t" {
			if sawDashT {
				log.Fatalf("ERROR: -t switch specified more than once: %v\n", a)
			}
			sawDashT = true
			intExpected = true
		} else {
			log.Fatalf("ERROR: Unexpected cmd line argument: %v\n", a)
		}
	}
	if sawDashT && nindexers == 0 {
		log.Fatalf("ERROR: -t switch has no number of indexers after it: %v\n", args)
	}
}

// func contains(args []string, searchFor string) bool {
// 	for _, a := range args {
// 		if a == searchFor {
// 			return true
// 		}
// 	}
// 	return false
// }

func pr(s string) {
	if verbose {
		fmt.Print(s)
	}
}

func prn(s string) {
	if verbose {
		fmt.Println(s)
	}
}

func prf(format string, vals ...interface{}) {
	if verbose {
		fmt.Printf(format, vals...)
	}
}
