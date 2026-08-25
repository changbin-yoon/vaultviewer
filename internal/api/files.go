package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/accesslens/accesslens/internal/auth"
	"github.com/accesslens/accesslens/internal/model"
	"github.com/accesslens/accesslens/internal/ontology"
)

func registerFileRoutes(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("GET /api/tree", auth.RequireAuth(d.Sessions, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		items, err := d.Engine.List(r.URL.Query().Get("path"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	}))

	mux.HandleFunc("GET /api/search", auth.RequireAuth(d.Sessions, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		results, err := d.Engine.Search(r.URL.Query().Get("q"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}))

	// The vault-wide note graph (nodes + typed/untyped edges), for
	// consumers other than the frontend's own graph view — an AI agent,
	// a script, an MCP server — that want the ontology without fetching
	// every note and re-parsing frontmatter/wikilinks themselves. Read
	// access only, same tier as /api/tree and /api/search.
	mux.HandleFunc("GET /api/graph", auth.RequireAuth(d.Sessions, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		graph, err := ontology.Build(d.Engine)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(graph)
	}))

	mux.HandleFunc("POST /api/namespace", auth.RequireWrite(d.Sessions, func(w http.ResponseWriter, r *http.Request, user model.User) {
		if err := d.Engine.CreateNamespace(r.URL.Query().Get("path"), user.Username); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("PUT /api/rename", auth.RequireWrite(d.Sessions, func(w http.ResponseWriter, r *http.Request, user model.User) {
		from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
		if err := d.Engine.Rename(from, to, user.Username, r.URL.Query().Get("reason")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/file", auth.RequireAuth(d.Sessions, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		file, err := d.Engine.Read(r.URL.Query().Get("path"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(file)
	}))

	saveFile := auth.RequireWrite(d.Sessions, func(w http.ResponseWriter, r *http.Request, user model.User) {
		r.Body = http.MaxBytesReader(w, r.Body, d.MaxBodyBytes)
		content, err := io.ReadAll(r.Body)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				http.Error(w, fmt.Sprintf("request body exceeds %d byte limit", d.MaxBodyBytes), http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		if err := d.Engine.Save(r.URL.Query().Get("path"), content, user.Username, r.URL.Query().Get("reason")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PUT /api/file", saveFile)
	mux.HandleFunc("POST /api/file", saveFile)

	mux.HandleFunc("DELETE /api/file", auth.RequireDelete(d.Sessions, func(w http.ResponseWriter, r *http.Request, user model.User) {
		if err := d.Engine.Delete(r.URL.Query().Get("path"), user.Username); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("GET /api/history", auth.RequireAuth(d.Sessions, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		history, err := d.Engine.GetHistory(r.URL.Query().Get("path"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(history)
	}))
}
