package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"golang.org/x/oauth2"
	"jeremy.visser.name/unlockr/access"
	"jeremy.visser.name/unlockr/debug"
	"jeremy.visser.name/unlockr/session"
)

type OAuthWordPress struct {
	ProfileURL string `json:"profileurl"` // Required
}

type WordPressUser struct {
	UserLogin   access.Username `json:"user_login"`
	UserEmail   string          `json:"user_email"`
	DisplayName string          `json:"display_name"`
	UserRoles   access.Groups   `json:"user_roles"`
}

func (w *OAuthWordPress) profileURL() string {
	if w.ProfileURL == "" {
		panic("WordPressOAuthHandler: ProfileURL is not set")
	}
	return w.ProfileURL
}

func (w *OAuthWordPress) User(ctx context.Context, username access.Username) (*access.User, error) {
	ts, ok := TokenFromContext(ctx)
	if !ok {
		return nil, errors.New("token not in context")
	}
	return w.user(ctx, ts)
}

func (w *OAuthWordPress) user(ctx context.Context, src oauth2.TokenSource) (*access.User, error) {
	client := oauth2.NewClient(ctx, src)
	req, err := http.NewRequestWithContext(ctx, "GET", w.profileURL(), nil)
	if err != nil {
		return nil, err
	}
	if debug.Debug {
		log.Printf("> %+v", req)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if sc := resp.StatusCode; 400 <= sc && sc <= 499 {
		return nil, fmt.Errorf("%w (HTTP code %d)", session.ErrSessionExpired, sc)
	} else if sc != http.StatusOK {
		return nil, fmt.Errorf("got HTTP code %d from server", sc)
	}
	var wu WordPressUser
	if err := json.NewDecoder(resp.Body).Decode(&wu); err != nil {
		return nil, err
	}
	if wu.UserLogin == "" {
		return nil, fmt.Errorf("OAuthWordPress.user: empty UserLogin: %#v", &wu)
	}
	return &access.User{
		Username: wu.UserLogin,
		Nickname: wu.DisplayName,
		Groups:   wu.UserRoles,
	}, nil
}
