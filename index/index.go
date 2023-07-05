package index

import (
	"encoding/json"
	"net/http"
	"time"

	"jeremy.visser.name/unlockr/access"
	"jeremy.visser.name/unlockr/device"
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
	Epoch   int64                     `json:"epoch"`
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
		Epoch:   epoch,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&ir); err != nil {
		http.Error(w, "Error getting index", http.StatusInternalServerError)
	}
}
