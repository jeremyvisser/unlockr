package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"jeremy.visser.name/unlockr/access"
	"jeremy.visser.name/unlockr/auth"
	"jeremy.visser.name/unlockr/debug"
	"jeremy.visser.name/unlockr/device"
	"jeremy.visser.name/unlockr/ewelink"
	"jeremy.visser.name/unlockr/noop"
	"jeremy.visser.name/unlockr/session"
	"jeremy.visser.name/unlockr/store"
)

var (
	//go:embed config-sample.json
	configSample string
)

type Config struct {
	Devices struct {
		Ewelink map[device.ID]*ewelink.Device `json:"ewelink"`
		Noop    map[device.ID]*noop.Device    `json:"noop"`
		//Shelly  map[string]shelly.Device `json:"shelly"`
	} `json:"devices"`
	Credentials struct {
		Ewelink *ewelink.Ewelink `json:"ewelink"`
	} `json:"credentials"`
	DataStore struct {
		File *store.FileStore `json:"file"`
		DB   *store.DBStore   `json:"db"`
	} `json:"datastore"`
	Auth *jsonAuthType `json:"auth"`
}

type jsonAuthType struct {
	http.Handler
}

type AuthType string

const (
	Password AuthType = "password"
	OAuth    AuthType = "oauth"
)

func (a *jsonAuthType) UnmarshalJSON(v []byte) error {
	var t struct {
		Type AuthType `json:"type"`
	}
	json.Unmarshal(v, &t)
	switch t.Type {
	case OAuth:
		a.Handler = new(auth.OAuthHandler)
	case Password:
		a.Handler = new(auth.PasswordAuthHandler)
	default:
		return fmt.Errorf("invalid auth type: %s", v)
	}
	return json.Unmarshal(v, a.Handler)
}

func (a *jsonAuthType) MarshalJSON() ([]byte, error) {
	switch ah := a.Handler.(type) {
	case *auth.OAuthHandler:
		return json.Marshal(struct {
			Type AuthType
			*auth.OAuthHandler
		}{
			OAuth,
			ah,
		})
	case *auth.PasswordAuthHandler:
		return json.Marshal(struct {
			Type AuthType
			*auth.PasswordAuthHandler
		}{
			Password,
			ah,
		})
	}
	return json.Marshal(nil)
}

func (c *Config) Load(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(c); err != nil {
		return err
	}
	if debug.Debug {
		if logcfg, err := json.MarshalIndent(c, "", "  "); err == nil {
			log.Printf("Loaded config:\n%s", logcfg)
		} else {
			log.Printf("Error while printing debug config: %v", err)
		}
	}
	return nil
}

func (c *Config) GetDevices() device.DeviceList {
	dl := make(device.DeviceList)
	log.Printf("Loading devices from config:")
	device.AddDevices(dl, c.Devices.Ewelink)
	device.AddDevices(dl, c.Devices.Noop)
	//device.AddDevices(dl, c.Devices.Shelly)
	if debug.Debug {
		log.Printf("  %#v", dl)
	}
	return dl
}

// GetDataStore returns the first datastore configured
func (c *Config) GetDataStores() (access.UserStore, session.SessionStore, error) {
	switch {
	case c.DataStore.File != nil:
		return &store.UserStoreCache{UserStore: c.DataStore.File},
			&store.SessionStoreCache{SessionStore: nil}, // memory-only
			nil
	case c.DataStore.DB != nil:
		return &store.UserStoreCache{UserStore: c.DataStore.DB},
			&store.SessionStoreCache{SessionStore: c.DataStore.DB},
			nil
	default:
		return nil, nil, errors.New("no datastore configured")
	}
}
