package main

import (
	"fmt"
	"os"
    _ "github.com/bmizerany/pq"
    "database/sql"
)

func main() {
	if (len(os.Args) != 2) {
		fmt.Println("ERROR: must provide search string on cmd line")
	}

	// db, err := sql.Open("postgres", "user=midpeter444 password=jiffylube dbname=sakila sslmode=disable")
    db, err := sql.Open("postgres", "user=midpeter444 password=jiffylube dbname=fslocate sslmode=disable")
    if (err != nil) {
		fmt.Println(err)
		return
	}
	defer db.Close()
	
	// st, err := db.Prepare("select first_name, last_name from actor where first_name like '%" + os.Args[1] + "%'")
	st, err := db.Prepare("select path from files where path like '%" + os.Args[1] + "%'")
    if (err != nil) {
		fmt.Println(err)
		return
	}

	r, err := st.Query()
    if (err != nil) {
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
