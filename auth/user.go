package auth

import (
	"encoding/json"
	"log"
	"net/http"

	"jeremy.visser.name/unlockr/access"
)

func ServeUser(w http.ResponseWriter, r *http.Request) {
	user, ok := access.FromContext(r.Context())
	if !ok {
		http.Error(w, "Not logged in", http.StatusUnauthorized)
		return
	}

	// Remove unwanted fields (after copying so we don't mutate original):
	u := *user
	u.PasswordHash = ""
	u.Groups = nil

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(&u)
	if err != nil {
		log.Print("user info failed:", err)
		http.Error(w, "Error getting user info", http.StatusInternalServerError)
		return
	}
}
