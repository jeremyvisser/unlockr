package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"jeremy.visser.name/unlockr/access"
	"jeremy.visser.name/unlockr/debug"
	"jeremy.visser.name/unlockr/session"
)

type DBStore struct {
	// Driver and DataSourceName meanings as per https://pkg.go.dev/database/sql
	Driver         string `json:"driver"`
	DataSourceName string `json:"dsn"`

	Queries *DBQueries `json:"queries"`

	lastSessionClean time.Time
	cleanMu          sync.Mutex

	db *sql.DB
}

const sessionCleanInterval = 1 * time.Hour

type DBQueries struct {
	// SQL query to retrieve user details.
	// Must return a single row with columns:
	//   password_hash (string), nickname (string)
	// WHERE username = ?
	User string `json:"user"`

	// SQL query to retrieve group memberships.
	// Must return multiple rows with single column:
	//   group_name (string)
	// WHERE username = ?
	GroupMemberships string `json:"groupmemberships"`

	// SQL query to retrieve a session.
	// Must return a single row with columns:
	//   session_id (string), username (string), expiry (unix-timestamp), extra (json)
	// WHERE session_id = ?
	Session string `json:"session"`

	// SQL query to delete expired sessions.
	SessionClean string `json:"sessionclean"`

	// SQL query to create or update a session.
	// Must insert the following values:
	//	 session_id (string), username (string), expiry (unix-timestamp)
	SessionSave string `json:"sessionsave"`
}

func (d *DBStore) getDB() (*sql.DB, error) {
	if d.db == nil {
		db, err := sql.Open(d.Driver, d.DataSourceName)
		if err != nil {
			return nil, err
		}
		d.db = db
	}
	return d.db, nil
}

func (d *DBStore) queries() *DBQueries {
	if d.Queries == nil {
		panic("DB queries were not configured")
	}
	return d.Queries
}

func (d *DBStore) User(ctx context.Context, u access.Username) (*access.User, error) {
	user, err := d.user(ctx, u)
	if err != nil {
		return nil, err
	}
	user.Groups, err = d.groups(ctx, u)
	if err != nil {
		return nil, err
	}
	if debug.Debug() {
		log.Printf("got user[%s] from DB: %+v", u, user)
	}
	return user, nil
}

func (d *DBStore) user(ctx context.Context, u access.Username) (*access.User, error) {
	db, err := d.getDB()
	if err != nil {
		return nil, err
	}
	user := new(access.User)
	err = db.QueryRowContext(ctx, d.queries().User, u).Scan(
		&user.Username, &user.PasswordHash, &user.Nickname,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("DB query failed: %w", err)
	}
	return user, nil
}

func (d *DBStore) groups(ctx context.Context, u access.Username) (access.Groups, error) {
	db, err := d.getDB()
	if err != nil {
		return nil, err
	}
	groups := make(access.Groups, 0)
	rows, err := db.QueryContext(ctx, d.queries().GroupMemberships, u)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// If ErrNoRows, then user merely has no group memberships, which isn't
			// an error, but suggests the user might need to get out of the house more.
			return groups, nil
		}
		return nil, fmt.Errorf("DB query failed: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var g access.GroupName
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (d *DBStore) Session(ctx context.Context, id session.SessionId) (*session.Session, error) {
	db, err := d.getDB()
	if err != nil {
		return nil, err
	}

	s := new(session.Session)
	s.Extra = session.Extra{}
	var expiry int64
	err = db.QueryRowContext(ctx,
		d.queries().Session,
		id,
	).Scan(
		&s.Username, &expiry, &s.Extra)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, session.ErrNoSession
		}
		return nil, fmt.Errorf("DB query failed: %w", err)
	}
	s.Expiry = time.Unix(expiry, 0)

	go d.CleanSessions(context.Background())
	if s.IsExpired() {
		return nil, session.ErrSessionExpired
	}
	return s, nil
}

// CleanSessions removes the expired sessions from the DB.
// If ctx is nil, context.Background() is used.
// It can safely be called as a goroutine, ensuring only one deletion.
func (d *DBStore) CleanSessions(ctx context.Context) error {
	if !d.cleanMu.TryLock() {
		return nil // already cleaning, skip this occasion
	}
	defer d.cleanMu.Unlock()
	if time.Since(d.lastSessionClean) < sessionCleanInterval && !debug.Debug() {
		return nil // we cleaned too recently, skip this occasion
	}
	db, err := d.getDB()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	result, err := db.ExecContext(ctx, d.queries().SessionClean)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return err
	} else if rows > 0 {
		log.Printf("cleaned %d sessions from DB", rows)
	}
	d.lastSessionClean = time.Now()
	return nil
}

func (d *DBStore) SaveSession(ctx context.Context, id session.SessionId, s *session.Session) error {
	db, err := d.getDB()
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
		d.queries().SessionSave,
		id,
		s.Username,
		s.Expiry.Unix(),
		s.Extra,
	)
	go d.CleanSessions(context.Background())
	return err
}
