package index

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"jeremy.visser.name/unlockr/access"
	"jeremy.visser.name/unlockr/auth/guest"
	"jeremy.visser.name/unlockr/device"
	"jeremy.visser.name/unlockr/noop"
)

func init() {
	epoch = 42 // make tests deterministic
}

func sampleDl() device.DeviceList {
	return device.DeviceList{
		"invisible": &noop.Device{
			Base: device.Base{
				Name: "Invisible to all",
				ACL: &access.ACL{
					Default: "deny",
				},
			},
		},
		"all": &noop.Device{
			Base: device.Base{
				Name: "Visible to all",
				ACL: &access.ACL{
					Default: "allow",
				},
			},
		},
		"montagues": &noop.Device{
			Base: device.Base{
				Name: "Visible to Montagues",
				ACL: &access.ACL{
					Allow: access.List{
						Groups: []access.GroupName{"montagues"},
					},
					Default: "deny",
				},
			},
		},
		"capulets": &noop.Device{
			Base: device.Base{
				Name: "Visible to Capulets",
				ACL: &access.ACL{
					Allow: access.List{
						Groups: []access.GroupName{"capulets"},
					},
					Default: "deny",
				},
			},
		},
	}
}

// Simulate a HTTP request to the index and parse the response:
func testIndex(ctx context.Context, idx *Index, t *testing.T) *IndexResponse {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/index", nil)
	if err != nil {
		t.Error(err)
	}

	var buf bytes.Buffer
	w := httptest.ResponseRecorder{Body: &buf}
	idx.ServeHTTP(&w, r)

	if got, want := w.Result().StatusCode, http.StatusOK; got != want {
		t.Errorf("ServeHTTP: StatusCode: got %d, want %d", got, want)
	}
	if got, want := w.Result().Header.Get("Content-Type"), "application/json"; got != want {
		t.Errorf("Content-Type: got %s, want %s", got, want)
	}

	var ir IndexResponse
	if err := json.NewDecoder(w.Result().Body).Decode(&ir); err != nil && err != io.EOF {
		t.Error(err)
		if body, _ := io.ReadAll(w.Result().Body); body != nil {
			t.Errorf("Received body:\n%s", string(body))
		}
	}

	return &ir
}

// Test that the index contains a valid device list and user:
func TestIndex(t *testing.T) {
	dl := sampleDl()
	idx := Index{dl}

	u := access.User{
		Username: "nilbert",
		Nickname: "Nilbert Nullingsworth",
		Groups:   []access.GroupName{"montagues"},
	}
	uresp := u
	uresp.Groups = nil // response has no groups
	ctx := u.NewContext(context.Background())

	got := testIndex(ctx, &idx, t)
	want := &IndexResponse{
		User: uresp,
		Devices: device.DeviceListResponse{
			"all":       device.DeviceResponse{Name: "Visible to all"},
			"montagues": device.DeviceResponse{Name: "Visible to Montagues"},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("IndexResponse:\n\tgot: %#v\n\twant: %#v", got, want)
	}
}

// Tests that the index contains a valid guest lifetime if set:
func TestIndexGuest(t *testing.T) {
	g := guest.Config{
		Lifetime: guest.Lifetime(42 * time.Hour),
	}
	ctx := g.NewContext(context.Background())

	u := access.User{Username: "nilbert"}
	ctx = u.NewContext(ctx)

	idx := Index{device.DeviceList{}}
	got := testIndex(ctx, &idx, t)
	want := &IndexResponse{
		Devices: device.DeviceListResponse{},
		User:    u,
		Guest: &GuestResponse{
			Lifetime: g.Lifetime,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("IndexResponse:\n\tgot: %#v\n\twant: %#v", got, want)
		t.Errorf("GuestResponse:\n\tgot: %#v\n\twant: %#v", got.Guest, want.Guest)
	}
}
