package common

import (
	"bufio"
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

const IgnoreFile = "conf/fslocate.ignore"

type IgnorePatterns struct {
	suffixes []string
	patterns []string
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

//
// Reads in the ingore patterns from IgnoreFile
// and returns the entries as an IgnorePatterns struct
//
func ReadInIgnorePatterns() *IgnorePatterns {
	var suffixes, patterns []string

	if !FileExists(IgnoreFile) {
		fmt.Fprintf(os.Stderr, "WARN: Unable to find ignore patterns file: %v\n", IgnoreFile)
		return nil
	}

	file, err := os.Open(IgnoreFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARN: Unable to open file for reading: %v\n", IgnoreFile)
		return nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ln := strings.TrimSpace(scanner.Text())
		if len(ln) != 0 && !strings.HasPrefix(ln, "#") {
			suffixes, patterns = CategorizeIgnorePattern(suffixes, patterns, ln)
		}
	}

	if err = scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: Error reading in %v: %v\n", IgnoreFile, err)
	}
	return &IgnorePatterns{suffixes: suffixes, patterns: patterns}
}

//
// Uses the ignore patterns to determine if the file/dir passed in should
// not be indexed. The full path (abspath) is checked as a pure string match first.
// If that is not found in the ignore patterns, then a regex based search is done (??)
//
func ShouldIgnore(ignore *IgnorePatterns, abspath string) bool {
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

func CategorizeIgnorePattern(suffixes, patterns []string, token string) ([]string, []string) {
	tok := token
	if strings.HasPrefix(tok, "*") {
		tok = tok[1:]
		suffixes = append(suffixes, tok)
	} else if strings.HasSuffix(tok, "/") {
		suffixes = append(suffixes, EnsurePrefix(tok[:len(tok)-1], "/"))
		patterns = append(patterns, EnsurePrefix(tok, "/"))
	} else {
		patterns = append(patterns, EnsurePrefix(tok, "/"))
	}
	return suffixes, patterns
}

func EnsurePrefix(s string, prefix string) string {
	if strings.HasPrefix(s, prefix) {
		return s
	}
	return prefix + s
}

func FileExists(fpath string) bool {
	_, err := os.Stat(fpath)
	return err == nil
}

func RandVal() string {
	n := rand.Intn(9999999999)
	return strconv.Itoa(n)
}

func CreateFullPath(dir, fname string) string {
	var buf bytes.Buffer
	buf.WriteString(dir)
	buf.WriteRune(os.PathSeparator)
	buf.WriteString(fname)
	return buf.String()
}
