package stringset

import (
	"bytes"
)

type Set map[string]bool

// Note: don't need a pointer to Set here bcs it is a
//       map under the hood and map ref
func (set Set) Add(st string) {
	set[st] = true
}

// AddAll adds all entries in the second set (sst) to the
// first set (the called object, 'set')
// Returns the called object with the newly added entries
// in order to allow call chaining. It is safe to ignore the
// return value if desired.
func (set Set) AddAll(sst Set) Set {
	for k := range sst {
		set[k] = true
	}
	return set
}

// AddAll adds all entries in the string slice (ssl) to the set.
// Returns the called object with the newly added entries
// in order to allow call chaining. It is safe to ignore the
// return value if desired.
func (set Set) AddAllInSlice(ssl []string) Set {
	for _, str := range ssl {
		set[str] = true
	}
	return set
}

func New(args ...string) Set {
	sset := Set{}
	for _, s := range args {
		sset.Add(s)
	}
	return sset
}


// Difference creates a new Set containing the strings
// in set that are not in set2.
func (set Set) Difference(set2 Set) Set {
	diffSet := New()
	for k := range set {
		if ! set2.Contains(k) {
			diffSet.Add(k)
		}
	}
	return diffSet
}

// 
// IsSubset returns true if the all values in the set passed in as
// an argument (sst) are in the values of the set the method was
// called on.
// Returns true if the set passed in (sst) is the empty set.
// TODO: can nil be passed in?
// 
func (set Set) IsSubset(sst Set) bool {
	for k := range set {
		if !sst[k] {
			return false
		}
	}
	return true
}

func (set Set) Remove(st string) {
	delete(set, st)
}

func (set Set) Contains(st string) bool {
	_, present := set[st]
	return present
}

func (set Set) Slice() []string {
	slc := make([]string, 0, len(set))
	for k := range set {
		slc = append(slc, k)
	}
	return slc
}

func (set Set) String() string {
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
	
