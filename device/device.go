package device

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"jeremy.visser.name/unlockr/access"
)

type Device interface {
	GetName() Name
	GetACL() *access.ACL
}

type PowerControl interface {
	Power(ctx context.Context, on bool) error
}

type ID string
type Name string

type Base struct {
	Name `json:"name"`
	ACL  *access.ACL `json:"acl,omitempty"`
}

func (b *Base) GetName() Name {
	return b.Name
}

func (b *Base) GetACL() *access.ACL {
	if b.ACL == nil {
		return access.DefaultACL
	}
	return b.ACL
}

type DeviceList map[ID]Device

type DeviceListResponse map[ID]DeviceResponse

// DeviceResponse is a subset of Base that is relevant to the user.
type DeviceResponse struct {
	Name `json:"name"`
}

// AddDevices appends a map of T type devices to a generic DeviceList.
// T must implement the Device interface.
func AddDevices[T Device](dst DeviceList, src map[ID]T) {
	for k, v := range src {
		if _, ok := dst[k]; !ok {
			dst[k] = v
		}
	}
}

func (d DeviceList) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/device/"
	path, ok := strings.CutPrefix(r.URL.Path, prefix)
	if !ok {
		panic("prefix /api/device/ not found: misconfigured handler")
	}
	id, args, _ := strings.Cut(path, "/")
	if id != "" {
		d.ServeDevice(r.Context(), w, r, ID(id), args)
		return
	}
	d.ServeList(r.Context(), w, r)
}

// ForUser returns the subset of devices that u is allowed to access
func (d DeviceList) ForUser(u *access.User) (ud DeviceListResponse) {
	ud = make(DeviceListResponse)
	for id := range d {
		if err := d[id].GetACL().UserCanAccess(u); err != nil {
			continue
		}
		ud[id] = DeviceResponse{Name: d[id].GetName()}
	}
	return ud
}

func (d DeviceList) ServeList(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	u, ok := access.FromContext(ctx)
	if !ok {
		http.NotFound(w, r)
		return
	}
	l := d.ForUser(u)
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(l)
	if err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
}

func (d DeviceList) ServeDevice(ctx context.Context, w http.ResponseWriter, r *http.Request, id ID, args string) {
	dev, ok := d[id]
	if !ok {
		http.NotFound(w, r)
		return
	}
	u, ok := access.FromContext(ctx)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := dev.GetACL().UserCanAccess(u); err != nil {
		log.Printf("Device[%s]: user[%s] not allowed by ACL", dev.GetName(), u.Username)
		http.Error(w, "Not allowed to access device", http.StatusForbidden)
		return
	}
	action, sub, _ := strings.Cut(args, "/")
	switch action {
	case "power":
		if _, ok := dev.(PowerControl); !ok {
			log.Printf("Device[%s] doesn't have PowerControl", id)
			http.NotFound(w, r)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "Must use POST", http.StatusMethodNotAllowed)
			return
		}
		var on bool
		switch sub {
		case "on", "off":
			on = sub == "on"
		default:
			http.Error(w, "must be 'power/on' or 'power/off'", http.StatusBadRequest)
			return
		}
		if err := dev.(PowerControl).Power(r.Context(), on); err != nil {
			log.Print("power: error from device: ", err)
			http.Error(w, "Error controlling device", http.StatusInternalServerError)
			return
		}
		io.WriteString(w, "ok")
		return
	}
	http.NotFound(w, r)
}
