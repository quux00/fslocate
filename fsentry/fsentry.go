package fsentry

const (
	DIR  = "d"
	FILE = "f"
)

// corresponds to the fslocate database table 'fsentry'
type E struct {
	Path       string // full path for file or dir
	Typ        string // DIR_TYPE or FILE_TYPE
	IsTopLevel bool   // true = specified in the user's config/index file
}


type Set map[E]bool

func NewSet(args ...E) Set {
	fsset := Set{}
	for _, e := range args {
		fsset.Add(e)
	}
	return fsset
}

// Note: don't need a pointer to Set here bcs it is a
//       map under the hood and map ref
func (set Set) Add(e E) {
	set[e] = true
}

// Difference creates a new Set containing the strings
// in set that are not in set2.
func (set Set) Difference(set2 Set) Set {
	diffSet := NewSet()
	for k := range set {
		if ! set2.Contains(k) {
			diffSet.Add(k)
		}
	}
	return diffSet
}


func (set Set) Contains(entry E) bool {
	_, present := set[entry]
	return present
}

