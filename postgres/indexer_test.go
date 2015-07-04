package postgres

import (
	"database/sql"
	"fmt"
	"fslocate/fsentry"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemoveStarSuffix(t *testing.T) {
	s1 := ""
	s2 := "a"
	s3 := "*"
	s4 := "a*"
	s5 := "abc/*"

	if removeStarSuffix(s1) != s1 {
		t.Errorf(removeStarSuffix(s1))
	}

	if removeStarSuffix(s2) != s2 {
		t.Errorf(removeStarSuffix(s2))
	}

	if removeStarSuffix(s3) != "" {
		t.Errorf(removeStarSuffix(s3))
	}

	if removeStarSuffix(s4) != "a" {
		t.Errorf(removeStarSuffix(s4))
	}

	if removeStarSuffix(s5) != "abc/" {
		t.Errorf(removeStarSuffix(s5))
	}

}

func TestRegexEscape(t *testing.T) {
	s1 := "*.class"
	s2 := "hi[mom]"
	s3 := "f$y.class"
	s4 := "xxx"
	s5 := ""
	s6 := "()"

	if regexEscape(s1) != `\*\.class` {
		t.Errorf(regexEscape(s1))
	}
	if regexEscape(s2) != `hi\[mom\]` {
		t.Errorf(regexEscape(s2))
	}
	if regexEscape(s3) != `f\$y\.class` {
		t.Errorf(regexEscape(s3))
	}
	if regexEscape(s4) != `xxx` {
		t.Errorf(regexEscape(s4))
	}
	if regexEscape(s5) != `` {
		t.Errorf(regexEscape(s5))
	}
	if regexEscape(s6) != `\(\)` {
		t.Errorf(regexEscape(s6))
	}
}

func TestDbHandlerInsertAndDelete(t *testing.T) {
	var (
		db  *sql.DB
		err error
	)

	db, err = initDb()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return
	}
	defer db.Close()

	dbChan := make(chan dbTask, 10)
	replyChan := make(chan dbReply, 1)
	var reply dbReply

	go dbHandler(db, dbChan)

	fse := fsentry.E{"/var/log/hive/foo", "f", false}

	// ask handler to do insert
	dbChan <- dbTask{INSERT, fse, replyChan}
	reply = <-replyChan
	if reply.err != nil {
		t.Errorf("reply.err is not nil: %v", reply.err)
	}

	// Now query that new entry
	dbChan <- dbTask{QUERY, fse, replyChan}
	reply = <-replyChan
	if len(reply.fsentries) != 1 {
		t.Errorf("reply.fsentries len: %v", len(reply.fsentries))
	}
	if reply.fsentries[0].Path != fse.Path {
		t.Errorf("paths don't match: %v", reply.fsentries[0].Path)
	}

	// Now delete that entry
	dbChan <- dbTask{DELETE, fse, replyChan}
	reply = <-replyChan
	if reply.err != nil {
		t.Errorf("reply.err is not nil: %v", reply.err)
	}

	dbChan <- dbTask{action: QUIT, replyChan: replyChan}
	<-replyChan
}

func TestIndexerShouldMessageDoneAndShutDownIfNothingOnDirChan(t *testing.T) {
	dirChan := make(chan []fsentry.E, 10)
	dbChan := make(chan dbTask, 1)
	doneChan := make(chan int, 1)

	go indexer(&indexerMateriel{idxNum: 1, dirChan: dirChan, dbChan: dbChan, doneChan: doneChan})

	select {
	case idx := <-doneChan:
		if idx != 1 {
			t.Errorf("Index: %d\n", idx)
		}
	case <-time.After(2 * time.Second):
		t.Error("indexer go routine did not respond with 'Done' in alloted time")
	}
}

func TestReadDatabaseProps(t *testing.T) {
	uname, passw, err := readDatabaseProperties()
	if err != nil {
		t.Errorf(fmt.Sprintf("%v", err))
	}
	if len(uname) == 0 {
		t.Errorf("uname is empty")
	}
	if len(passw) == 0 {
		t.Errorf("passw is empty")
	}
}

func TestIndexerOneEntryOnDirChan(t *testing.T) {
	/* ---[ SET UP ]--- */

	// create channels
	dirChan := make(chan []fsentry.E, 5)
	dbChan := make(chan dbTask, 1)
	doneChan := make(chan int, 1)
	replyChan := make(chan dbReply, 1)

	// initialize conx to the db
	db, err := initDb()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		return
	}
	defer db.Close()

	// create temp dir and files
	var (
		dirpath string
		lsFiles []string
	)
	dirpath, lsFiles = createTempDirWithFiles()
	dirpath, err = filepath.Abs(dirpath)
	if err != nil {
		panic(err)
	}
	if presentInDb(db, dirpath, lsFiles) {
		t.Errorf("Test dir and files are in the database when shouldn't be: %v, %v",
			dirpath, lsFiles)
		return
	}

	// put one entry on the dirChan for the indexer to process
	direntry := fsentry.E{Path: dirpath, Typ: fsentry.DIR}
	dirChan <- []fsentry.E{direntry}

	// now start up the handler and indexer in goroutines
	go dbHandler(db, dbChan)
	go indexer(&indexerMateriel{idxNum: 100, dirChan: dirChan, dbChan: dbChan, doneChan: doneChan})

	// wait for indexer to finish work on the one entry in dirChan
	idx := <-doneChan
	if idx != 100 {
		t.Errorf("idx wrong: %d", idx)
	}

	if !presentInDb(db, dirpath, lsFiles) {
		t.Errorf("Test dir and files are NOT in the database when should be: %v, %v",
			dirpath, lsFiles)
		return
	}

	for _, f := range lsFiles {
		f, err = filepath.Abs(f)
		if err != nil {
			panic(err)
		}
		fse := fsentry.E{Path: f, Typ: fsentry.FILE}
		dbChan <- dbTask{DELETE, fse, replyChan}
		reply := <-replyChan
		if reply.err != nil {
			t.Errorf("Unable to delete %v", f)
		}
	}

	dbChan <- dbTask{DELETE, direntry, replyChan}
	<-replyChan

	dbChan <- dbTask{action: QUIT, replyChan: replyChan}
	<-replyChan

	err = os.RemoveAll(dirpath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: unable to delete tmp dir/files: %v\n", dirpath)
	}
}

/* ---[ Helper Fns ]--- */

func presentInDb(db *sql.DB, dirpath string, lsFiles []string) bool {
	stmt, err := db.Prepare("SELECT count(*) FROM fsentry WHERE path = $1")
	if err != nil {
		panic(err)
	}
	defer stmt.Close()

	rows1, err := stmt.Query(dirpath)
	if err != nil {
		panic(err)
	}
	defer rows1.Close()

	rows1.Next()
	var cnt int
	rows1.Scan(&cnt)
	if cnt == 1 {
		return true
	}

	for _, f := range lsFiles {
		f, err = filepath.Abs(f)
		if err != nil {
			panic(err)
		}
		rows2, err := stmt.Query(f)
		if err != nil {
			panic(err)
		}
		rows2.Next()
		rows2.Scan(&cnt)
		rows2.Close()
		if cnt == 1 {
			return true
		}
	}

	return false
}

func createTempDirWithFiles() (dirpath string, lsFiles []string) {
	var err error
	dirpath, err = ioutil.TempDir(".", "")
	if err != nil {
		panic(err)
	}
	tmpFile1 := dirpath + "/" + "f1.txt"
	tmpFile2 := dirpath + "/" + "f2.txt"
	tmpFile3 := dirpath + "/" + "f3.txt"

	ioutil.WriteFile(tmpFile1, []byte("f1"), 0777)
	ioutil.WriteFile(tmpFile2, []byte("f2"), 0777)
	ioutil.WriteFile(tmpFile3, []byte("f3"), 0777)

	return dirpath, []string{tmpFile1, tmpFile2, tmpFile3}
}
