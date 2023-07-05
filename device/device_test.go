package device

import (
	"reflect"
	"testing"

	"jeremy.visser.name/unlockr/access"
)

func TestForUser(t *testing.T) {
	const unused = ""
	alice, bob := &access.User{
		Username:     "alice",
		Nickname:     "Alice Montague",
		PasswordHash: unused,
		Groups:       []access.GroupName{"montagues"},
	}, &access.User{
		Username:     "bob",
		Nickname:     "Bob Capulet",
		PasswordHash: unused,
		Groups:       []access.GroupName{"capulets"},
	}
	dl := DeviceList{
		"dev-alice": &Base{
			Name: "Alice's Device",
			ACL: &access.ACL{
				Allow: access.List{
					Users:  []access.Username{"alice"},
					Groups: nil,
				},
				Deny:    access.List{Users: nil, Groups: nil},
				Default: "deny",
			},
		},
		"dev-montagues": &Base{
			Name: "Montagues Device",
			ACL: &access.ACL{
				Allow: access.List{
					Users:  nil,
					Groups: []access.GroupName{"montagues"},
				},
				Deny:    access.List{Users: nil, Groups: nil},
				Default: "deny",
			},
		},
		"dev-capulets": &Base{
			Name: "Capulets Device",
			ACL: &access.ACL{
				Deny: access.List{Groups: []access.GroupName{"montagues"}},
				// implicit default: allow
			},
		},
	}

	want := DeviceListResponse{
		"dev-alice":     DeviceResponse{Name: "Alice's Device"},
		"dev-montagues": DeviceResponse{Name: "Montagues Device"},
	}
	if got := dl.ForUser(alice); !reflect.DeepEqual(got, want) {
		t.Errorf("alice:\n\tgot: %+v\n\twant: %+v", got, want)
	}

	want = DeviceListResponse{
		"dev-capulets": DeviceResponse{Name: "Capulets Device"},
	}
	if got := dl.ForUser(bob); !reflect.DeepEqual(got, want) {
		t.Errorf("bob:\n\tgot: %+v\n\twant: %+v", got, want)
	}
}
