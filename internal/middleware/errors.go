package middleware

import (
	"encoding/json"
	"net/http"
)

type errorBody struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := errorBody{}
	body.Error.Type = errType
	body.Error.Message = msg
	_ = json.NewEncoder(w).Encode(body)
}
