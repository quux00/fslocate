package stringset

import (
	"bytes"
)

type StringSet map[string]bool

// Note: don't need a pointer to StringSet here bcs it is a
//       map under the hood and map ref
func (set StringSet) Add(st string) {
	set[st] = true
}

// AddAll adds all entries in the second set (sst) to the
// first set (the called object, 'set')
// Returns the called object with the newly added entries
// in order to allow call chaining. It is safe to ignore the
// return value if desired.
func (set StringSet) AddAll(sst StringSet) StringSet {
	for k := range sst {
		set[k] = true
	}
	return set
}

// AddAll adds all entries in the string slice (ssl) to the set.
// Returns the called object with the newly added entries
// in order to allow call chaining. It is safe to ignore the
// return value if desired.
func (set StringSet) AddAllInSlice(ssl []string) StringSet {
	for _, str := range ssl {
		set[str] = true
	}
	return set
}

func New(args ...string) StringSet {
	sset := StringSet{}
	for _, s := range args {
		sset.Add(s)
	}
	return sset
}


func (set StringSet) Difference(set2 StringSet) {
	diffSet := make(StringSet, 0, max(len(set), len(set2)))
	// LEFT OFF: need to do difference
}

// IsSubset returns true if the all values in the set passed in as
// an argument (sst) are in the values of the set the method was
// called on.
// Returns true if the set passed in (sst) is the empty set.
// TODO: can nil be passed in?
func (set StringSet) IsSubset(sst StringSet) bool {
	for k := range set {
		if !sst[k] {
			return false
		}
	}
	return true
}

func (set StringSet) Remove(st string) {
	delete(set, st)
}

func (set StringSet) Contains(st string) bool {
	_, present := set[st]
	return present
}

func (set StringSet) String() string {
	var buffer bytes.Buffer
	buffer.WriteString("[")
	first := true
	for k := range set {
		if !first {
			buffer.WriteString(",")
		}
		buffer.WriteString("\"")
		buffer.WriteString(k)
		buffer.WriteString("\"")
		first = false
	}
	buffer.WriteString("]")
	return buffer.String()
}

func max(a, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}
	