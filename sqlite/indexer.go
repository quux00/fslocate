package sqlite

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var verbose bool

const (
	DBPATH = "db/fslocate.db"
	DBCHAN_BUFSZ = 4096
	DIRCHAN_BUFSZ = 10000
	INDEX_FILE  = "conf/fslocate.indexlist"
	IGNORE_FILE = "conf/fslocate.ignore"
	PATH_SEP    = string(os.PathSeparator)
)

type ignorePatterns struct {
	suffixes []string
	patterns []string
}


func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

//
// Attempts to index all the directories specified in the INDEX_FILE
// using the specified number of indexer threads (goroutines).  If the number
// of entries in INDEX_FILE is less than numIndexers, then numIndexers will
// be adjusted down to match that number.
//
func (_ SqliteFsLocate) Index(numIndexers int, beVerbose bool) {
	verbose = beVerbose
	nindexers := numIndexers
	prf("Using %v indexer(s)\n", nindexers)

	/* ---[ set up new database ]--- */
	tmpDb := "db/fslocate.db." + randVal()
	
	prf("Creating db %v\n", tmpDb)
	db, err := sql.Open("sqlite3", tmpDb)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer os.Remove(tmpDb)
	defer db.Close()

	_, err = db.Exec("CREATE TABLE fsentry(path text NOT NULL UNIQUE)")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to create table: %v\n", err)
		return
	}
	prn("fsentry table created")


	/* ---[ launch indexers ]--- */

	entryChan := make(chan string, DBCHAN_BUFSZ)
	doneChan := make(chan bool, nindexers)

	var patterns *ignorePatterns = readInIgnorePatterns()
	
	var toplevelEntries []string = getToplevelEntries(nindexers)
	// if there are fewer entries than requested indexers then decrease 
	// the number of indexers launched
	nindexers = len(toplevelEntries)
	runtime.GOMAXPROCS(nindexers + 1)  // run in parallel fashion -> indexers and dbwriter in separate threads
	for _, entry := range toplevelEntries {
		prf("Indexing top level entries: %s\n", entry)
		go indexer(entryChan, doneChan, patterns, entry)
	}

	prn("indexers launched")

	// the database writer runs in the 'main' thread/goroutine
	streamEntriesIntoDb(entryChan, doneChan, nindexers, db)

	db.Close()
	os.Remove(DBPATH)
	os.Rename(tmpDb, DBPATH)
}


func getToplevelEntries(nindexers int) []string {
	if ! fileExists(INDEX_FILE) {
		log.Fatal("ERROR: Cannot find file " + INDEX_FILE)
	}

	file, err := os.Open(INDEX_FILE)
	if err != nil {
		log.Fatal("ERROR: Cannot open file " + INDEX_FILE)
	}
	defer file.Close()

	dirList := make([]string, nindexers)
	pos := 0
	nentries := 0
	scnr := bufio.NewScanner(file)
	for scnr.Scan() {
		ln := strings.TrimSpace(scnr.Text())
		if len(ln) != 0 && !strings.HasPrefix(ln, "#") {
			dirList[pos] += "," + ln
			pos = (pos + 1) % nindexers
			nentries++
		}
	}
	if err = scnr.Err(); err != nil {
		log.Fatalf("ERROR while reading %s: %v\n", INDEX_FILE, err)
	}

	if nentries < len(dirList) {
		dirList = dirList[0:nentries]
	}
	
	for i, _ := range dirList {
		dirList[i] = strings.TrimLeft(dirList[i], ",")
	}

	return dirList
}

func fileExists(fpath string) bool {
	_, err := os.Stat(fpath)
	return err == nil
}

func streamEntriesIntoDb(entryChan chan string, doneChan chan bool, nindexers int, db *sql.DB) {
	doneCnt := 0
	timeOutCnt := 0	
	
	// turn off auto-commit; begin one large transaction
	// Note: db.Begin() and tx.Commit() seem NOT to work with the sqlite3 driver
	_, err := db.Exec("BEGIN")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: BEGIN Tx failed %v\n", err)
		return
	}
	
	
LOOP:
	for {
		select {
		case entry := <-entryChan:
			err := insertIntoDb(entry, db)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR inserting into db: %v\n", err)
				return
			}
			prf("inserted %s into db\n", entry)
			
		case <-doneChan:
			doneCnt++
			prf("done call received: count is: %d; break cond met? = %v\n", doneCnt, doneCnt >= nindexers)
			
		case <-time.After(300 * time.Millisecond):
			timeOutCnt++
			prf("TIMEOUT: count is: %d; break cond met? = %v\n", doneCnt, doneCnt >= nindexers)
			if doneCnt >= nindexers {
				break LOOP
			}
			if timeOutCnt > 5 {
				fmt.Fprintln(os.Stderr, "WARN: TIMEOUT.")
			}
		}
	}

	_, err = db.Exec("COMMIT")
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: COMMIT Tx failed %v\n", err)
		return
	}
}

func insertIntoDb(entry string, db *sql.DB) error {
	// TODO: check later if there is any value in using prepared stmts with Sqlite
	_, err := db.Exec("INSERT INTO fsentry VALUES(?)", entry)
	return err
}

func indexer(entryChan chan string, doneChan chan bool, ignorePats *ignorePatterns, dirpath string) {
	dirChan := make(chan string, DIRCHAN_BUFSZ)
	dirChan <- strings.TrimRight(dirpath, PATH_SEP)

	var err error
	numErrors := 0
	
	for ; len(dirChan) > 0; {
		prn("indexer loop")
		dir := <- dirChan
		entryChan <- dir

		var entries []os.FileInfo
		entries, err = ioutil.ReadDir(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARN: %v\n", err)
			numErrors++
			if numErrors > 3 {
				fmt.Fprintln(os.Stderr, "ERROR: too many errors, stopping indexing")
				break
			}
		}

		for _, e := range entries {
			fullpath := createFullPath(dir, e.Name())
			if ! shouldIgnore(ignorePats, fullpath) {
				if e.IsDir() {
					dirChan <- fullpath
				} else {
					entryChan <- fullpath
				}
			}
		}
	}
	doneChan <- true
}

func createFullPath(dir, fname string) string {
	var buf bytes.Buffer
	buf.WriteString(dir)
	buf.WriteRune(os.PathSeparator)
	buf.WriteString(fname)
	return buf.String()
}

func randVal() string {
	n := rand.Intn(9999999999)
	return strconv.Itoa(n)
}


//
// Reads in the ingore patterns from IGNORE_FILE
// and returns the entries as an ignorePatterns struct
//
func readInIgnorePatterns() *ignorePatterns {
	var suffixes, patterns []string

	if !fileExists(IGNORE_FILE) {
		fmt.Fprintf(os.Stderr, "WARN: Unable to find ignore patterns file: %v\n", IGNORE_FILE)
		return nil
	}

	file, err := os.Open(IGNORE_FILE)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: Unable to open file for reading: %v\n", IGNORE_FILE)
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ln := strings.TrimSpace(scanner.Text())
		if len(ln) != 0 && !strings.HasPrefix(ln, "#") {
			suffixes, patterns = categorizeIgnorePattern(suffixes, patterns, ln)
		}
	}

	if err = scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: Error reading in %v: %v\n", IGNORE_FILE, err)
	}
	return &ignorePatterns{suffixes: suffixes, patterns: patterns}
}

func categorizeIgnorePattern(suffixes, patterns []string, token string) ([]string, []string) {
	tok := token
	if strings.HasPrefix(tok, "*") {
		tok = tok[1:]
		suffixes = append(suffixes, tok)
	} else if strings.HasSuffix(tok, "/") {
		suffixes = append(suffixes, ensurePrefix(tok[:len(tok)-1], "/"))
		patterns = append(patterns, ensurePrefix(tok, "/"))
	} else {
		patterns = append(patterns, ensurePrefix(tok, "/"))
	}
	return suffixes, patterns
}

func ensurePrefix(s string, prefix string) string {
	if strings.HasPrefix(s, prefix) {
		return s
	}
	return prefix + s
}

//
// Uses the ignore patterns to determine if the file/dir passed in should
// not be indexed. The full path (abspath) is checked as a pure string match first.
// If that is not found in the ignore patterns, then a regex based search is done (??)
//
func shouldIgnore(ignore *ignorePatterns, abspath string) bool {
	if ignore == nil {
		return false
	}
	for _, suffix := range ignore.suffixes {
		if strings.HasSuffix(abspath, suffix) {
			return true
		}
	}

	for _, pat := range ignore.patterns {
		if strings.Contains(abspath, pat) {
			return true
		}
	}
	return false
}



/* ---[ helpers ]--- */

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
