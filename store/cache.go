package store

import (
	"context"
	"errors"
	"log"

	"jeremy.visser.name/unlockr/access"
	"jeremy.visser.name/unlockr/debug"
	"jeremy.visser.name/unlockr/session"

	lru "github.com/hashicorp/golang-lru/v2"
)

const (
	// Number of cached users and sessions:
	defaultUserCacheLen    = 200
	defaultSessionCacheLen = 10 * defaultUserCacheLen
)

var ErrNoStore = errors.New("cache: session not found in memory store (since no session storage is defined, restarts will erase sessions)")

type UserStoreCache struct {
	// If nil, cached users may still be used,
	// but will be forgotten if cache eviction occurred.
	access.UserStore

	Length int

	uc *lru.Cache[access.Username, *access.User]
}

func (c *UserStoreCache) init() {
	if c.uc == nil {
		if c.Length <= 0 {
			c.Length = defaultUserCacheLen
		}
		uc, err := lru.New[access.Username, *access.User](c.Length)
		if err != nil {
			panic(err)
		}
		c.uc = uc
	}
}

func (c *UserStoreCache) User(ctx context.Context, u access.Username) (*access.User, error) {
	c.init()
	if user, ok := c.uc.Get(u); ok {
		return user, nil
	}
	if c.UserStore == nil {
		return nil, ErrNoStore
	}
	if user, err := c.UserStore.User(ctx, u); err != nil {
		return nil, err
	} else {
		if debug.Debug {
			log.Printf("cache miss: user[%s]", u)
		}
		c.uc.Add(u, user)
		return user, nil
	}
}

// CacheUser stores an existing user in the cache.
func (c *UserStoreCache) CacheUser(username access.Username, user *access.User) {
	c.init()
	c.uc.Add(username, user)
}

type SessionStoreCache struct {
	// If nil, cached sessions may still be used,
	// but will be forgotten if cache eviction occurred.
	session.SessionStore

	Length int

	sc *lru.Cache[session.SessionId, *session.Session]
}

func (c *SessionStoreCache) init() {
	if c.sc == nil {
		if c.Length <= 0 {
			c.Length = defaultSessionCacheLen
		}
		sc, err := lru.New[session.SessionId, *session.Session](c.Length)
		if err != nil {
			panic(err)
		}
		c.sc = sc
	}
}

func (c *SessionStoreCache) Session(ctx context.Context, id session.SessionId) (*session.Session, error) {
	c.init()
	if s, ok := c.sc.Get(id); ok {
		return s, nil
	}
	if c.SessionStore == nil {
		return nil, ErrNoStore
	}
	if s, err := c.SessionStore.Session(ctx, id); err != nil {
		return nil, err
	} else {
		c.sc.Add(id, s)
		if debug.Debug {
			log.Printf("cache miss: session[%s]", id)
		}
		return s, nil
	}
}

func (c *SessionStoreCache) SaveSession(ctx context.Context, id session.SessionId, s *session.Session) error {
	c.init()
	if c.SessionStore != nil {
		if err := c.SessionStore.SaveSession(ctx, id, s); err != nil {
			return err
		}
	}
	c.sc.Add(id, s)
	return nil
}

func (c *SessionStoreCache) CleanSessions(ctx context.Context) error {
	c.init()
	for _, k := range c.sc.Keys() {
		v, ok := c.sc.Peek(k)
		if !ok {
			continue
		}
		if v.IsExpired() {
			c.sc.Remove(k)
		}
	}
	if c.SessionStore != nil {
		return c.SessionStore.CleanSessions(ctx)
	}
	return nil
}
