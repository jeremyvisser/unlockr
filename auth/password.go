package auth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"

	"jeremy.visser.name/unlockr/access"
	"jeremy.visser.name/unlockr/session"
)

type AuthRequest struct {
	access.Username `json:"username"`
	Password        string `json:"password"`
}

// PasswordAuthHandler authenticates the request before passing it to the underlying Handler.
type PasswordAuthHandler struct {
	http.Handler         `json:"-"`
	access.UserStore     `json:"-"`
	session.SessionStore `json:"-"`
}

func (h *PasswordAuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case LoginURL:
		h.ServeLogin(w, r)
		return
	case LogoutURL:
		h.ServeLogout(w, r)
		return
	}
	ctx, _, s, err := session.FromRequest(r.Context(), r, h.SessionStore)
	if err != nil {
		log.Print("session not valid: ", err)
		http.Error(w, "session not valid", http.StatusUnauthorized)
		return
	}
	u, err := h.UserStore.User(ctx, s.Username)
	if err != nil {
		log.Print("user not valid: ", err)
		http.Error(w, "user not valid", http.StatusUnauthorized)
		return
	}
	ctx = u.NewContext(ctx)
	h.Handler.ServeHTTP(w, r.WithContext(ctx))
}

func (h *PasswordAuthHandler) ServeLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "must be POST", http.StatusMethodNotAllowed)
		return
	}
	var ar AuthRequest
	err := json.NewDecoder(r.Body).Decode(&ar)
	if err != nil {
		http.Error(w, "Badly formatted auth request", http.StatusBadRequest)
		return
	}
	user, err := h.UserStore.User(r.Context(), ar.Username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Print("user not found: ", err)
			http.Error(w, "Authentication error", http.StatusUnauthorized)
		} else {
			log.Print("user lookup failed: ", err)
			http.Error(w, "Internal server error while logging in", http.StatusInternalServerError)
		}
		return
	}
	err = user.Authenticate(ar.Password)
	if err != nil {
		log.Print("auth failed:", err)
		http.Error(w, "Authentication error", http.StatusUnauthorized)
		return
	}
	// User is successfully authenticated at this point, so create session:
	_, err = session.Register(user.Username, nil, w, r, h.SessionStore)
	if err != nil {
		log.Print("session registration failed:", err)
		http.Error(w, "Session registration error", http.StatusInternalServerError)
		return
	}
	if redir := r.URL.Query().Get("redirect"); redir != "" {
		newurl, err := url.Parse(redir)
		if err != nil || !newurl.IsAbs() || newurl.Host != "" {
			log.Print("bad redirect, should be relative:", err)
		} else {
			http.Redirect(w, r, newurl.RequestURI(), http.StatusSeeOther)
			return
		}
	}
	io.WriteString(w, "ok")
}

func (h *PasswordAuthHandler) ServeLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "must be POST", http.StatusMethodNotAllowed)
		return
	}
	session.Logout(w, r, h.SessionStore)
	io.WriteString(w, "bye")
}
