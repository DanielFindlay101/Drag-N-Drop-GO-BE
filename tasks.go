// request JSON → decode into a Go struct → validate → INSERT into MySQL → respond 201 + the new task

package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Tag         string    `json:"tag"`
	Colour      string    `json:"colour"`
	Completed   bool      `json:"completed"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type createTaskRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Tag         string `json:"tag"`
	Colour      string `json:"colour"`
}

// Pointer fields tell "not sent" (nil) apart from "sent as false / empty".
type updateTaskRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Tag         *string `json:"tag"`
	Colour      *string `json:"colour"`
	Completed   *bool   `json:"completed"`
}

var allowedColours = map[string]bool{
	"green": true, "blue": true, "red": true, "yellow": true, "indigo": true,
}

const taskColumns = "id, title, description, tag, colour, completed, createdAt, updatedAt"

// getTask is shared by createTask and updateTask.
func (s *Server) getTask(id, userID string) (Task, error) {
	var t Task
	err := s.db.QueryRow(
		"SELECT "+taskColumns+" FROM tasks WHERE id = ? AND userId = ?", id, userID,
	).Scan(&t.ID, &t.Title, &t.Description, &t.Tag, &t.Colour, &t.Completed, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Decode the json body of the request
	var req createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate against the column limits in schema.sql
	title := strings.TrimSpace(req.Title)
	switch {
	case title == "" || len(title) > 255:
		http.Error(w, "title must be 1–255 characters", http.StatusBadRequest)
		return
	case len(req.Description) > 500:
		http.Error(w, "description must be at most 500 characters", http.StatusBadRequest)
		return
	case len(req.Tag) > 50:
		http.Error(w, "tag must be at most 50 characters", http.StatusBadRequest)
		return
	case !allowedColours[req.Colour]:
		http.Error(w, "unknown colour", http.StatusBadRequest)
		return
	}

	// Generate the id of the task
	id := uuid.NewString()
	_, err := s.db.Exec(
		"INSERT INTO tasks (id, title, description, tag, colour, userId) VALUES (?, ?, ?, ?, ?, ?)",
		id, title, req.Description, req.Tag, req.Colour, userID,
	)
	if err != nil {
		log.Println("create task: insert:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Read it back so the response includes completed/createdAt/updatedAt
	t, err := s.getTask(id, userID)
	if err != nil {
		log.Println("create task: select:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) fetchAllTasks(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Only fixed SQL fragments are appended; user input stays in placeholders.
	query := "SELECT " + taskColumns + " FROM tasks WHERE userId = ?"
	args := []any{userID}

	switch r.URL.Query().Get("completed") {
	case "true":
		query += " AND completed = TRUE"
	case "false":
		query += " AND completed = FALSE"
	}
	query += " ORDER BY createdAt DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		log.Println("fetch tasks: query:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Non-nil so an empty result encodes as [] rather than null
	tasks := []Task{}
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Tag, &t.Colour, &t.Completed, &t.CreatedAt, &t.UpdatedAt); err != nil {
			log.Println("fetch tasks: scan:", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		log.Println("fetch tasks: rows:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) updateTask(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := r.PathValue("id")

	var req updateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Build "col = ?" pairs only for the fields that were sent.
	var sets []string
	var args []any
	add := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}

	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" || len(title) > 255 {
			http.Error(w, "title must be 1–255 characters", http.StatusBadRequest)
			return
		}
		add("title", title)
	}
	if req.Description != nil {
		if len(*req.Description) > 500 {
			http.Error(w, "description must be at most 500 characters", http.StatusBadRequest)
			return
		}
		add("description", *req.Description)
	}
	if req.Tag != nil {
		if len(*req.Tag) > 50 {
			http.Error(w, "tag must be at most 50 characters", http.StatusBadRequest)
			return
		}
		add("tag", *req.Tag)
	}
	if req.Colour != nil {
		if !allowedColours[*req.Colour] {
			http.Error(w, "unknown colour", http.StatusBadRequest)
			return
		}
		add("colour", *req.Colour)
	}
	if req.Completed != nil {
		add("completed", *req.Completed)
	}

	if len(sets) == 0 {
		http.Error(w, "no fields provided to update", http.StatusBadRequest)
		return
	}

	args = append(args, id, userID)
	_, err := s.db.Exec(
		"UPDATE tasks SET "+strings.Join(sets, ", ")+" WHERE id = ? AND userId = ?",
		args...,
	)
	if err != nil {
		log.Println("update task:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Re-read instead of trusting RowsAffected: MySQL reports 0 when the
	// new values equal the old ones, which isn't a "not found".
	t, err := s.getTask(id, userID)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Println("update task: select:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, t)
}

func (s *Server) fetchAllTags(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromContext(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := s.db.Query(
		"SELECT DISTINCT tag FROM tasks WHERE userId = ? AND tag <> '' ORDER BY tag",
		userID,
	)
	if err != nil {
		log.Println("fetch tags: query:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	tags := []string{}
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			log.Println("fetch tags: scan:", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		log.Println("fetch tags: rows:", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, tags)
}
