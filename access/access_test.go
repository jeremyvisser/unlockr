package access

import "testing"

func sampleUsers() (alice, bob, charlie, kevin, nilbert *User) {
	const unused = ""
	return &User{
			"alice",
			"Alice Montague",
			unused,
			[]GroupName{"montagues"},
		}, &User{
			"bob",
			"Bob Capulet",
			unused,
			[]GroupName{"capulets"},
		}, &User{
			"charlie",
			"Charlie Rottenweather",
			unused,
			[]GroupName{"plebs"},
		}, &User{
			"kevin",
			"Kevin Tomatothrower",
			unused,
			nil,
		}, &User{
			"nilbert",
			"Nilbert Nullingsworth",
			unused,
			nil,
		}
}

func TestACL(t *testing.T) {
	alice, bob, charlie, kevin, nilbert := sampleUsers()
	acl := &ACL{
		Deny: List{
			Users:  []Username{"kevin"},
			Groups: []GroupName{"plebs"},
		},
		Allow: List{
			Users:  []Username{"bob", "eve", "mallorie", "charlie"},
			Groups: []GroupName{"montagues"},
		},
		Default: "deny",
	}
	t.Logf(`
		ACL:
		  %+v
		alice:
		  %+v
		bob:
		  %+v`, acl, alice, bob)

	// alice: allowed through group list
	if got, want := acl.UserCanAccess(alice), error(nil); got != want {
		t.Errorf("UserCanAccess(alice): got %v, want %v", got, want)
	}
	// bob: allowed through user list
	if got, want := acl.UserCanAccess(bob), error(nil); got != want {
		t.Errorf("UserCanAccess(bob): got %v, want %v", got, want)
	}
	// charlie: denied due to indirect group deny taking precedence over direct user allow
	if got, want := acl.UserCanAccess(charlie), ErrAccessDenied; got != want {
		t.Errorf("UserCanAccess(charlie): got %v, want %v", got, want)
	}
	// kevin: denied due to direct user deny
	if got, want := acl.UserCanAccess(kevin), ErrAccessDenied; got != want {
		t.Errorf("UserCanAccess(kevin): got %v, want %v", got, want)
	}
	// nilbert: denied due to default
	if got, want := acl.UserCanAccess(nilbert), ErrAccessDenied; got != want {
		t.Errorf("UserCanAccess(nilbert), default=%s: got %v, want %v", acl.Default, got, want)
	}
	// nilbert: denied with invalid default
	acl.Default = "invalid"
	if got, want := acl.UserCanAccess(nilbert), ErrAccessDenied; got != want {
		t.Errorf("UserCanAccess(nilbert), default=%s: got %v, want %v", acl.Default, got, want)
	}
	// nilbert: allowed due to default
	acl.Default = "allow"
	if got, want := acl.UserCanAccess(nilbert), error(nil); got != want {
		t.Errorf("UserCanAccess(nilbert), default=%s: got %v, want %v", acl.Default, got, want)
	}
}
