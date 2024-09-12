package index

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"jeremy.visser.name/go/unlockr/access"
	"jeremy.visser.name/go/unlockr/auth/guest"
	"jeremy.visser.name/go/unlockr/device"
)

var (
	epoch = time.Now().Unix()
)

type Index struct {
	DL device.DeviceList
}

type IndexResponse struct {
	User    access.User               `json:"user"`
	Devices device.DeviceListResponse `json:"devices"`
	Guest   *GuestResponse            `json:"guest,omitempty"`
	Epoch   int64                     `json:"epoch"`
}

type GuestResponse struct {
	Lifetime guest.Lifetime `json:"lifetime"`
}

func (idx *Index) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u, ok := access.FromContext(r.Context())
	if !ok {
		http.NotFound(w, r)
		return
	}

	ir := IndexResponse{
		User: access.User{
			// Explicitly copy only the fields we want to return:
			Username: u.Username,
			Nickname: u.Nickname,
			// We don't return the Groups or PasswordHash fields.
		},
		Devices: idx.DL.ForUser(u),
		Guest:   idx.GuestConfig(r.Context()),
		Epoch:   epoch,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&ir); err != nil {
		http.Error(w, "Error getting index", http.StatusInternalServerError)
	}
}

// GuestConfig returns the configuration the user is allowed to see.
// We don't pass the config entirety, as fields added in future may be secret.
func (idx *Index) GuestConfig(ctx context.Context) *GuestResponse {
	cfg, ok := guest.ConfigFromContext(ctx)
	if ok {
		return &GuestResponse{
			Lifetime: cfg.Lifetime,
		}
	}
	return nil
}
