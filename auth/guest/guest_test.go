package guest

import (
	"encoding/json"
	"reflect"
	"testing"

	"jeremy.visser.name/unlockr/access"
)

func TestExtraType(t *testing.T) {
	e := Extra{}
	data, err := json.Marshal(e)
	if err != nil {
		t.Error(err)
	}
	dataWant := `{"Type":"guest","User":null,"Parent":"","Expiry":"0001-01-01T00:00:00Z"}`
	if string(data) != dataWant {
		t.Errorf("extra data invalid\n\tgot: %s\n\twant: %s", data, dataWant)
	}
}

func TestExtraTypePtr(t *testing.T) {
	e := Extra{}
	data, err := json.Marshal(&e)
	if err != nil {
		t.Error(err)
	}
	dataWant := `{"Type":"guest","User":null,"Parent":"","Expiry":"0001-01-01T00:00:00Z"}`
	if string(data) != dataWant {
		t.Errorf("extra data invalid\n\tgot: %s\n\twant: %s", data, dataWant)
	}
}

func TestDecodeSuccess(t *testing.T) {
	extraData := []byte(`{"type":"guest","user":{"username":"guest"},"parent":"nobody","expiry":"2024-01-01T12:00:00.000000000+11:00"}`)

	var e Extra
	if err := json.Unmarshal(extraData, &e); err != nil {
		t.Errorf("json.Unmarshal: err: want nil, got %v", err)
	}
}

func TestBadTypeFail(t *testing.T) {
	extraData := []byte(`{"type":"INVALIDTYPE","user":{"username":"guest"},"parent":"nobody","expiry":"2024-01-01T12:00:00.000000000+11:00"}`)

	var e Extra
	if err := json.Unmarshal(extraData, &e); err == nil {
		t.Errorf("json.Unmarshal: err: want !nil, got %v\n\t%#v", err, e)
	}
}

func TestMissingTypeFail(t *testing.T) {
	extraData := []byte(`{"user":{"username":"guest"},"parent":"nobody","expiry":"2024-01-01T12:00:00.000000000+11:00"}`)

	var e Extra
	if err := json.Unmarshal(extraData, &e); err == nil {
		t.Errorf("json.Unmarshal: err: want !nil, got %v\n\t%#v", err, e)
	}
}

func TestOAuthExtraFail(t *testing.T) {
	extraData := []byte(`{"access_token":"abcdef","token_type":"Bearer","refresh_token":"defghi","expiry":"2024-01-01T12:00:00.000000000+11:00"}`)

	var e Extra
	if err := json.Unmarshal(extraData, &e); err == nil {
		t.Errorf("json.Unmarshal: err: want !nil, got %v\n\t%#v", err, e)
	}
}

func TestNewUser(t *testing.T) {
	parent := access.User{
		Username: "parent",
		Nickname: "Parent",
		Groups:   access.Groups{"one", "two"},
	}

	guest, err := NewUser(&parent)
	if err != nil {
		t.Errorf("NewUser: %v", err)
	}
	want := access.User{
		Username: "guest",
		Nickname: "Guest of Parent",
		Groups:   access.Groups{"one", "two", "guest"},
	}
	if !reflect.DeepEqual(guest, &want) {
		t.Errorf("unexpected guest value:\n\tgot: %#v\n\twant: %#v", guest, want)
	}

	gatecrasher, err := NewUser(guest)
	if err == nil {
		t.Errorf("gatecrasher succeeded, wanted err\n\t%#v", gatecrasher)
	}
}
