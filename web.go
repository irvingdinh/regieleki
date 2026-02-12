package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

//go:embed index.html
var indexHTML embed.FS

type WebServer struct {
	store *Store
	token string
	srv   *http.Server
}

func NewWebServer(store *Store, token string) *WebServer {
	return &WebServer{store: store, token: token}
}

func (s *WebServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/records", s.handleList)
	mux.HandleFunc("POST /api/records", s.handleCreate)
	mux.HandleFunc("PUT /api/records/{id}", s.handleUpdate)
	mux.HandleFunc("DELETE /api/records/{id}", s.handleDelete)
	mux.Handle("GET /", http.FileServer(http.FS(indexHTML)))
	if s.token != "" {
		return requireAuth(s.token, mux)
	}
	return mux
}

func (s *WebServer) ListenAndServe(addr string) error {
	s.srv = &http.Server{
		Addr:         addr,
		Handler:      s.Handler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	slog.Info("http server listening", "addr", addr)
	return s.srv.ListenAndServe()
}

func (s *WebServer) Shutdown(ctx context.Context) error {
	if s.srv != nil {
		return s.srv.Shutdown(ctx)
	}
	return nil
}

func (s *WebServer) handleList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.store.List())
}

func (s *WebServer) handleCreate(w http.ResponseWriter, r *http.Request) {
	var rec Record
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if err := validateRecord(&rec); err != "" {
		jsonError(w, err, http.StatusBadRequest)
		return
	}

	created, saveErr := s.store.Add(rec)
	if saveErr != nil {
		jsonError(w, "failed to save", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (s *WebServer) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	var rec Record
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		jsonError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	if err := validateRecord(&rec); err != "" {
		jsonError(w, err, http.StatusBadRequest)
		return
	}

	updated, saveErr := s.store.Update(id, rec.Domain, rec.Type, rec.Value)
	if saveErr != nil {
		if errors.Is(saveErr, os.ErrNotExist) {
			jsonError(w, "record not found", http.StatusNotFound)
		} else {
			jsonError(w, "failed to save", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

func (s *WebServer) handleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	if err := s.store.Delete(id); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			jsonError(w, "record not found", http.StatusNotFound)
		} else {
			jsonError(w, "failed to save", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func validateRecord(r *Record) string {
	r.Domain = strings.TrimSpace(r.Domain)
	r.Value = strings.TrimSpace(r.Value)
	r.Type = strings.ToUpper(strings.TrimSpace(r.Type))

	if r.Domain == "" {
		return "domain is required"
	}
	if r.Value == "" {
		return "value is required"
	}

	switch r.Type {
	case "A":
		ip := net.ParseIP(r.Value)
		if ip == nil || ip.To4() == nil {
			return "invalid IPv4 address"
		}
	case "AAAA":
		ip := net.ParseIP(r.Value)
		if ip == nil || ip.To4() != nil {
			return "invalid IPv6 address"
		}
	case "CNAME":
		if strings.ContainsAny(r.Value, " \t") {
			return "invalid CNAME target"
		}
	default:
		return "type must be A, AAAA, or CNAME"
	}

	return ""
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
