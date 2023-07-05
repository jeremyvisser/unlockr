package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"jeremy.visser.name/unlockr/access"
)

var ErrNoSession = errors.New("no valid session token found")
var ErrSessionExpired = errors.New("session expired")

const Lifetime = 30 * 24 * time.Hour
const cookieName = "Unlockr-Session"
const cookiePath = "/api"
const tokenLength = 30

type SessionId string

type SessionStore interface {
	Session(ctx context.Context, id SessionId) (*Session, error)
	SaveSession(ctx context.Context, id SessionId, s *Session) error
	CleanSessions(ctx context.Context) error
}

type Session struct {
	Username access.Username
	Expiry   time.Time

	// Extra is metadata stored with the Session. Because it may end up in
	// persistent storage, its underlying type is []byte.
	Extra Extra
}

type Extra json.RawMessage

func (s *Session) IsExpired() bool {
	return time.Now().After(s.Expiry)
}

func (s *Session) Renew(ctx context.Context, w http.ResponseWriter, id SessionId, ss SessionStore) {
	if s.shouldRenew() {
		s.renew()
		if err := ss.SaveSession(ctx, id, s); err == nil {
			setCookie(w, id, s.Expiry)
		}
	}
}

// shouldRenew is true if we are more than half-way towards the expiration time.
func (s *Session) shouldRenew() bool {
	return !s.IsExpired() && time.Until(s.Expiry).Hours() < (Lifetime.Hours()/2)
}

func (s *Session) renew() {
	if !s.IsExpired() {
		s.Expiry = time.Now().Add(Lifetime)
	}
}

func (s *Session) Expire(ctx context.Context, w http.ResponseWriter, id SessionId, ss SessionStore) {
	s.Expiry = time.Time{} // zero-value
	_ = ss.SaveSession(ctx, id, s)
	expireCookie(w)
}

type key int

var ctxKey key

func (s *Session) NewContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKey, s)
}

func FromContext(ctx context.Context) (s *Session, ok bool) {
	s, ok = ctx.Value(ctxKey).(*Session)
	return
}

// FromRequest finds a session token, fetches the associated Session, and returns
// a Session, and r's Context which can be passed to FromContext.
func FromRequest(ctx context.Context, r *http.Request, ss SessionStore) (context.Context, SessionId, *Session, error) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil, "", nil, fmt.Errorf("%w: %w", ErrNoSession, err)
	}

	id := SessionId(c.Value)
	s, err := ss.Session(r.Context(), id)
	if err != nil {
		return nil, "", nil, err
	}

	if s.IsExpired() {
		go ss.CleanSessions(context.Background())
		return nil, "", nil, ErrSessionExpired
	}

	return s.NewContext(ctx), id, s, nil
}

// Register registers a new session and sets a cookie for the user.
//
// extra may be nil if not needed. Currently, extra is used for storing
// OAuth tokens with the session.
func Register(u access.Username, extra Extra, w http.ResponseWriter, r *http.Request, ss SessionStore) (id SessionId, err error) {
	ss.CleanSessions(r.Context()) // an opportunity to cleanup expired sessions
	id, err = newID(r.Context(), ss)
	if err != nil {
		return "", err
	}

	s := &Session{
		Username: u,
		Expiry:   time.Now().Add(Lifetime),
		Extra:    extra,
	}
	s.renew()

	if err := ss.SaveSession(r.Context(), id, s); err != nil {
		return "", err
	}

	setCookie(w, id, s.Expiry)
	return id, nil
}

func Logout(w http.ResponseWriter, r *http.Request, ss SessionStore) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return // nothing to do
	}
	id := SessionId(c.Value)
	if s, err := ss.Session(r.Context(), id); err == nil {
		s.Expire(r.Context(), w, id, ss)
	}
	ss.CleanSessions(r.Context())
}

func newID(ctx context.Context, ss SessionStore) (id SessionId, err error) {
	buf := make([]byte, tokenLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id = SessionId(base64.StdEncoding.EncodeToString(buf))
	if s, err := ss.Session(ctx, id); err == nil {
		return "", fmt.Errorf("session id collision: id[%s] already belongs to %+v", id, s)
	}
	return id, nil
}

func setCookie(w http.ResponseWriter, id SessionId, expiry time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    string(id),
		Expires:  expiry,
		SameSite: http.SameSiteStrictMode,
		Path:     cookiePath,
	})
}

func expireCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:    cookieName,
		Expires: time.Time{},
		MaxAge:  -1,
		Path:    cookiePath,
	})
}
