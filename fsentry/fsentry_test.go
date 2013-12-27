package fsentry

import (
	"testing"
)

func TestNewWithStringSlice(t *testing.T) {
	lsEntries := []E{
		{path: "/usr/local/foo",     typ: DIR},
		{path: "/usr/local/foo/bar", typ: FILE},
		{path: "/usr/local/foo/qux", typ: FILE},
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
		{path: "/usr/local/foo",     typ: DIR},
		{path: "/usr/local/foo",     typ: FILE},
		{path: "/usr/local/foo/bar", typ: FILE, isTopLevel: true},
		{path: "/usr/local/foo/bar", typ: FILE, isTopLevel: false},
		{path: "/usr/local/foo/qux", typ: FILE},
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
		{path: "/usr/local/foo",     typ: DIR},
		{path: "/usr/local/foo",     typ: FILE},
		{path: "/usr/local/foo/bar", typ: FILE, isTopLevel: true},
		{path: "/usr/local/foo/bar", typ: FILE, isTopLevel: false},
		{path: "/usr/local/foo/qux", typ: FILE},
	}
	set := NewSet(lsEntries...)

	for _, e := range lsEntries {
		if ! set.Contains(e) {
			t.Errorf("does not contain: %v", e)
		}
	}

	ent := E{path: "/var/log/foo", typ: FILE}
	if set.Contains(ent) {
		t.Errorf("should not contain: %v", ent)
	}
}

func TestDifferenceBothListsHaveUniqueEntries(t *testing.T) {
	ls1 := []E{
		{path: "/usr/local/foo",     typ: DIR},
		{path: "/usr/local/foo",     typ: FILE},
		{path: "/usr/local/foo/bar", typ: FILE, isTopLevel: true},
		{path: "/usr/local/foo/bar", typ: FILE, isTopLevel: false},
		{path: "/usr/local/foo/qux", typ: FILE},
	}

	ls2 := []E{
		{path: "/usr/local/foo",     typ: DIR},
		{path: "/usr/local/foo/qux", typ: FILE},
		{path: "/var/log/foo", typ: DIR},
		{path: "/var/log/foo/bar", typ: FILE},
	}

	set1 := NewSet(ls1...)
	set2 := NewSet(ls2...)

	// diff ls1 => ls2

	exp12 := []E{
		{path: "/usr/local/foo",     typ: FILE},
		{path: "/usr/local/foo/bar", typ: FILE, isTopLevel: true},
		{path: "/usr/local/foo/bar", typ: FILE, isTopLevel: false},
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
		{path: "/var/log/foo", typ: DIR},
		{path: "/var/log/foo/bar", typ: FILE},
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
