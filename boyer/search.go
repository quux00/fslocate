//
// The original goal of this package was to use a boyer-moore string
// search through a simple textual database format.  For now I'm
// just using the bytes.Index function to search through the textual
// database, since it has been fast enough for the usage scenarios so far.
//
package boyer

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
)

func (_ BoyerFsLocate) Search(s string) {

	file, err := os.Open(OUT_FILE)
	if err != nil {
		log.Fatalf("ERROR 1: %v\n", err)
	}
	defer file.Close()

	needle := []byte(s)
	var rb []byte
	b := make([]byte, BUFSZ)

	for {
		n, err := file.Read(b)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("ERROR 2: %v\n", err)
		}
		if n <= 0 {
			break
		}
		rb = b[0:n]

		for {
			n = bytes.Index(rb, needle)
			if n < 0 {
				break
			}
			entry, endpos := extractEntry(rb, n)
			fmt.Println(string(entry))
			rb = rb[endpos+1:]
		}
	}
}

//
// extractEntry searches back and forward for RECORD_SEP
// and returns the byte slice between them.  It also returns
// the ending position of the byte slice
//
func extractEntry(b []byte, pos int) ([]byte, int) {
	var start, end int

	for start = pos; start > 0; start-- {
		if b[start] == RECORD_SEP {
			start++
			break
		}
	}

	for end = pos; end < len(b); end++ {
		if b[end] == RECORD_SEP {
			break
		}
	}

	return b[start:end], end
}
