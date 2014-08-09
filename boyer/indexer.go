package boyer

import (
	"bufio"
	"bytes"
	"fmt"
	"fslocate/common"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

var verbose bool

const (
	OUT_FILE    = "db/fslocate.boyer"
	INDEX_FILE  = "conf/fslocate.indexlist"
	PATH_SEP    = string(os.PathSeparator)
	BUFSZ       = 2097152 // 2MiB cache before flush to disk
	RECORD_SEP  = 0x1e    // "Record Separator" char in ASCII
)

type BoyerFsLocate struct{}

/* ---[ INDEX ]--- */

func (_ BoyerFsLocate) Index(numIndexes int, beVerbose bool) {
	verbose = beVerbose

	tmpOut := OUT_FILE + common.RandVal()
	prn("Temp out file: " + tmpOut)
	file, err := os.Create(tmpOut)
	if err != nil {
		log.Fatalf("ERROR: %v\n", err)
	}
	defer os.Remove(tmpOut)
	defer file.Close()

	dirChan := make(chan string, 10000)
	getToplevelEntries(dirChan)
	prf("Read in %d top level entries\n", len(dirChan))
	ignorePats := common.ReadInIgnorePatterns()

	var buf bytes.Buffer
	for len(dirChan) > 0 {
		dir := <-dirChan
		prf("Procesing dir: %s\n", dir)
		err := writeEntry(&buf, file, dir)
		if err != nil {
			log.Fatalf("ERROR: %v\n", err)
		}

		var entries []os.FileInfo
		entries, err = ioutil.ReadDir(dir)
		if err != nil {
			log.Fatalf("ERROR: %v\n", err)
		}

		// processEntries(dir, entries, ignorePats, dirChan, file)
		for _, e := range entries {
			fullpath := common.CreateFullPath(dir, e.Name())
			if !common.ShouldIgnore(ignorePats, fullpath) {
				if e.IsDir() {
					dirChan <- fullpath
				} else {
					prf("Writing entry: %s\n", fullpath)
					err := writeEntry(&buf, file, fullpath)
					if err != nil {
						log.Fatalf("ERROR: %v\n", err)
					}
				}
			}
		}
	}

	padToLimit(&buf)
	flushBuffer(&buf, file)

	file.Close()
	os.Remove(OUT_FILE)
	err = os.Rename(tmpOut, OUT_FILE)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Unable to copy new boyer db to %s: %v\n", OUT_FILE, err)
		return
	}
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
	_, err := file.WriteString(buf.String()) // TODO: write buf.Bytes instead?  more performant?
	file.Sync()
	buf.Reset()
	return err
}


func getToplevelEntries(ch chan string) {
	if !common.FileExists(INDEX_FILE) {
		log.Fatal("ERROR: Cannot find file " + INDEX_FILE)
	}

	file, err := os.Open(INDEX_FILE)
	if err != nil {
		log.Fatal("ERROR: Cannot open file " + INDEX_FILE)
	}
	defer file.Close()

	scnr := bufio.NewScanner(file)
	for scnr.Scan() {
		ln := strings.TrimSpace(scnr.Text())
		if len(ln) != 0 && !strings.HasPrefix(ln, "#") {
			ch <- ln
		}
	}
	if err = scnr.Err(); err != nil {
		log.Fatalf("ERROR while reading %s: %v\n", INDEX_FILE, err)
	}
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
