package main

import (
	"database/sql"
	"fslocate/fsentry"
	"fslocate/stringset"
	_ "github.com/bmizerany/pq"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

var (
	toplevelDirs []string = []string{"/a/b/c", "/d/e", "/f/g/h/III"}
	nottoplevel  string   = "/f/g/h/III/foo"
)

func TestSyncTopLevelEntries(t *testing.T) {
	// setup
	tmpdir1, err := ioutil.TempDir(".", "111")
	if err != nil {
		panic(err)
	}
	tmpdir2, err := ioutil.TempDir(".", "222")
	if err != nil {
		panic(err)
	}

	configDirs := []string{tmpdir1, tmpdir2}
	configFilePath := writeFakeConfigFile(configDirs)

	db := initTestDb()
	defer db.Close()
	populateTestDb(db)

	dbChan := make(chan dbTask, 5)
	dirChan := make(chan []fsentry.E, 10)
	replyChan := make(chan dbReply, 1)
	go dbHandler(db, dbChan)

	// run code under test
	err = syncTopLevelEntries(db, TopLevelInfo{dirChan, dbChan, configFilePath})
	if err != nil {
		t.Errorf("fail: %v", err)
	}

	// validate it put two items on the dirChan
	var entryPathsOnChan []string

LOOP:
	for len(entryPathsOnChan) < 2 {
		select {
		case dirEntries := <-dirChan:
			for _, entry := range dirEntries {
				entryPathsOnChan = append(entryPathsOnChan, entry.Path)
			}
		case <-time.After(1500 * time.Millisecond):
			t.Errorf("syncTopLevelEntries seems to not have enough entries on the dirChan: entries: %v", entryPathsOnChan)
			break LOOP
		}
	}

	sort.Strings(entryPathsOnChan)
	if entryPathsOnChan[0] != tmpdir1 {
		t.Errorf("No match: %v :: %v", entryPathsOnChan[0], tmpdir1)
	}
	if entryPathsOnChan[1] != tmpdir2 {
		t.Errorf("No match: %v :: %v", entryPathsOnChan[1], tmpdir2)
	}

	// cleanup
	dbChan <- dbTask{action: QUIT, replyChan: replyChan}
	<-replyChan
	deleteFromTestDb(db)
	os.RemoveAll(filepath.Dir(configFilePath))
	os.RemoveAll(tmpdir1)
	os.RemoveAll(tmpdir2)
}

// helper
func writeFakeConfigFile(configDirs []string) (configFilePath string) {
	tmpdir, err := ioutil.TempDir(".", "")
	if err != nil {
		panic(err)
	}

	configFilePath = tmpdir + "/dirs.config"
	fout, err := os.Create(tmpdir + "/dirs.config")
	defer fout.Close()

	for _, dirpath := range configDirs {
		_, err := fout.WriteString(dirpath + "\n")
		if err != nil {
			panic(err)
		}
	}
	fout.Sync()

	return configFilePath
}

func TestPathsToDeleteInDbDbDirIsChildOfConfigDir(t *testing.T) {
	configDirs := []string{"/d/e", "/new/not/in/db", "/a/b"}
	deleteDirs := pathsToDeleteInDb(configDirs, toplevelDirs)

	sort.Strings(deleteDirs)

	if len(deleteDirs) != 2 {
		t.Errorf("len: %v", len(deleteDirs))
	}
	// topleveDirs[0] will NOT have a % attached
	expectedDelSet := stringset.New(toplevelDirs[0], toplevelDirs[2])
	actualDelSet := stringset.New(deleteDirs...)

	diffSet := expectedDelSet.Difference(actualDelSet)
	if len(diffSet) != 0 {
		t.Errorf("Differences not 0: %v; \n%v; \n%v", diffSet, expectedDelSet, actualDelSet)
	}
}

func TestPathsToDeleteInDbConfigDirIsChildOfDbDir(t *testing.T) {
	configDirs := []string{"/d/e", "/new/not/in/db", "/a/b/c/foo/bar"}
	deleteDirs := pathsToDeleteInDb(configDirs, toplevelDirs)

	sort.Strings(deleteDirs)

	if len(deleteDirs) != 2 {
		t.Errorf("len: %v", len(deleteDirs))
	}
	expectedDelSet := stringset.New(toplevelDirs[0], toplevelDirs[2])
	actualDelSet := stringset.New(deleteDirs...)

	diffSet := expectedDelSet.Difference(actualDelSet)
	if len(diffSet) != 0 {
		t.Errorf("Differences not 0: %v; \n%v; \n%v", diffSet, expectedDelSet, actualDelSet)
	}
}

func TestPathsToDeleteInDbUnrelatedDirs(t *testing.T) {
	configDirs := []string{"/foo/bar", "/d/e", "/new/not/in/db"}
	deleteDirs := pathsToDeleteInDb(configDirs, toplevelDirs)
	sort.Strings(deleteDirs)

	if len(deleteDirs) != 2 {
		t.Errorf("len: %v", len(deleteDirs))
	}

	expectedDelSet := stringset.New(toplevelDirs[0], toplevelDirs[2])
	actualDelSet := stringset.New(deleteDirs...)

	diffSet := expectedDelSet.Difference(actualDelSet)
	if len(diffSet) != 0 {
		t.Errorf("Differences not 0: %v; \n%v; \n%v", diffSet, expectedDelSet, actualDelSet)
	}
}

func TestTopLevelDirsInDb(t *testing.T) {

	db := initTestDb()
	defer db.Close()
	populateTestDb(db)

	// do test here
	dbIndexDirs, err := toplevelDirsInDb(db)
	if err != nil {
		panic(err)
	}

	if len(dbIndexDirs) != 3 {
		t.Errorf("len: %v", len(dbIndexDirs))
	}

	sort.Strings(dbIndexDirs)
	for i, tld := range toplevelDirs {
		if dbIndexDirs[i] != tld {
			t.Errorf("%v", dbIndexDirs[i])
		}
	}

	deleteFromTestDb(db)
}

/* ---[ Helper fns ]--- */

func initTestDb() *sql.DB {
	db, err := sql.Open("postgres", "user=midpeter444 password=jiffylube dbname=testfslocate sslmode=disable")
	if err != nil {
		panic(err)
	}
	return db
}

func populateTestDb(db *sql.DB) {

	deleteFromTestDb(db)

	for _, tld := range toplevelDirs {
		_, err := db.Exec("INSERT INTO fsentry (path, type, toplevel) VALUES ($1, 'd', 't')", tld)
		if err != nil {
			panic(err)
		}
	}

	_, err := db.Exec("INSERT INTO fsentry (path, type, toplevel) VALUES ($1, 'd', 'f')", nottoplevel)
	if err != nil {
		panic(err)
	}
}

func deleteFromTestDb(db *sql.DB) {
	for _, tld := range toplevelDirs {
		_, err := db.Exec("DELETE FROM fsentry WHERE path = $1", tld)
		if err != nil {
			panic(err)
		}
	}

	_, err := db.Exec("DELETE FROM fsentry WHERE path = $1", nottoplevel)
	if err != nil {
		panic(err)
	}
}
