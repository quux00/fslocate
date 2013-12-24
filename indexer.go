// TODO: move this to its own subpackage
// DESIGN
// The indexer has two phases, the latter of which has three main controls of flow
// ----------------------------------
// Phase 1: read in the conf.fslocate.indexlist file to read in all the 'toplevel' dirs
//          This is compared with toplevel entries in the database and inserts or deletions
//          in the db are done as appropriate
// ----------------------------------
// Phase 2: 2 goroutines and 1 controller in the main thread
//   Main thread:
//     creates indexer goroutines and database controller goroutine
//     The indexer goroutines are giving a channel to listen on for the next directory to search
//     The # of indexer threads is determined by the nindexers value
//     The main thread keeps a list of directories to be searched and feeds the nextdir-channel
//   Indexer goroutine:
//     Waits for a dir to search on the nextdir-channel
//     Once it has one it sends a query to the dbquery-ch asking for all the entries in the db
//        for that dir and it sends a channel to be messaged back on with the answer
//     While waiting it looks up the entries in the fs
//     It waits for the answer from the db and compares the results.  Based on that it puts
//     db deletions on the dbdelete-channel and db-inserts on the dbinsert-channel
//     When it has finished it puts all subdirs it found onto the nextdir-channel  (WAIT: MAY NOT NEED THE MAIN THREAD TO MONITOR THE NEXTDIR-CH => MAY BE SELF-REGULATING !!)
//   Database handler goroutine:
//     LEFT OFF HERE
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
	"fslocate/stringset"
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

	var ignorePatterns stringset.Set = readInIgnorePatterns()
	var topIndexDirs []string = readInTopLevelIndexDirs()

	// DEBUG
	// fmt.Printf("nindexers: %v\n", nindexers)
	// prf("%T: %v\n", args, args)
	fmt.Printf("%v\n", ignorePatterns)
	fmt.Printf("%v\n", topIndexDirs)
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

	err = processTopLevelEntries(topIndexDirs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
		return
	}

	// TODO: put this in a goroutine
	// TODO: split up the topIndexDirs into nindexers equal sized pieces
	doIndex(topIndexDirs, ignorePatterns)
}

// doIndex is the main logic controller for the indexing
// >>> MORE HERE <<<
func doIndex(indexDirs []string, ignorePatterns stringset.Set) {
	var err error
	for _, dir := range filter(indexDirs, ignorePatterns) {
		prf("Searching: %v\n", dir)
		files, subdirs := scanDir(dir)

		err = syncWithDatabase( filter(files, ignorePatterns), filter(subdirs, ignorePatterns) )
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
			return
		}
		doIndex(subdirs, ignorePatterns)
	}
}


func initDb() error {
	var err error
	db, err = sql.Open("postgres", "user=midpeter444 password=jiffylube dbname=fslocate sslmode=disable")
	if err != nil {
		return err
	}
	return nil
}


// processTopLevelEntries 
func processTopLevelEntries(confIndexDirs []string) error {
	var (
		err error
		dbIndexDirs, delPaths []string
	)

	dbIndexDirs, err = lookUpTopLevelDirsInDb()
	if err != nil {
		return err
	}
	prf("DB-Index-Dirs: %v\n", dbIndexDirs)

	delPaths = determineTopLevelPathsToDeleteInDb(confIndexDirs, dbIndexDirs)
	// TODO: send this into its own goroutine -> need to pass a channel in to synchronize on
	err = dbDelete(delPaths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
		return err
	}

	// insert all toplevel dirs as such
	for _, dir := range confIndexDirs {
		if finfo, err := os.Stat(dir); err != nil {
			return err
		} else if !finfo.IsDir() {
			return fmt.Errorf("ERROR: %v in in the config indexdir is not a directory", dir)
		}
	}
	_, dirsToInsert, err := findEntriesNotInDb(nil, confIndexDirs)
	if err != nil {
		return err
	}

	if dirsToInsert != nil && len(dirsToInsert) > 0 {
		dbInsert(confIndexDirs, DIR_TYPE, true)
	}

	return nil
}

// isChildOfAny checks whether path is a child of any of the
// paths in the paths Set.
// TODO: Set should be renamed Set, since you have to say stringset.Set
func isChildOfAny(paths stringset.Set, candidateChild string) bool {
	for candidateParent := range paths {
		if strings.HasPrefix(candidateChild, candidateParent) {
			return true
		}
	}
	return false
}


// determineTopLevelPathsToDeleteInDb determines what top level entries to delete from
// the database table. The values it returns may have SQL wildcards (%), so delete
// stmts should be done with 'like' not '='
func determineTopLevelPathsToDeleteInDb(confIndexDirs, dbIndexDirs []string) []string {
	
	dbset := stringset.New(dbIndexDirs...)
	confset := stringset.New(confIndexDirs...)
	inDbOnlySet := dbset.Difference(confset)
	delPaths := make([]string, 0)

	for dbdir := range inDbOnlySet {
		// entries only in the dbset that are children of an entry in the confIndexDir
		// only needs to be deleted itself (not its children). A dbdir that is a child
		// of a confIndexDir is determined by the confIndexDir being a prefix of the dbdir.
		// If dbdir is not a child, then delete it and all its children (by appending % wildcard)
		if isChildOfAny(confset, dbdir) {
			delPaths = append(delPaths, dbdir)
		} else {
			delPaths = append(delPaths, dbdir + "%")
		}
	}
	
	return delPaths
}

func lookUpTopLevelDirsInDb() (dbIndexDirs []string, err error) {
	var (
		stmt *sql.Stmt
		rows *sql.Rows
	)

	stmt, err = db.Prepare("SELECT path FROM files WHERE toplevel = true")
	if err != nil {
		return
	}
	defer stmt.Close()

	rows, err = stmt.Query()
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var path string
		rows.Scan(&path)
		dbIndexDirs = append(dbIndexDirs, path)
	}

	return dbIndexDirs, nil
}

// filter removes all pathNames where the basename (filepath.Base)
// matches an "ignore" pattern in the ignorePatterns Set
// create and returns a new []string; it does not modify the pathNames
// slice passed in
func filter(pathNames []string, ignorePatterns stringset.Set) []string {
	keepers := make([]string, 0, len(pathNames))
	for _, path := range pathNames {
		basepath := filepath.Base(path)
		if ! ignorePatterns.Contains(basepath) {
			keepers = append(keepers, path)
		}
	}
	return keepers
}

func syncWithDatabase(fileNames, dirNames []string) error {
	filesToInsert, dirsToInsert, err := findEntriesNotInDb(fileNames, dirNames)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
		return err
	}
	err = dbInsert(filesToInsert, FILE_TYPE, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
		return err
	}
	err = dbInsert(dirsToInsert, DIR_TYPE, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
		return err
	}
	// dbDelete() // TODO: how will this work?
	return nil
}

func findEntriesNotInDb(filePaths, dirPaths []string) (filesToInsert, dirsToInsert []string, err error) {
	var (
		stmt *sql.Stmt
		count int
	)

	stmt, err = db.Prepare("SELECT count(path) FROM files WHERE path = $1")
	if err != nil { return }
	defer stmt.Close()

	f := func(pathNames []string) (pathsToInsert []string, err error) {
		pathsToInsert = make([]string, 0, len(pathsToInsert))

		for _, path := range pathNames {
			err = stmt.QueryRow(path).Scan(&count)
			if err != nil {
				return pathsToInsert, err
			}
			if count == 0 {
				pathsToInsert = append(pathsToInsert, path)
			}
		}
		return pathsToInsert, nil
	}

	filesToInsert, err = f(filePaths)
	if err != nil { return }

	dirsToInsert, err  = f(dirPaths)
	if err != nil { return }

	return filesToInsert, dirsToInsert, err
}

// dbDelete deletes each of the paths passed in from the files table
// the paths may have wildcards, since SQL is uses like not = to
// select which rows to delete.
func dbDelete(paths []string) error {
	stmt, err := db.Prepare("DELETE FROM files WHERE path like $1")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, path := range paths {
		prf("Deleting: %v\n", path)

		res, err := stmt.Exec(path)
		if err != nil {
			return err
		}
		_, err = res.RowsAffected()
		if err != nil {
			return err
		}
		// not checking row count
		// because with the wildcards a parent with wildcard may delete a child
		// and when the child delete stmt executes it doesn't delete anything
	}
	return nil
}


// dbInsert inserts into the files table
// all pathsToInsert must either be toplevel or not toplevel, where
// 'toplevel' means it is a starting or 'root' directory in the user's config dir
func dbInsert(pathsToInsert []string, entryType string, toplevel bool) error {
	var (
		stmt *sql.Stmt
		res sql.Result
		err error
	)

	stmt, err = db.Prepare("INSERT INTO files(path, type, toplevel) values($1, $2, $3)")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: unable to prepare insert stmt: %v\n", err)
		return err
	}
	defer stmt.Close()
	
	for _, path := range pathsToInsert {
		prf("Inserting: %v\n", path)

		res, err = stmt.Exec(path, entryType, toplevel)
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

// scanDir looks at all the entries in the specified directory.
// It returns a slice of files (full path) and a slice of subdirs (full path)
// It does not recurse into subdirectories.
func scanDir(dirpath string) (files, subdirs []string) {
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


// readInTopLevelIndexDirs reads in from the fslocate config file that lists
// all the root directories to search and index.  It returns a list of
// strings - each a path to search.  The config file is assumed to have
// one path entry per line.
// If the config file cannot be found or read, a warning is printed to STDERR
// and an empty string slice is returned
func readInTopLevelIndexDirs() []string {
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

func readInIgnorePatterns() stringset.Set {
	ignoreFilePath := "conf/fslocate.ignore"
	ignorePatterns := stringset.Set{}

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
	} else if nindexers == 0 {
		nindexers = 1
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
