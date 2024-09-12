package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"golang.org/x/oauth2"
	"jeremy.visser.name/go/unlockr/access"
	"jeremy.visser.name/go/unlockr/debug"
	"jeremy.visser.name/go/unlockr/session"
)

type OAuthWordPress struct {
	// BaseURL is the full base URL for WordPress, which corresponds to WP_HOME.
	// The /wp-json/ REST API should be accessible underneath here.
	// Required if ProfileURL is unset.
	BaseURL string `json:"baseurl"`

	// ProfileURL is the full URL to the /wp/v2/users/me?context=edit REST API
	// endpoint. Required if BaseURL is unset.
	ProfileURL string `json:"profileurl"`
}

type WordPressUser struct {
	Username    access.Username `json:"username"`
	Email       string          `json:"email"`
	DisplayName string          `json:"name"`
	Roles       access.Groups   `json:"roles"`
}

func (w *OAuthWordPress) profileURL() string {
	if w.ProfileURL == "" {
		if w.BaseURL != "" {
			p, err := url.JoinPath(w.BaseURL, "/wp-json/wp/v2/users/me?context=edit&_fields=username,email,name,roles")
			if err != nil {
				panic(err)
			}
			return p
		}
		panic("OAuthWordPress: BaseURL must be set to WordPress home page (underneath which /wp-json/ is located)")
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
	if debug.Debug() {
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
	if wu.Username == "" {
		return nil, fmt.Errorf("OAuthWordPress.user: empty UserLogin: %#v", &wu)
	}
	return &access.User{
		Username: wu.Username,
		Nickname: wu.DisplayName,
		Groups:   wu.Roles,
	}, nil
}
