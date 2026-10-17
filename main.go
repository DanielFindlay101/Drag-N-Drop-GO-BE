package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	_ "github.com/go-sql-driver/mysql"
)

type Server struct {
	db *sql.DB
}


func main() {

	//Connection url to docker	
	db, err := sql.Open("mysql", "user:password@tcp(127.0.0.1:3306)/go-todo-app?parseTime=true") 
	if err != nil {
		log.Fatal(err)
	}

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	srv := &Server{db: db}
	http.HandleFunc("GET /tasks", srv.fetchAllTasks)
	http.HandleFunc("POST /tasks", srv.createTask)
	http.HandleFunc("GET /tags", srv.fetchAllTags)


	fmt.Println("Server is running")

	
	log.Fatal(http.ListenAndServe(":80", nil))
}