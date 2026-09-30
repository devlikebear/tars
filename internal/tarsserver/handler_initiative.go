package tarsserver

import (
	"net/http"

	"github.com/devlikebear/tars/internal/initiative"
)

func newInitiativeAPIHandler(runtime *initiative.Runtime) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/initiative/status", func(w http.ResponseWriter, r *http.Request) {
		if !requireMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, runtime.Snapshot())
	})
	return mux
}
