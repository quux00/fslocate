package mboyer

import (
	"fslocate/boyer"
)

func (_ MBoyerFsLocate) Search(s string) {
	bfsl := boyer.BoyerFsLocate{}
	bfsl.Search(s)
}
