//
// mboyer combines the sqlite multi-threaded channel impl with the "boyer-db" storage
// of the boyer module, which is single threaded
//
package mboyer

import (
	"bufio"
	"bytes"
	"fmt"
	"fslocate/common"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"strings"
	"time"
)

var verbose bool

const (
	OUT_FILE      = "db/fslocate.boyer"
	INDEX_FILE    = "conf/fslocate.indexlist"
	PATH_SEP      = string(os.PathSeparator)
	DBCHAN_BUFSZ  = 4096
	DIRCHAN_BUFSZ = 10000
	BUFSZ         = 2097152 // 2MiB "boyer-db" cache before flush to disk
	RECORD_SEP    = 0x1e    // "Record Separator" char in ASCII
)

type MBoyerFsLocate struct{}

/* ---[ INDEX ]--- */

func (_ MBoyerFsLocate) Index(numIndexers int, beVerbose bool) {
	verbose = beVerbose
	nindexers := numIndexers

	tmpOut := OUT_FILE + common.RandVal()
	prn("Temp out file: " + tmpOut)
	file, err := os.Create(tmpOut)
	if err != nil {
		log.Fatalf("ERROR: %v\n", err)
	}
	defer os.Remove(tmpOut)
	defer file.Close()

	/* ---[ launch indexers ]--- */
	entryChan := make(chan string, DBCHAN_BUFSZ)
	doneChan := make(chan bool, nindexers)

	var patterns *common.IgnorePatterns = common.ReadInIgnorePatterns()

	var toplevelEntries []string = getToplevelEntries(nindexers)
	// if there are fewer entries than requested indexers then decrease
	// the number of indexers launched
	nindexers = len(toplevelEntries)
	runtime.GOMAXPROCS(nindexers + 1) // run in parallel fashion -> indexers and dbwriter in separate threads
	for _, entry := range toplevelEntries {
		prf("Indexing top level entries: %s\n", entry)
		go indexer(entryChan, doneChan, patterns, entry)
	}
	prn("indexers launched")

	streamEntriesIntoDb(entryChan, doneChan, nindexers, file)

	file.Close()
	os.Remove(OUT_FILE)
	err = os.Rename(tmpOut, OUT_FILE)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to copy new boyer db to %s: %v\n", OUT_FILE, err)
		return
	}
}

func streamEntriesIntoDb(entryChan chan string, doneChan chan bool, nindexers int, file *os.File) {
	doneCnt := 0
	timeOutCnt := 0

	var buf bytes.Buffer

LOOP:
	for {
		select {
		case entry := <-entryChan:
			err := writeEntry(&buf, file, entry)
			if err != nil {
				log.Fatalf("ERROR: %v\n", err)
			}
			prf("inserted %s into boyer-db\n", entry)

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

	padToLimit(&buf)
	flushBuffer(&buf, file)
}

// TODO: haven't dealt with case where len(entry) > BUFSZ
func writeEntry(buf *bytes.Buffer, file *os.File, entry string) error {
	// +1 to add in the size of the record separator char
	if buf.Len()+len(entry)+1 > BUFSZ {
		prf("writeEntry: PadToLimit called for entry: %s\n", entry)
		padToLimit(buf)
		flushBuffer(buf, file)
	}

	_, err := buf.WriteString(entry)
	if err != nil {
		return err
	}
	_, err = buf.WriteRune(RECORD_SEP)
	if err != nil {
		return err
	}

	if buf.Len() == BUFSZ {
		flushBuffer(buf, file)
	}
	return nil
}

func padToLimit(buf *bytes.Buffer) {
	var diff = BUFSZ - buf.Len()
	for i := 0; i < diff; i++ {
		buf.WriteRune(RECORD_SEP)
	}
}

func flushBuffer(buf *bytes.Buffer, file *os.File) error {
	_, err := file.Write(buf.Bytes())	
	file.Sync()
	buf.Reset()
	return err
}

func getToplevelEntries(nindexers int) []string {
	if !common.FileExists(INDEX_FILE) {
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

func indexer(entryChan chan string, doneChan chan bool, ignorePats *common.IgnorePatterns, dirpath string) {
	dirChan := make(chan string, DIRCHAN_BUFSZ)
	dirChan <- strings.TrimRight(dirpath, PATH_SEP)

	var err error
	numErrors := 0

	for len(dirChan) > 0 {
		prn("indexer loop")
		dir := <-dirChan
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
			fullpath := common.CreateFullPath(dir, e.Name())
			if !common.ShouldIgnore(ignorePats, fullpath) {
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
