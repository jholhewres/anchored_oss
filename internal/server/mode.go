package server

import (
	"encoding/json"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/config"
)

type modeResponse struct {
	Mode string `json:"mode"`
}

func modeHandler(cfg *config.Config) http.HandlerFunc {
	mode := "selfhosted"
	if cfg.IsCloud() {
		mode = "cloud"
	}

	resp := modeResponse{Mode: mode}

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
