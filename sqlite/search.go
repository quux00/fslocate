package sqlite

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"strings"
)

type SqliteFsLocate struct {}

func (_ SqliteFsLocate) Search(term string) {
	db, err := sql.Open("sqlite3", "db/fslocate.db")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	st, err := db.Prepare("select path from fsentry where lower(path) like '%" + strings.ToLower(term) + "%'")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer st.Close()

	r, err := st.Query()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer r.Close()

	for r.Next() {
		var path string
		r.Scan(&path)
		fmt.Println(path)
	}
}
