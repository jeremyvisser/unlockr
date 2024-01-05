package guest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"jeremy.visser.name/unlockr/access"
	"jeremy.visser.name/unlockr/debug"
	"jeremy.visser.name/unlockr/session"
)

const key = "guest"

type Config struct {
	// Lifetime is how long a guest pass can exist for.
	// Zero value means guest passes are not allowed.
	Lifetime Lifetime `json:"lifetime"`
}

func (c *Config) Enabled() bool {
	if c == nil {
		return false // zero-value occurs when guest block isn't configured
	}
	return c.Lifetime > 0
}

type Lifetime time.Duration

func (l *Lifetime) UnmarshalJSON(v []byte) error {
	var s string
	err := json.Unmarshal(v, &s)
	if err != nil {
		return err
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return err
	}

	*l = Lifetime(d)
	return nil
}

type Extra struct {
	Type extraType // "guest"

	User   *access.User    // User is the guest
	Parent access.Username // Parent created the guest

	// Expiry is never renewed:
	Expiry time.Time
}

func (e *Extra) IsValid() bool {
	return e.Expiry.After(time.Now())
}

func (e Extra) MarshalJSON() ([]byte, error) {
	e.Type = key
	return json.Marshal(jsonExtra(e))
}

func (e *Extra) UnmarshalJSON(data []byte) error {
	var je jsonExtra
	if err := json.Unmarshal(data, &je); err != nil {
		return err
	}
	if je.Type != key {
		return fmt.Errorf("%w: got '%s', want '%s'", ErrType, e.Type, key)
	}
	*e = Extra(je)
	return nil
}

var _ json.Marshaler = (*Extra)(nil)
var _ json.Unmarshaler = (*Extra)(nil)

type extraType string
type jsonExtra Extra // un-implement json.Marshaler/Unmarshaler

var ErrType = errors.New("invalid session extra type")

type Handler struct {
	Passthru     http.Handler
	Handler      http.Handler
	SessionStore session.SessionStore
	Config       *Config
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	{
		// If any failures occur, pass to next handler:
		ctx, _, s, err := session.FromRequest(r.Context(), r, h.SessionStore)
		if err != nil {
			if debug.Debug {
				log.Printf("GuestHandler: session.FromRequest: %v", err)
			}
			goto passthru
		}

		// Try decoding guest session, but fail gracefully:
		var extra Extra
		if err := json.Unmarshal(s.Extra, &extra); err != nil {
			if debug.Debug {
				log.Printf("GuestHandler: decoding session extra failed (not a guest session?): %v", err)
			}
			goto passthru
		}

		// This appears to be a guest session.
		if debug.Debug {
			log.Printf("GuestHandler: guest session\n\t%#v\n\t%#v", s, extra)
		}

		if !extra.IsValid() {
			http.Error(w, "Guest session expired", http.StatusForbidden)
			if debug.Debug {
				log.Printf("Guest session expired:\n\tCurrent time: %s\n\tExpiry time: %s", time.Now(), extra.Expiry)
			}
			return
		}

		// Put guest into context and invoke child handler:
		ctx = extra.User.NewContext(ctx)
		h.Handler.ServeHTTP(w, r.WithContext(ctx))
		return
	}

passthru:
	if debug.Debug {
		log.Printf("GuestHandler: passthrough to next handler: %v", h.Passthru)
	}
	h.Passthru.ServeHTTP(w, r)
}

type Info struct {
	Token  session.SessionId `json:"token"`
	Expiry time.Time         `json:"expiry"`
}

func (h *Handler) ServeGuestNew(w http.ResponseWriter, r *http.Request) {
	if !h.Config.Enabled() {
		http.Error(w, "guest access not enabled", http.StatusForbidden)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "must be POST", http.StatusMethodNotAllowed)
		return
	}

	u, ok := access.FromContext(r.Context())
	if !ok {
		http.Error(w, "invalid user", http.StatusForbidden)
		return
	}

	id, s, err := h.NewSession(r.Context(), u)
	if err != nil {
		log.Printf("Error creating guest pass: %v", err)
		http.Error(w, "error creating guest pass", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Info{
		Token:  id,
		Expiry: s.Expiry,
	})
}

func (h *Handler) NewSession(ctx context.Context, parent *access.User) (session.SessionId, *session.Session, error) {
	g, err := NewUser(parent)
	if err != nil {
		return "", nil, err
	}

	expiry := time.Now().Add(time.Duration(h.Config.Lifetime))
	extra, err := json.Marshal(Extra{
		User:   g,
		Parent: parent.Username,
		Expiry: expiry,
	})
	if err != nil {
		return "", nil, err
	}

	s := &session.Session{
		Username: g.Username,
		Expiry:   expiry,
		Extra:    extra,
	}
	if debug.Debug {
		log.Printf("GuestHandler.NewSession:\n\t%#v\n\t%s", s, s.Extra)
	}
	id, err := session.New(ctx, s, h.SessionStore)
	if err != nil {
		return "", nil, err
	}
	return id, s, nil
}

// NewUser returns a user with equivalent group memberships to parent.
// This ephemeral user is intended to be embedded in a session record, and
// destroyed once expired.
func NewUser(parent *access.User) (guest *access.User, err error) {
	if parent.Username == key {
		return nil, ErrNoGatecrashers
	}
	guest = &access.User{
		Username: key,
		Nickname: fmt.Sprintf("Guest of %s", parent.Nickname),
		Groups:   append(parent.Groups, key),
	}
	return guest, nil
}

var ErrNoGatecrashers = errors.New("guests cannot invite guests")
