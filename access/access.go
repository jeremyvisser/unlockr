package access

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"
)

type UserStore interface {
	User(context.Context, Username) (*User, error)
}

type Username string
type GroupName string

type Users map[Username]User
type Groups []GroupName

type User struct {
	Username     `json:"username"`
	Nickname     string `json:"nickname"`
	PasswordHash string `json:"password_hash,omitempty"`

	Groups `json:"groups,omitempty"`
}

func (u *User) Authenticate(password string) error {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
}

func (g Groups) HasGroup(h GroupName) bool {
	for _, v := range g {
		if v == h {
			return true
		}
	}
	return false
}

type List struct {
	Users  []Username  `json:"users"`
	Groups []GroupName `json:"groups"`
}

// HasUser checks if the User is listed, either as a username or as a group member
func (l *List) HasUser(u *User) bool {
	for _, v := range l.Users {
		if v == u.Username {
			return true
		}
	}
	for _, v := range l.Groups {
		if u.HasGroup(v) {
			return true
		}
	}
	return false
}

type ACL struct {
	Deny    List   `json:"deny"`
	Allow   List   `json:"allow"`
	Default string `json:"default"`
}

var DefaultACL = &ACL{Default: "allow"}

var ErrAccessDenied = errors.New("access denied")

func (a *ACL) UserCanAccess(u *User) error {
	switch {
	case a.Deny.HasUser(u):
		return ErrAccessDenied
	case a.Allow.HasUser(u):
		return nil
	case a.Default == "allow" || a.Default == "":
		return nil
	}
	return ErrAccessDenied
}

type key int

var ctxKey key

// NewContext returns a copy of ctx with User attached.
// Use FromContext to retrieve the User.
func (u *User) NewContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKey, u)
}

func FromContext(ctx context.Context) (u *User, ok bool) {
	u, ok = ctx.Value(ctxKey).(*User)
	return
}
