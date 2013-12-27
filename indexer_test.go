package main

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


func TestDbHandlerInsertAndDelete(t *testing.T) {
	var (
		db *sql.DB
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

	fmt.Println("Handler running")

	fse := fsentry.E{"/var/log/hive/foo", "f", false}

	// ask handler to do insert
	fmt.Println("Request to insert")
	dbChan <- dbTask{INSERT, fse, replyChan}
	reply = <- replyChan
	if reply.err != nil {
		t.Errorf("reply.err is not nil: %v", reply.err)
	}

	// fmt.Println("Now query that new entry")
	// dbChan <- dbTask(QUERY, fse, replyChan)


	fmt.Println("Now delete that entry")
	dbChan <- dbTask{DELETE, fse, replyChan}
	reply = <- replyChan
	if reply.err != nil {
		t.Errorf("reply.err is not nil: %v", reply.err)
	}

	time.Sleep(2 * time.Second)
	fmt.Println("Sending QUIT action to dbHandler")
	dbChan <- dbTask{action: QUIT, replyChan: replyChan}
	fmt.Println("Waiting for QUIT ack from dbHandler")
	<- replyChan
	fmt.Println("DONE")
}


func TestIndexerShouldMessageDoneAndShutDownIfNothingOnDirChan(t *testing.T) {
	dirChan  := make(chan fsentry.E, 10)
	dbChan   := make(chan dbTask, 1)
	doneChan := make(chan int, 1)

	go indexer(1, dirChan, dbChan, doneChan)

	select {
	case idx := <- doneChan:
		if idx != 1 {
			t.Errorf("Index: %d\n", idx)
		}
	case <- time.After(2 * time.Second):
		t.Error("indexer go routine did not respond with 'Done' in alloted time")
	}
}

func TestIndexerOneEntryOnDirChan(t *testing.T) {
	/* ---[ SET UP ]--- */

	// create channels
	dirChan   := make(chan fsentry.E, 5)
	dbChan    := make(chan dbTask, 1)
	doneChan  := make(chan int, 1)
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
	dirChan <- direntry

	// now start up the handler and indexer in goroutines
	go dbHandler(db, dbChan)
	go indexer(100, dirChan, dbChan, doneChan)

	// wait for indexer to finish work on the one entry in dirChan
	idx := <- doneChan
	if idx != 100 { t.Errorf("idx wrong: %d", idx) }

	if ! presentInDb(db, dirpath, lsFiles) {
		t.Errorf("Test dir and files are NOT in the database when should be: %v, %v",
			dirpath, lsFiles)
		return
	}

	for _, f := range lsFiles {
		f, err = filepath.Abs(f)
		if err != nil { panic(err) }
		fse := fsentry.E{Path: f, Typ: fsentry.FILE}
		dbChan <- dbTask{DELETE, fse, replyChan}
		reply := <- replyChan
		if reply.err != nil { t.Errorf("Unable to delete %v", f) }
	}

	dbChan <- dbTask{DELETE, direntry, replyChan}
	<- replyChan

	dbChan <- dbTask{action: QUIT, replyChan: replyChan}
	<- replyChan

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

	return dirpath, []string{tmpFile1, tmpFile2, tmpFile3,}
}
