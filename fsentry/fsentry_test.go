package fsentry

import (
	"testing"
)

func TestNewWithStringSlice(t *testing.T) {
	lsEntries := []E{
		E{Path: "/usr/local/foo",     Typ: DIR},
		E{Path: "/usr/local/foo/bar", Typ: FILE},
		E{Path: "/usr/local/foo/qux", Typ: FILE},
	}
	set := NewSet(lsEntries...)

	if len(set) != 3 { t.Errorf("len: %v", len(set)) }

	for _, ent := range lsEntries {
		if ! set.Contains(ent) { t.Errorf("%v", set) }
	}
}


func TestAdd(t *testing.T) {
	set := Set{}
	if len(set) != 0 {
		t.Errorf("len: %v", len(set))
	}

	lsEntries := []E{
		{Path: "/usr/local/foo",     Typ: DIR},
		{Path: "/usr/local/foo",     Typ: FILE},
		{Path: "/usr/local/foo/bar", Typ: FILE, IsTopLevel: true},
		{Path: "/usr/local/foo/bar", Typ: FILE, IsTopLevel: false},
		{Path: "/usr/local/foo/qux", Typ: FILE},
	}

	set.Add(lsEntries[0])
	if len(set) != 1 { t.Errorf("len: %v", len(set)) }
	set.Add(lsEntries[0])
	if len(set) != 1 { t.Errorf("len: %v", len(set)) }

	set.Add(lsEntries[1])
	if len(set) != 2 { t.Errorf("len: %v", len(set)) }

	set.Add(lsEntries[2])
	set.Add(lsEntries[3])
	set.Add(lsEntries[4])
	set.Add(lsEntries[2])
	set.Add(lsEntries[3])
	set.Add(lsEntries[4])
	if len(set) != 5 { t.Errorf("len: %v", len(set)) }
}

func TestContains(t *testing.T) {
	lsEntries := []E{
		{Path: "/usr/local/foo",     Typ: DIR},
		{Path: "/usr/local/foo",     Typ: FILE},
		{Path: "/usr/local/foo/bar", Typ: FILE, IsTopLevel: true},
		{Path: "/usr/local/foo/bar", Typ: FILE, IsTopLevel: false},
		{Path: "/usr/local/foo/qux", Typ: FILE},
	}
	set := NewSet(lsEntries...)

	for _, e := range lsEntries {
		if ! set.Contains(e) {
			t.Errorf("does not contain: %v", e)
		}
	}

	ent := E{Path: "/var/log/foo", Typ: FILE}
	if set.Contains(ent) {
		t.Errorf("should not contain: %v", ent)
	}
}

func TestDifferenceBothListsHaveUniqueEntries(t *testing.T) {
	ls1 := []E{
		{Path: "/usr/local/foo",     Typ: DIR},
		{Path: "/usr/local/foo",     Typ: FILE},
		{Path: "/usr/local/foo/bar", Typ: FILE, IsTopLevel: true},
		{Path: "/usr/local/foo/bar", Typ: FILE, IsTopLevel: false},
		{Path: "/usr/local/foo/qux", Typ: FILE},
	}

	ls2 := []E{
		{Path: "/usr/local/foo",     Typ: DIR},
		{Path: "/usr/local/foo/qux", Typ: FILE},
		{Path: "/var/log/foo", Typ: DIR},
		{Path: "/var/log/foo/bar", Typ: FILE},
	}

	set1 := NewSet(ls1...)
	set2 := NewSet(ls2...)

	// diff ls1 => ls2

	exp12 := []E{
		{Path: "/usr/local/foo",     Typ: FILE},
		{Path: "/usr/local/foo/bar", Typ: FILE, IsTopLevel: true},
		{Path: "/usr/local/foo/bar", Typ: FILE, IsTopLevel: false},
	}

	diffSet := set1.Difference(set2)
	if len(diffSet) != len(exp12) {
		t.Errorf("Len mismatch: %v", len(diffSet))
	}

	for _, e := range exp12 {
		if ! diffSet.Contains(e) {
			t.Errorf("does not contain: %v", e)
		}
	}

	if diffSet.Contains(ls1[0]) {
		t.Errorf("should not contain: %v", ls1[0])
	}


	// diff ls2 => ls1

	exp21 := []E{
		{Path: "/var/log/foo", Typ: DIR},
		{Path: "/var/log/foo/bar", Typ: FILE},
	}

	diffSet = set2.Difference(set1)
	if len(diffSet) != len(exp21) {
		t.Errorf("Len mismatch: %v", len(diffSet))
	}

	for _, e := range exp21 {
		if ! diffSet.Contains(e) {
			t.Errorf("does not contain: %v", e)
		}
	}

	if diffSet.Contains(ls2[0]) {
		t.Errorf("should not contain: %v", ls1[0])
	}
}
