package main

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"fslocate/fsentry"
	"fslocate/stringset"
	"os"
	"strings"
)

type TopLevelInfo struct {
	dirChan        chan []fsentry.E
	dbChan         chan dbTask
	configFilePath string
}

//
// syncTopLevelEntries is the main entry point for handling
// "toplevel" entries in the database and the user's config file.
// This method reads in the user config file of the directories
// to index, sync it to what top level entries are the database
// (which might require delete entries from the DB) and putting
// the dirs to search on the dbChan and the entries to delete
// from the db on the dbChan for the goroutines to process.
//
// When called the dbHandler goroutine should already be running,
// otherwise this fn will hang.  The indexer goroutine does not
// have to be running.
//
func syncTopLevelEntries(db *sql.DB, params TopLevelInfo) error {
	var (
		err          error
		topIndexDirs []string
	)

	topIndexDirs, err = readInTopLevelIndexDirs(params.configFilePath)
	if err != nil {
		return err
	}
	if len(topIndexDirs) == 0 {
		return errors.New("No directories to index listed in the config file")
	}

	dbIndexDirs, err := toplevelDirsInDb(db)
	if err != nil {
		return err
	}
	prf("DB-Index-Dirs: %v\n", dbIndexDirs)

	delPaths := pathsToDeleteInDb(topIndexDirs, dbIndexDirs)
	replyChan := make(chan dbReply)
	for _, delpath := range delPaths {
		params.dbChan <- dbTask{DELETE, fsentry.E{delpath, fsentry.DIR, true}, replyChan}
		reply := <-replyChan
		if reply.err != nil {
			return reply.err
		}
	}

	// ensure that the top level conf index dirs exist and are dirs
	var dirEntries []fsentry.E
	for _, dir := range topIndexDirs {
		prf("Checking if %v is a directory\n", dir)
		if finfo, err := os.Stat(dir); err != nil {
			return err
		} else if !finfo.IsDir() {
			return fmt.Errorf("ERROR: %v in in the config indexdir is not a directory", dir)
		}
		dirEntries = append(dirEntries, fsentry.E{dir, fsentry.DIR, true})
	}
	prf("Putting %v on dirChan\n", dirEntries)
	params.dirChan <- dirEntries

	return nil
}

//
// readInTopLevelIndexDirs reads in from the fslocate config file that lists
// all the root directories to search and index.  It returns a list of
// strings - each a path to search.  The config file is assumed to have
// one path entry per line.
// If the config file cannot be found or read, a warning is printed to STDERR
// and an empty string slice is returned
//
func readInTopLevelIndexDirs(configFilePath string) ([]string, error) {
	indexDirsPath := configFilePath
	var indexDirs []string

	if !fileExists(indexDirsPath) {
		return indexDirs, fmt.Errorf("Unable to find conf file: %v\n", indexDirsPath)
	}

	file, err := os.Open(indexDirsPath)
	if err != nil {
		return indexDirs, err
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
		return indexDirs, err
	}

	return indexDirs, nil
}

//
// Returns ls of all paths in the database that are marked as toplevel
//
func toplevelDirsInDb(db *sql.DB) (dbIndexDirs []string, err error) {
	var (
		rows *sql.Rows
	)
	rows, err = db.Query("SELECT path FROM fsentry WHERE toplevel = true")
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

//
// pathsToDeleteInDb determines what "top level" entries to delete
// from the databasea. The values it returns may have SQL wildcards (%), so delete
// stmts should be done with 'like' not '='
//
func pathsToDeleteInDb(confIndexDirs, dbIndexDirs []string) []string {

	dbset := stringset.New(dbIndexDirs...)
	confset := stringset.New(confIndexDirs...)
	inDbOnlySet := dbset.Difference(confset)
	delPaths := make([]string, 0, 1)

	for dbdir := range inDbOnlySet {
		delPaths = append(delPaths, dbdir+"%")
	}

	return delPaths
}
