package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"

	"jeremy.visser.name/unlockr/access"
)

type FileStore struct {
	Path string `json:"path"`
	data *FileStoreData
}

type FileStoreData struct {
	Users access.Users `json:"users"`
}

// load reads the entire data store and returns a FileStoreData representation
func (f *FileStore) load() error {
	if f.data != nil {
		return nil
	}
	f.data = &FileStoreData{
		Users: make(access.Users),
	}
	file, err := os.Open(f.Path)
	if err != nil {
		return err
	}
	defer file.Close()
	err = json.NewDecoder(file).Decode(f.data)
	log.Printf("loaded %d users and %d groups from: %s",
		len(f.data.Users), len(f.data.Users), file.Name())
	return err
}

func (f *FileStore) User(ctx context.Context, u access.Username) (*access.User, error) {
	if err := f.load(); err != nil {
		return nil, err
	}
	// Copy user (can't create pointer to map index):
	user := new(access.User)
	var ok bool
	*user, ok = f.data.Users[u]
	if !ok {
		return nil, sql.ErrNoRows
	}
	// Populate username from map key:
	user.Username = u
	return user, nil
}
