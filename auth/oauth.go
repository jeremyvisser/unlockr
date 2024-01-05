package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/oauth2"
	"jeremy.visser.name/unlockr/access"
	"jeremy.visser.name/unlockr/debug"
	"jeremy.visser.name/unlockr/session"
	"jeremy.visser.name/unlockr/store"
)

type csrfTokens struct {
	cache *lru.Cache[string, struct{}]
}

const csrfTokenLength int = 100
const userStoreLength int = 300

type OAuthHandler struct {
	*oauth2.Config
	Profile         *jsonOAuthProfile `json:"profile"`
	PostRedirectURL string            `json:"postredirecturl"`

	http.Handler `json:"-"`

	SessionStore session.SessionStore `json:"-"`
	users        store.UserStoreCache

	csrfTokens *csrfTokens
}

type OAuthProfile interface {
	access.UserStore
	user(context.Context, oauth2.TokenSource) (*access.User, error)
}

type jsonOAuthProfile struct {
	OAuthProfile
}

type profileType string

const (
	profileWordPress profileType = "wordpress"
)

func (j *jsonOAuthProfile) UnmarshalJSON(v []byte) error {
	var t struct {
		Type profileType `json:"type"`
	}
	if err := json.Unmarshal(v, &t); err != nil {
		return err
	}
	switch t.Type {
	case profileWordPress:
		j.OAuthProfile = new(OAuthWordPress)
	default:
		return fmt.Errorf("unsupported OAuth profile: %v", t)
	}
	return json.Unmarshal(v, j.OAuthProfile)
}

func (h *OAuthHandler) init() error {
	if h.Profile == nil {
		log.Fatal("OAuth profile must be set (so we can get user profiles)")
	}
	if h.Config.RedirectURL == "" {
		h.Config.RedirectURL = OAuthRedirectURL
	}
	if h.csrfTokens == nil {
		h.csrfTokens = newCSRFTokens()
	}
	if h.users.Length <= 0 {
		h.users.Length = userStoreLength
	}
	if h.users.UserStore == nil {
		h.users.UserStore = h.Profile
	}
	return nil
}

func (h *OAuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	{
		h.init()
		switch r.URL.Path {
		case LoginURL:
			h.RedirectLogin(w, r)
			return
		case OAuthRedirectURL:
			h.ServeExchange(w, r)
			return
		}

		// Put the Session into Context:
		ctx, id, s, err := session.FromRequest(r.Context(), r, h.SessionStore)
		if errors.Is(err, session.ErrNoSession) || errors.Is(err, session.ErrSessionExpired) {
			goto errLogin
		} else if err != nil {
			log.Print("session.FromRequest: ", err)
			http.Error(w, "Error retrieving session", http.StatusInternalServerError)
			return
		}

		// Put the OAuth token into Context:
		ctx = NewContextToken(ctx, NewSessionTokenSource(ctx, id, s, h.SessionStore, h.Config))

		// Put the User into Context:
		u, err := h.users.User(ctx, s.Username)
		if _, isOAuthError := err.(*oauth2.RetrieveError); errors.Is(err, session.ErrNoSession) || errors.Is(err, session.ErrSessionExpired) || isOAuthError {
			log.Print("OAuthHandler: user auth expired: ", err)
			s.Expire(ctx, w, id, h.SessionStore)
			goto errLogin
		} else if err != nil {
			log.Print("OAuthHandler: retrieving user failed: ", err)
			goto errLogin
		}
		ctx = u.NewContext(ctx)

		// Handlers may retrieve the above values from Context:
		h.Handler.ServeHTTP(w, r.WithContext(ctx))
		return
	}

errLogin:
	http.Redirect(w, r, LoginURL, http.StatusSeeOther)
}

func (h *OAuthHandler) RedirectLogin(w http.ResponseWriter, r *http.Request) {
	ctok, err := h.csrfTokens.New()
	if err != nil {
		http.Error(w, "Internal error (generating CSRF token)", http.StatusInternalServerError)
		return
	}
	url := h.Config.AuthCodeURL(ctok, oauth2.AccessTypeOnline)
	http.Redirect(w, r, url, http.StatusSeeOther)
}

// ServeExchange gets an OAuth token, looks up the user, and creates a session.
func (h *OAuthHandler) ServeExchange(w http.ResponseWriter, r *http.Request) {
	ctok := r.FormValue("state")
	if ctok == "" || !h.csrfTokens.Valid(ctok) {
		log.Print("OAuthHandler: exchange: invalid CSRF token")
		http.Error(w, "Invalid CSRF token", http.StatusBadRequest)
		return
	}

	code := r.FormValue("code")
	token, err := h.Config.Exchange(r.Context(), code)
	if err != nil {
		log.Printf("OAuthHandler: exchange: %v", err)
		http.Error(w, "Token exchange failed", http.StatusBadRequest)
		return
	}

	// We call the OAuth-specific user(), not User(), because we don't know the username yet:
	user, err := h.Profile.user(r.Context(), oauth2.StaticTokenSource(token))
	if err != nil {
		log.Printf("OAuthHandler: failed getting user: %v", err)
		http.Error(w, "Please try logging in again. (User profile failed)", http.StatusInternalServerError)
		return
	}
	h.users.CacheUser(user.Username, user)

	extra, err := json.Marshal(token)
	if err != nil {
		log.Printf("OAuthHandler: failed to json encode tokens: %v", err)
		http.Error(w, "Failed to create session. Try again.", http.StatusInternalServerError)
		return
	}

	_, err = session.Register(user.Username, extra, w, r, h.SessionStore)
	if err != nil {
		log.Printf("OAuthHandler: failed creating session: %v", err)
		http.Error(w, "Failed to create session. Try again.", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, h.PostRedirectURL, http.StatusSeeOther)
}

func (h *OAuthHandler) ServeLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "must be POST", http.StatusMethodNotAllowed)
		return
	}
	session.Logout(w, r, h.SessionStore)
	io.WriteString(w, "bye")
}

func newCSRFTokens() *csrfTokens {
	ct := new(csrfTokens)
	cache, err := lru.New[string, struct{}](csrfTokenLength)
	if err != nil {
		panic(err)
	}
	ct.cache = cache
	return ct
}

func (c csrfTokens) Valid(token string) bool {
	return c.cache.Contains(token)
}

func (c csrfTokens) New() (token string, err error) {
	buf := make([]byte, 15)
	_, err = rand.Read(buf)
	if err != nil {
		return "", err
	}
	tok := base64.URLEncoding.EncodeToString(buf)
	c.cache.Add(tok, struct{}{})
	return tok, nil
}

func NewSessionTokenSource(ctx context.Context,
	id session.SessionId,
	s *session.Session,
	ss session.SessionStore,
	cfg *oauth2.Config) *SessionTokenSource {

	return &SessionTokenSource{
		ctx: ctx,
		id:  id,
		s:   s,
		ss:  ss,
		cfg: cfg,
	}
}

type SessionTokenSource struct {
	ctx context.Context

	id session.SessionId
	s  *session.Session
	ss session.SessionStore

	cfg *oauth2.Config

	_t *oauth2.Token // leave unset, since it's derived from s.Extra
}

// Token wraps an oauth2.TokenSource and keeps the underlying Session.Extra
// in sync, calling SaveSession if the Token is refreshed.
//
// The mechanism is rather convoluted due to a number of restrictions, such as
// the token refreshers being unexported by oauth2, the MySQL driver needing
// json.RawMessage (rather than a json.Marshaler), and that a lot of context
// is needed to refresh the token.
func (st *SessionTokenSource) Token() (*oauth2.Token, error) {
	if st._t == nil {
		err := json.Unmarshal(st.s.Extra, &st._t)
		if err != nil {
			return nil, err
		}
	}
	ts := st.cfg.TokenSource(st.ctx, st._t)
	t, err := ts.Token()
	if err != nil {
		return nil, err
	}
	if t != st._t {
		if debug.Debug {
			log.Printf("token refreshed (%s -> %s), updating session [%s]", st._t.AccessToken, t.AccessToken, st.id)
		} else {
			log.Print("token refreshed, updating session")
		}
		st._t = t
		st.s.Extra, err = json.Marshal(st._t)
		if err != nil {
			return nil, err
		}
		err = st.ss.SaveSession(st.ctx, st.id, st.s)
		if err != nil {
			return nil, err
		}
	}
	return t, err
}

type key int

var ctxTokenKey key

func NewContextToken(ctx context.Context, ts oauth2.TokenSource) context.Context {
	return context.WithValue(ctx, ctxTokenKey, ts)
}

func TokenFromContext(ctx context.Context) (ts oauth2.TokenSource, ok bool) {
	ts, ok = ctx.Value(ctxTokenKey).(oauth2.TokenSource)
	return
}

// SessionTokenSource implements oauth2.TokenSource. Enforce the interface:
var _ oauth2.TokenSource = (*SessionTokenSource)(nil)
