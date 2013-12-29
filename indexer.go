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

//
// ISSUES / TODOs
// * TODO: Filter out things to be ignored from the ignore conf file
// * TODO: write test (manual or unit) that puts stuff in the db that is not on the fs
//
package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"fslocate/fsentry"
	"fslocate/stringset"
	_ "github.com/bmizerany/pq"
	"io/ioutil"
	"os"
	"strings"
	"time"
)

var nindexers int

// constant message types to the dbHandler
const (
	INSERT = iota
	DELETE
	QUERY 
	QUIT    // tell dbHandler thread to shut down
)

const (
	CONFIG_FILE = "conf/fslocate.indexlist"
	IGNORE_FILE = "conf/fslocate.ignore"
)

type dbTask struct {
	action    int       // INSERT, DELETE, QUERY or QUIT
	entry     fsentry.E // full path and type (file or dir)
	replyChan chan dbReply
}

type dbReply struct {
	fsentries []fsentry.E
	err       error
}

// Index needs to be documented
func Index(numIndexers int) {
	nindexers = numIndexers
	prf("Using %v indexer(s)\n", nindexers)

	var (
		err error
		// ignorePatterns stringset.Set = readInIgnorePatterns()  // TODO: still need to utiliize these ignore patterns
		db *sql.DB    // safe for concurrent use by multiple goroutines
	)

	// have the db conx global allows multiple dbHandler routines to share it if necessary
	db, err = initDb()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Cannot established connection to fslocate db: %v\n", err)
		return
	}
	defer db.Close()

	
	dbChan := make(chan dbTask, 32)       // to send requests to the DbHandler
	dirChan := make(chan fsentry.E, 8192) // to enqueue new dirs to search (shared by indexers)
	doneChan := make(chan int)            // for indexers to signal 'done' to main thread

	// launch a single dbHandler goroutine
	go dbHandler(db, dbChan)


	/* ---[ First deal with toplevel entries ]--- */

	// these require special handling in terms of what to delete from the db
	// when this returns there will be some number of directories on the dirChan
	err = syncTopLevelEntries(db, TopLevelInfo{dirChan, dbChan, CONFIG_FILE})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}

	/* ---[ Kick off the indexers ]--- */
	
	for i := 0; i < nindexers; i++ {
		go indexer(i, dirChan, dbChan, doneChan)
	}

	/* ---[ The master thread waits for the indexers to finish ]--- */

	countDownLatch := nindexers
	for ; countDownLatch > 0; countDownLatch-- {
		idxNum := <- doneChan
		prf("Indexer #%d is done\n", idxNum)
	}

	/* ---[ Once indexers done, tell dbHandler to close down the db resources ]--- */

	replyChan := make(chan dbReply)
	pr("Telling dbHandler to shutdown ... ")
	dbChan <- dbTask{action: QUIT, replyChan: replyChan}
	prn("DONE (dbHandler shutdown)")

	select {
	case <-replyChan:
	case <-time.After(5 * time.Second):
		fmt.Fprintln(os.Stderr, "WARN: DbHandler thread did not shutdown in the alloted time")
	}
}

func initDb() (*sql.DB, error) {
	return sql.Open("postgres", "user=midpeter444 password=jiffylube dbname=fslocate sslmode=disable")
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
// direntry should be a fsentry with typ = fsentry.DIR
//
//
func indexEntry(direntry fsentry.E, dirChan chan fsentry.E, dbChan chan dbTask, replyChan chan dbReply) error {
	prf("QUERY sent to dbHandler for: %v\n", direntry)
	dbChan <- dbTask{QUERY, direntry, replyChan}

	var fsentries []fsentry.E
	fsentries, err := scanDir(direntry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: Unable to fully read entries in dir %v: %v\n", direntry.Path, err)
		return err
	}

	prf("Attempting to get QUERY results from dbChan for: %v\n", direntry)
	reply := <- replyChan
	prf("Reply is: %v\n", reply)
	if reply.err != nil {
		fmt.Fprintf(os.Stderr, "Error while reading from the database: %v\n", err)
		return reply.err
	}
	dbrecsSet := fsentry.NewSet(reply.fsentries...)
	fsrecsSet := fsentry.NewSet(fsentries...)

	fsonly := fsrecsSet.Difference(dbrecsSet)
	dbonly := dbrecsSet.Difference(fsrecsSet)

	prf("dbrecSets: %v\n", dbrecsSet)
	prf("fsrecSets: %v\n", fsrecsSet)
	prf("dbonly: %v\n", dbonly)
	prf("fsonly: %v\n", fsonly)
	
	N := len(fsonly) + len(dbonly)
	tmpReplyChan := make(chan dbReply, N)
	for entry, _ := range dbonly {
		dbChan <- dbTask{DELETE, entry, tmpReplyChan}
	}

	for entry, _ := range fsonly {
		dbChan <- dbTask{INSERT, entry, tmpReplyChan}

		if entry.Typ == fsentry.DIR && entry.Path != direntry.Path {
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
func scanDir(direntry fsentry.E) (entries []fsentry.E, err error) {
	var lsFileInfo []os.FileInfo
	entries = make([]fsentry.E, 1, 4)
	entries[0] = direntry

	prf("Scanning %v for files\n", direntry.Path)
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
		rows.Scan(&nextEntry.Path, &nextEntry.Typ, &nextEntry.IsTopLevel)
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
	prf("getChildEntriesInDb: []entries coming in: %v\n", entries)
	children := entries[0].Path + "/%"
	grandchildren := entries[0].Path + "/%/%"

	rows, err := qStmt2.Query(children, grandchildren)
	if err != nil {
		return entries, err
	}
	defer rows.Close()

	for rows.Next() {
		var nextEntry fsentry.E
		rows.Scan(&nextEntry.Path, &nextEntry.Typ, &nextEntry.IsTopLevel)
		entries = append(entries, nextEntry)
	}
	prf("getChildEntriesInDb: []entries going out: %v\n", entries)
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

	// store boolean vals as 0 or 1 in the db
	toplevel := 0
	if entry.IsTopLevel {
		toplevel = 1
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

	qryStmt, err := db.Prepare("SELECT path, type, toplevel FROM fsentry WHERE path = $1")
	if err != nil {
		panic(err)
	}
	defer qryStmt.Close()

	qryStmt2, err := db.Prepare("SELECT path, type, toplevel FROM fsentry WHERE path LIKE $1 AND path NOT LIKE $2")
	if err != nil {
		panic(err)
	}
	defer qryStmt2.Close()

	var task dbTask
	var replyChan chan dbReply

MAINLOOP:
	for {
		task = <-dbChan
		switch task.action {
		case QUERY:
			prf(">> dbHandler QUERYING: %v\n", task.entry)
			entries, err := dbQuery(qryStmt, qryStmt2, task.entry)
			task.replyChan <- dbReply{entries, err}

		case INSERT:
			prf(">> dbHandler INSERTING: %v\n", task.entry)
			err = dbInsert(insStmt, task.entry)
			task.replyChan <- dbReply{err: err}

		case DELETE:
			prf(">> dbHandler DELETING: %v\n", task.entry)
			err = dbDelete(delStmt, task.entry)
			task.replyChan <- dbReply{err: err}

		case QUIT:
			prn(">> dbHandler received QUIT notice")
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
	ignoreFilePath := IGNORE_FILE
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

func pr(s string) {
	if verbose {
		fmt.Print(s)
		os.Stdout.Sync()
	}
}

func prn(s string) {
	if verbose {
		fmt.Println(s)
		os.Stdout.Sync()
	}
}

func prf(format string, vals ...interface{}) {
	if verbose {
		fmt.Printf(format, vals...)
		os.Stdout.Sync()
	}
}
