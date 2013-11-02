package main

import (
	"database/sql"
	"fmt"
	_ "github.com/bmizerany/pq"
	"strings"
)

func Search(term string) {
	db, err := sql.Open("postgres", "user=midpeter444 password=jiffylube dbname=fslocate sslmode=disable")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	st, err := db.Prepare("select path from files where lower(path) like '%" + strings.ToLower(term) + "%'")
	if err != nil {
		fmt.Println(err)
		return
	}

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
