// TODO: move this to its own subpackage
// DESIGN
// The indexer has two phases, the latter of which has three main controls of flow
// ----------------------------------
// Phase 1: read in the conf.fslocate.indexlist file to read in all the 'toplevel' dirs
//          This is compared with toplevel entries in the database and inserts or deletions
//          in the db are done as appropriate
// ----------------------------------
// Phase 2: 2 types of goroutines and 1 controller in the main thread
//   Main thread:
//     creates indexer goroutines and database controller goroutine
//     The indexer goroutines are given a channel to listen on for the next directory to search
//     The # of indexer threads is determined by the nindexers value
//     The main thread keeps a list of directories to be searched and feeds the nextdir-channel
//   Indexer goroutine:
//     Waits for a dir to search on the nextdir-channel
//     Once it has one it sends a query to the dbquery-ch asking for all the entries in the db
//        for that dir and it sends a channel to be messaged back on with the answer
//     While waiting, it looks up the entries in the fs
//     It waits for the answer from the db and compares the results.  Based on that it puts
//       * db deletions on the dbdelete-channel and
//       * db-inserts on the dbinsert-channel
//     When it has finished it puts all subdirs it found onto the nextdir-channel  (WAIT: MAY NOT NEED THE MAIN THREAD TO MONITOR THE NEXTDIR-CH => MAY BE SELF-REGULATING !!)
//   Database handler goroutine:
//     LEFT OFF HERE
package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"fslocate/fsentry"
	"fslocate/stringset"
	_ "github.com/bmizerany/pq"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var verbose bool
var nindexers int

const (
	INSERT = "insert"
	DELETE = "delete"
	QUERY  = "query"
	QUIT   = "quit" // tell dbHandler thread to shut down
)

// TODO: this may need to change for query actions
type dbTask struct {
	action    string    // INSERT, DELETE, QUERY or QUIT
	entry     fsentry.E // full path and type (file or dir)
	replyChan chan dbReply
}

// TODO: this may need to change for inserts/deletes
type dbReply struct {
	fsentries []fsentry.E
	err       error
}

// Index needs to be documented
func Index(args []string) {
	parseArgs(args)
	prf("Using %v indexers", nindexers)

	var (
		err error
		ignorePatterns stringset.Set = readInIgnorePatterns()
		topIndexDirs []string = readInTopLevelIndexDirs()
		db *sql.DB    // safe for concurrent use by multiple goroutines
	)

	// DEBUG
	// fmt.Printf("nindexers: %v\n", nindexers)
	// prf("%T: %v\n", args, args)
	fmt.Printf("%v\n", ignorePatterns)
	fmt.Printf("%v\n", topIndexDirs)
	// END DEBUG

	// have the db conx global allows multiple dbHandler routines to share it if necessary
	db, err = initDb()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Cannot established connection to fslocate db: %v\n", err)
		return
	}
	defer db.Close()

	// TODO: deferring the initial top level entries until the end
	// err = processTopLevelEntries(topIndexDirs)
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
	// 	return
	// }
	// // TODO: remove this once the new goroutine versions are working below?
	// doIndexOrig(topIndexDirs, ignorePatterns)

	// once the top level dirs are dealt with, start go routines (??)
	// TODO: call as goroutine
	// TODO: split up the topIndexDirs into nindexers equal sized pieces
	// TODO: this will be the goroutine that handles incoming requests to query, delete or insert from the db
	// TODO: need to pass it a channel

	/* ---[ Kick off the routines ]--- */

	dbChan := make(chan dbTask, 32)       // to send requests to the DbHandler
	dirChan := make(chan fsentry.E, 8192) // to enqueue new dirs to search (shared by indexers)
	doneChan := make(chan int)            // for indexers to signal 'done' to main thread

	go dbHandler(db, dbChan)

	for i := 0; i < nindexers; i++ {
		go indexer(i, dirChan, dbChan, doneChan)
	}

	/* ---[ The master thread waits for the indexers to finish ]--- */

	countDownLatch := nindexers
	for countDownLatch > 0 {
		idxNum := <- doneChan
		prf("Indexer #%d is done\n", idxNum)
	}

	/* ---[ Once indexers done, tell dbHandler to close down the db resources ]--- */

	replyChan := make(chan dbReply)
	dbChan <- dbTask{action: QUIT, replyChan: replyChan}

	select {
	case <-replyChan:
	case <-time.After(5 * time.Second):
		fmt.Fprintln(os.Stderr, "WARN: DbHandler thread did not shutdown in the alloted time")
	}
}

//
// indexer runs in its own goroutine
// ... MORE HERE ...
//
func indexer(idxNum int, dirChan chan fsentry.E, dbChan chan dbTask, doneChan chan int) {
	var nextEntry fsentry.E
	misses := 0  // num times have read from dirChan and "missed" (timed out)
	             // when miss twice in a row, then send q done note on the doneChan and exit
	replyChan := make(chan dbReply, 16)
	maxMisses := 2

	for misses < maxMisses {
		select {
		case nextEntry = <- dirChan:
			misses = 0
			err := indexEntry(nextEntry, dirChan, dbChan, replyChan)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				doneChan <- idxNum
				return
			}

		default:
			misses++
			if misses < maxMisses {
				time.Sleep(250 * time.Millisecond)
			}
		}
	}

	doneChan <- idxNum
}

//
// direntry should be a fsentry with typ = DIR_TYPE
//
//
func indexEntry(direntry fsentry.E, dirChan chan fsentry.E, dbChan chan dbTask, replyChan chan dbReply) error {
	dbChan <- dbTask{QUERY, direntry, replyChan}

	var fsentries []fsentry.E
	fsentries, err := scanDir2(direntry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: Unable to fully read entries in dir %v: %v\n", direntry.Path, err)
		return err
	}

	reply := <- replyChan
	if reply.err != nil {
		fmt.Fprintf(os.Stderr, "Error while reading from the database: %v\n", err)
		return reply.err
	}
	dbrecsSet := fsentry.NewSet(reply.fsentries...)
	fsrecsSet := fsentry.NewSet(fsentries...)

	fsonly := fsrecsSet.Difference(dbrecsSet)
	dbonly := dbrecsSet.Difference(fsrecsSet)

	// DEBUG
	fmt.Printf(">> fsonly: %v\n", fsonly)
	fmt.Printf(">> dbonly: %v\n", dbonly)
	// END DEBUG

	N := len(fsonly) + len(dbonly)
	tmpReplyChan := make(chan dbReply, N)
	for entry, _ := range dbonly {
		dbChan <- dbTask{DELETE, entry, tmpReplyChan}
	}

	for entry, _ := range fsonly {
		dbChan <- dbTask{INSERT, entry, tmpReplyChan}

		if entry.Typ == fsentry.DIR {
			putOnDirChan(entry, dirChan)
		}
	}

	// now check the Db Replies for any errors
	err = checkErrorsOnDbReplies(tmpReplyChan, N)
	if err != nil {
		panic(err)
	}
	return nil
}

func checkErrorsOnDbReplies(replyChan chan dbReply, numExp int) error {
	timeout := time.After(10 * time.Second)
	for i := 0; i < numExp; i++ {
		select {
		case reply := <-replyChan:
			if reply.err != nil {
				return reply.err
			}
		case <-timeout:
			return fmt.Errorf("Timed out trying to read db replies. Read %d out of %d.", i, numExp)
		}
	}
	return nil
}

func putOnDirChan(entry fsentry.E, dirChan chan fsentry.E) {
	// putting onto dirChan could hang if dirChan fills up, so do a non-blocking put
	select {
	case dirChan <-entry:
	default:
		// TODO: when the put fails, need to have a backup signaling channel to the
		// main thead to start more indexers
		// for now just log in the issue
		fmt.Fprintf(os.Stderr, "WARN: dir channel full! Dropped: %v\n", entry.Path)
	}
}

//
// direntry should be a fsentry.E with Typ = DIR
// TODO: can this be merged with the orig scanDir ??
//
func scanDir2(direntry fsentry.E) (entries []fsentry.E, err error) {
	var lsFileInfo []os.FileInfo
	entries = make([]fsentry.E, 0, 4)

	lsFileInfo, err = ioutil.ReadDir(direntry.Path)
	if err != nil {
        return
	}

	for _, finfo := range lsFileInfo {
		enttyp := fsentry.FILE
		if finfo.IsDir() {
			enttyp = fsentry.DIR
		}
		// TODO: check if should skipDir (based on ignore patterns)
		entries = append(entries, fsentry.E{Path: direntry.Path + "/" + finfo.Name(), Typ: enttyp})
	}
	return entries, nil
}

//
// doIndexOrig is the main logic controller for the indexing
// >>> MORE HERE <<<
//
// func doIndexOrig(indexDirs []string, ignorePatterns stringset.Set) {
// 	var err error
// 	for _, dir := range filter(indexDirs, ignorePatterns) {
// 		prf("Searching: %v\n", dir)
// 		files, subdirs := scanDir(dir)

// 		err = syncWithDatabase( filter(files, ignorePatterns), filter(subdirs, ignorePatterns) )
// 		if err != nil {
// 			fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
// 			return
// 		}
// 		doIndex(subdirs, ignorePatterns)
// 	}
// }

func initDb() (*sql.DB, error) {
	return sql.Open("postgres", "user=midpeter444 password=jiffylube dbname=fslocate sslmode=disable")
}

//
// processTopLevelEntries deals with the "top level" dirs specified
// in the user's config file ("indexlist")
// confIndexDirs ls of directories to start processing
//
// func processTopLevelEntries(confIndexDirs []string) error {
// 	var (
// 		err error
// 		dbIndexDirs, delPaths []string
// 	)

// 	dbIndexDirs, err = lookUpTopLevelDirsInDb()
// 	if err != nil {
// 		return err
// 	}
// 	prf("DB-Index-Dirs: %v\n", dbIndexDirs)

// 	delPaths = determineTopLevelPathsToDeleteInDb(confIndexDirs, dbIndexDirs)
// 	// TODO: send this into its own goroutine -> need to pass a channel in to synchronize on
// 	err = dbDelete(delPaths)
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
// 		return err
// 	}

// 	// ensure that the top level conf index dirs exist and are dirs
// 	for _, dir := range confIndexDirs {
// 		if finfo, err := os.Stat(dir); err != nil {
// 			return err
// 		} else if !finfo.IsDir() {
// 			return fmt.Errorf("ERROR: %v in in the config indexdir is not a directory", dir)
// 		}
// 	}
// 	// insert all toplevel dirs as such
// 	_, dirsToInsert, err := findEntriesNotInDb(nil, confIndexDirs)
// 	if err != nil {
// 		return err
// 	}
// 	dbInsert(dirsToInsert, DIR_TYPE, true)

// 	return nil
// }

//
// isChildOfAny checks whether path is a child of any of the
// paths in the paths Set.
//
func isChildOfAny(paths stringset.Set, candidateChild string) bool {
	for candidateParent := range paths {
		if strings.HasPrefix(candidateChild, candidateParent) {
			return true
		}
	}
	return false
}

//
// determineTopLevelPathsToDeleteInDb determines what "top level" entries to delete
// from the databasea. The values it returns may have SQL wildcards (%), so delete
// stmts should be done with 'like' not '='
//
func determineTopLevelPathsToDeleteInDb(confIndexDirs, dbIndexDirs []string) []string {

	dbset := stringset.New(dbIndexDirs...)
	confset := stringset.New(confIndexDirs...)
	inDbOnlySet := dbset.Difference(confset)
	delPaths := make([]string, 0, 1)

	for dbdir := range inDbOnlySet {
		// TODO: this logic needs to be checked one more time and the lang below simplified
		// entries only in the dbset that are children of an entry in the confIndexDir
		// only needs to be deleted itself (not its children). A dbdir that is a child
		// of a confIndexDir is determined by the confIndexDir being a prefix of the dbdir.
		// If dbdir is not a child, then delete it and its children (append % wildcard)
		if isChildOfAny(confset, dbdir) {
			delPaths = append(delPaths, dbdir)
		} else {
			delPaths = append(delPaths, dbdir+"%")
		}
	}

	return delPaths
}

//
// Returns ls of all paths in the database that are marked as toplevel
//
// func lookUpTopLevelDirsInDb() (dbIndexDirs []string, err error) {
// 	var (
// 		stmt *sql.Stmt
// 		rows *sql.Rows
// 	)

// 	stmt, err = db.Prepare("SELECT path FROM fsentry WHERE toplevel = true")
// 	if err != nil {
// 		return
// 	}
// 	defer stmt.Close()

// 	rows, err = stmt.Query()
// 	if err != nil {
// 		return
// 	}
// 	defer rows.Close()

// 	for rows.Next() {
// 		var path string
// 		rows.Scan(&path)
// 		dbIndexDirs = append(dbIndexDirs, path)
// 	}

// 	return dbIndexDirs, nil
// }

// filter removes all pathNames where the basename (filepath.Base)
// matches an "ignore" pattern in the ignorePatterns Set
// create and returns a new []string; it does not modify the pathNames
// slice passed in
func filter(pathNames []string, ignorePatterns stringset.Set) []string {
	keepers := make([]string, 0, len(pathNames))
	for _, path := range pathNames {
		basepath := filepath.Base(path)
		if !ignorePatterns.Contains(basepath) {
			keepers = append(keepers, path)
		}
	}
	return keepers
}

// func syncWithDatabase(fileNames, dirNames []string) error {
// 	filesToInsert, dirsToInsert, err := findEntriesNotInDb(fileNames, dirNames)
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
// 		return err
// 	}
// 	err = dbInsert(filesToInsert, FILE_TYPE, false)
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
// 		return err
// 	}
// 	err = dbInsert(dirsToInsert, DIR_TYPE, false)
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
// 		return err
// 	}
// 	// dbDelete() // TODO: how will this work?
// 	return nil
// }

//
// findEntriesNotInDb looks in the database for each of the paths in
// filePaths and dirPaths. Any paths not in the database go into the
// filesToInsert and dirsToInsert string lists, respectively.
// This fn does NOT search the filesystem for entries.
//
// func findEntriesNotInDb(filePaths, dirPaths []string) (filesToInsert, dirsToInsert []string, err error) {
// 	var (
// 		stmt *sql.Stmt
// 		count int
// 	)

// 	stmt, err = db.Prepare("SELECT count(path) FROM files WHERE path = $1")
// 	if err != nil { return }
// 	defer stmt.Close()

// 	f := func(pathNames []string) (pathsToInsert []string, err error) {
// 		pathsToInsert = make([]string, 0, len(pathsToInsert))

// 		for _, path := range pathNames {
// 			err = stmt.QueryRow(path).Scan(&count)
// 			if err != nil {
// 				return pathsToInsert, err
// 			}
// 			if count == 0 {
// 				pathsToInsert = append(pathsToInsert, path)
// 			}
// 		}
// 		return pathsToInsert, nil
// 	}

// 	filesToInsert, err = f(filePaths)
// 	if err != nil {
// 		return
// 	}

// 	dirsToInsert, err = f(dirPaths)
// 	if err != nil {
// 		return
// 	}

// 	return filesToInsert, dirsToInsert, err
// }

//
// dbDelete deletes each of the paths passed in from the files table
// the paths may have wildcards, since SQL is uses like not = to
// select which rows to delete.
//
func dbDelete(delStmt *sql.Stmt, entry fsentry.E) error {
	prf("Deleting: %v (type: %s)\n", entry.Path, entry.Typ)

	res, err := delStmt.Exec(entry.Path, entry.Typ)
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
	return nil
}

//
// new: NOT TESTED
// dbQuery
//
func dbQuery(qStmt, qStmt2 *sql.Stmt, entry fsentry.E) ([]fsentry.E, error) {
	entries := make([]fsentry.E, 0, 4)

	rows, err := qStmt.Query(entry.Path)
	if err != nil {
		return entries, err
	}
	defer rows.Close()

	for rows.Next() {
		var nextEntry fsentry.E
		rows.Scan(&nextEntry.Path, &nextEntry.Typ)
		entries = append(entries, nextEntry)
	}

	switch len(entries) {
	case 0:
		return entries, nil
	case 1:
		return getChildEntriesInDb(qStmt2, entries)
	default:
		return entries, fmt.Errorf("Database integrity issue: path %v found %d times",
			entry.Path, len(entries))
	}
}

//
// getChildEntriesInDb xxxx
// will add to the entries list
// Assumption: entries has one element in it
//
func getChildEntriesInDb(qStmt2 *sql.Stmt, entries []fsentry.E) ([]fsentry.E, error) {
	lowerCasePath := strings.ToLower(entries[0].Path)
	children := lowerCasePath + "/%"
	grandchildren := lowerCasePath + "/%/%"

	rows, err := qStmt2.Query(fsentry.FILE, children, grandchildren)
	if err != nil {
		return entries, err
	}
	defer rows.Close()

	for rows.Next() {
		var nextEntry fsentry.E
		rows.Scan(&nextEntry.Path, &nextEntry.Typ)
		entries = append(entries, nextEntry)
	}
	return entries, nil
}

//
// dbInsert inserts into the fsentry table
// 'toplevel' means it is a starting or 'root' directory in the user's config dir
//
func dbInsert(insStmt *sql.Stmt, entry fsentry.E) error {
	var (
		res sql.Result
		err error
	)
	prf("Inserting: %v\n", entry.Path)

	toplevel := "f"
	if entry.IsTopLevel {
		toplevel = "t"
	}
	res, err = insStmt.Exec(entry.Path, entry.Typ, toplevel)
	if err != nil {
		return err
	}

	rowCnt, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowCnt != 1 {
		return fmt.Errorf("Number of rows affected was not 1. Was: %d", rowCnt)
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
			subdirs = append(subdirs, dirpath+"/"+finfo.Name())
		} else {
			files = append(files, dirpath+"/"+finfo.Name())
		}
	}
	return files, subdirs
}

//
// readInTopLevelIndexDirs reads in from the fslocate config file that lists
// all the root directories to search and index.  It returns a list of
// strings - each a path to search.  The config file is assumed to have
// one path entry per line.
// If the config file cannot be found or read, a warning is printed to STDERR
// and an empty string slice is returned
//
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

//
// DOCUMENT ME!!
//
func dbHandler(db *sql.DB, dbChan chan dbTask) {
	delStmt, err := db.Prepare("DELETE FROM fsentry WHERE path like $1 AND type = $2") // allows wildcards for path
	if err != nil {
		panic(err)
	}
	defer delStmt.Close()

	insStmt, err := db.Prepare("INSERT INTO fsentry (path, type, toplevel) VALUES($1, $2, $3)")
	if err != nil {
		panic(err)
	}
	defer insStmt.Close()

	// TODO: really need this one?
	qryStmt, err := db.Prepare("SELECT path, type FROM fsentry WHERE path = $1")
	if err != nil {
		panic(err)
	}
	defer qryStmt.Close()

	qryStmt2, err := db.Prepare("SELECT path, type FROM fsentry WHERE type = $1 AND lower(path) LIKE $2 AND lower(path) NOT LIKE $3")
	if err != nil {
		panic(err)
	}
	defer func() {
		fmt.Printf("!!! qryStmt2 being closed !!!\n", )
		qryStmt2.Close()
	}()

	var task dbTask
	var replyChan chan dbReply

MAINLOOP:
	for {
		task = <-dbChan
		switch task.action {
		case QUERY:
			fmt.Printf(">> dbHandle QUERYING\n")
			entries, err := dbQuery(qryStmt, qryStmt2, task.entry)
			task.replyChan <- dbReply{entries, err}

		case INSERT:
			fmt.Printf(">> dbHandle INSERTing\n")
			err = dbInsert(insStmt, task.entry)
			task.replyChan <- dbReply{err: err}

		case DELETE:
			fmt.Printf(">> dbHandle DELETEing\n")
			err = dbDelete(delStmt, task.entry)
			task.replyChan <- dbReply{err: err}

		case QUIT:
			fmt.Printf(">> dbHandler received QUIT notices\n")
			replyChan = task.replyChan
			break MAINLOOP
		}
	}

	replyChan <- dbReply{} // send back empty sentinel ack shutdown
}

func fileExists(fpath string) bool {
	_, err := os.Stat(fpath)
	return err == nil
}

func readInIgnorePatterns() stringset.Set {
	ignoreFilePath := "conf/fslocate.ignore"
	ignorePatterns := stringset.Set{}

	if !fileExists(ignoreFilePath) {
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

// TODO: is there a way to flush STDOUT in Go?

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
