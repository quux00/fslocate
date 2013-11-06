package stringset

import (
	"fmt"
	"sort"
	"testing"
)

func TestNewWithStringSlice(t *testing.T) {
	sslice := []string{"aa", "bb", "cc"}
	set := New(sslice...)

	if len(set) != 3 { t.Errorf("len: %v", len(set)) }
	if ! set.Contains("aa") { t.Errorf("%v", set) }
	if ! set.Contains("bb") { t.Errorf("%v", set) }
	if ! set.Contains("cc") { t.Errorf("%v", set) }
}

func TestAdd(t *testing.T) {
	set := StringSet{}
	if len(set) != 0 {
		t.Errorf("len: %v", len(set))
	}

	str1 := "1"
	str2 := "2"
	str3 := "3"

	set.Add(str1)
	if len(set) != 1 {
		t.Errorf("len: %v", len(set))
	}

	set.Add(str2)
	if len(set) != 2 {
		t.Errorf("len: %v", len(set))
	}

	set.Add(str3)
	if len(set) != 3 {
		t.Errorf("len: %v", len(set))
	}

	set.Add(str2)
	if len(set) != 3 {
		t.Errorf("len: %v", len(set))
	}

	set.Add(str1)
	if len(set) != 3 {
		t.Errorf("len: %v", len(set))
	}

	set.Add(str3)
	if len(set) != 3 {
		t.Errorf("len: %v", len(set))
	}

	if _, present := set[str1]; !present {
		t.Errorf("str1 not present")
	}
	if _, present := set[str2]; !present {
		t.Errorf("str2 not present")
	}
	if _, present := set[str3]; !present {
		t.Errorf("str3 not present")
	}
}

func TestStringSetContains(t *testing.T) {
	set := StringSet{}
	if len(set) != 0 {
		t.Errorf("len: %v", len(set))
	}

	str1 := "1"
	str2 := "2"
	str3 := "3"

	set.Add(str1)
	set.Add(str2)

	if !set.Contains(str1) {
		t.Errorf("str1 not present")
	}
	if !set.Contains(str2) {
		t.Errorf("str2 not present")
	}
	if set.Contains(str3) {
		t.Errorf("str3 present")
	}
}

func TestStringSetAddAll(t *testing.T) {

	set1 := StringSet{}
	set2 := StringSet{}

	str1 := "1"
	str2 := "2"
	str3 := "3"
	str4 := "4"
	str5 := "5"
	str6 := "6"

	set1.Add(str1)
	set1.Add(str2)
	set1.Add(str3)

	set2.Add(str4)
	set2.Add(str5)
	set2.Add(str6)

	if len(set1) != 3 {
		t.Errorf("len: %v", len(set1))
	}
	if len(set2) != 3 {
		t.Errorf("len: %v", len(set2))
	}

	set3 := set1.AddAll(set2)

	if len(set1) != 6 {
		t.Errorf("len: %v", len(set1))
	}
	if len(set2) != 3 {
		t.Errorf("len: %v", len(set2))
	}
	if len(set3) != 6 {
		t.Errorf("len: %v", len(set1))
	}

	if !set1.Contains(str5) {
		t.Errorf("%+v", set1)
	}
	if set2.Contains(str2) {
		t.Errorf("%v", set2)
	}

	// set3 should just be an alias for set1
	if len(set1) != len(set3) {
		t.Errorf("set1: %v; set3: %v", set1, set3)
	}
	if fmt.Sprintf("%v\n", set1) != fmt.Sprintf("%v\n", set1) {
		t.Errorf("set1: %v; set3: %v", set1, set3)
	}
}


func TestAddAllInSlice(t *testing.T) {

	sset := New("11", "22", "33")
	sslice := []string{"44", "55", "66"}

	if len(sset) != 3 {
		t.Errorf("len: %v", len(sset))
	}
	if len(sslice) != 3 {
		t.Errorf("len: %v", len(sslice))
	}

	sset = sset.AddAllInSlice(sslice)
	if len(sset) != 6 {
		t.Errorf("len: %v", len(sset))
	}
	if len(sslice) != 3 {
		t.Errorf("len: %v", len(sslice))
	}

	if ! sset.Contains("22") {
		t.Errorf("len: %v", len(sset))
	}
	if ! sset.Contains("66") {
		t.Errorf("len: %v", len(sset))
	}
}

func TestStringSetRemove(t *testing.T) {

	set := StringSet{}

	str1 := "1"
	str2 := "2"
	str3 := "3"

	set.Add(str1)
	set.Add(str2)
	set.Add(str3)

	if !set.Contains(str1) {
		t.Errorf("str1 not present")
	}
	if !set.Contains(str2) {
		t.Errorf("str2 not present")
	}
	if !set.Contains(str3) {
		t.Errorf("str3 not present")
	}

	set.Remove(str2)

	if len(set) != 2 {
		t.Errorf("len: %v", len(set))
	}
	if !set.Contains(str1) {
		t.Errorf("str1 not present")
	}
	if set.Contains(str2) {
		t.Errorf("str2 present")
	}
	if !set.Contains(str3) {
		t.Errorf("str3 not present")
	}

	set.Remove(str1)
	set.Remove(str3)
	if len(set) != 0 {
		t.Errorf("len: %v", len(set))
	}
}

func TestIsSubset(t *testing.T) {

	set1 := StringSet{}
	set2 := StringSet{}
	set3 := StringSet{}
	set4 := StringSet{}

	str1 := "1"
	str2 := "2"
	str3 := "3"
	str4 := "4"
	str5 := "5"
	str6 := "6"

	set1.Add(str1)
	set1.Add(str2)
	set1.Add(str3)

	set2.Add(str4)
	set2.Add(str5)
	set2.Add(str6)

	set3.Add(str1)
	set3.Add(str3)

	if set1.IsSubset(set2) {
		t.Errorf("set1: %v; set2: %v", set1, set2)
	}
	if set2.IsSubset(set1) {
		t.Errorf("set1: %v; set2: %v", set1, set2)
	}

	if !set3.IsSubset(set1) {
		t.Errorf("set1: %v; set3: %v", set1, set3)
	}
	if set1.IsSubset(set3) {
		t.Errorf("set1: %v; set3: %v", set1, set3)
	}

	if !set4.IsSubset(set1) {
		t.Errorf("set1: %v; set4: %v", set1, set4)
	}

	if set1.IsSubset(set4) {
		t.Errorf("set1: %v; set4: %v", set1, set4)
	}
}

func TestDifference(t *testing.T) {
	set1 := New("1", "2", "3", "4")
	set2 := New("21", "22", "3", "4", "55")

	var diffSet StringSet
	diffSet = set1.Difference(set2)

	if len(diffSet) != 2 { t.Errorf("len: %v", len(diffSet)) }
	if   diffSet.Contains("3") { t.Errorf("%v", diffSet) }
	if ! diffSet.Contains("1") { t.Errorf("%v", diffSet) }
	if ! diffSet.Contains("2") { t.Errorf("%v", diffSet) }

	diffSet = set2.Difference(set1)
	if len(diffSet) != 3 { t.Errorf("len: %v", len(diffSet)) }
	if   diffSet.Contains("3") { t.Errorf("%v", diffSet) }
	if ! diffSet.Contains("21") { t.Errorf("%v", diffSet) }
	if ! diffSet.Contains("22") { t.Errorf("%v", diffSet) }
	if ! diffSet.Contains("55") { t.Errorf("%v", diffSet) }
}


func TestSlice(t *testing.T) {
	set := New("21", "22", "3", "4", "55")
	slc := set.Slice()

	if len(slc) != len(set) { t.Errorf("%v vs. %v", len(slc), len(set)) }

	sort.Strings(slc)
	if slc[0] != "21" { t.Errorf("%v", slc) }
	if slc[1] != "22" { t.Errorf("%v", slc) }
	if slc[2] != "3" { t.Errorf("%v", slc) }
	if slc[3] != "4" { t.Errorf("%v", slc) }
	if slc[4] != "55" { t.Errorf("%v", slc) }
}
