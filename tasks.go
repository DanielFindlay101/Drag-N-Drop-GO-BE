// request JSON → decode into a Go struct → validate → INSERT into MySQL → respond 201 + the new task

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)


type Task struct {
	ID      		string     `json:"id"`
	Title			string	   `json:"title"`
	Description 	string 	   `json:"description"`
	Tag				string	   `json:"tag"`
 	Colour          string	   `json:"colour"`
	Completed		bool       `json:"completed"`
	CreatedAt   	time.Time  `json:"createdAt"`
	UpdatedAt   	time.Time  `json:"updatedAt"`
}

type createTaskRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Tag         string `json:"tag"`
	Colour      string `json:"colour"`
}

type Tag struct {
	Title string `json:"tag"`
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {

	// Decode the json body of the request
	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// check that the title isn't empty
	if strings.TrimSpace(req.Title) == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}


	// Generate the id of the task 
	id := uuid.NewString()
	_, err := s.db.Exec(
		"INSERT INTO tasks (id, title, description, tag, colour) VALUES (?, ?, ?, ?, ?)",
		id, req.Title, req.Description, req.Tag, req.Colour);
	if err != nil {
		log.Println("create task: insert:", err)
		return
	}

	// Return the created REST response
	var t Task
	err = s.db.QueryRow(
		" SELECT id, title, description, tag, colour, completed, createdAt, updatedAt FROM tasks WHERE id = ?",
		id,
	).Scan(&t.ID, &t.Title, &t.Description, &t.Tag, &t.Colour, &t.Completed, &t.CreatedAt, &t.UpdatedAt)
	 if err != nil {
		log.Println("create task: select", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	 }

	// Finally respond with the 201
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(t); err != nil {
		log.Println("create task: encode response:", err)
	}
}

func (s *Server) fetchAllTasks(w http.ResponseWriter, r *http.Request) {
	// Create the query
	rows, err := s.db.Query(`SELECT id, title, description, tag, colour, completed, createdAt, updatedAt FROM tasks`)

	if err != nil {
		log.Println("Error during query", err)
		return
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next(){
		var t Task
		
		err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Tag, &t.Colour, &t.Completed, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			log.Println("Error scanning a row", err)
			return
		}
		tasks = append(tasks, t)
	}

	if err := rows.Err(); err != nil {
		log.Println(err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(tasks); err != nil {
		log.Println("cannot fetch tasks: encode response:", err)
	}
}

func (s *Server) fetchAllTags(w http.ResponseWriter, r *http.Request){
	// Create the query
	rows, err := s.db.Query(`SELECT tag FROM tasks`)

	if err != nil {
		fmt.Println("Error fetching the tags", err)
		return
	}

	defer rows.Close()

	var tags []Tag
		for rows.Next(){
		var t Tag
		
		err := rows.Scan(&t.Title)

		if err != nil {
			log.Println("Error scanning a tag row", err)
			return
		}
		tags = append(tags,t)
	}

	if err := rows.Err(); err != nil {
		log.Println(err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(tags); err != nil {
		log.Println("cannot fetch tags: encode response:", err)
	}

}