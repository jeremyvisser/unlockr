package ewelink

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"jeremy.visser.name/unlockr/debug"
	"jeremy.visser.name/unlockr/device"
)

type Ewelink struct {
	// Email is the email address registered beforehand. Required.
	Email string `json:"email"`
	// Password is required.
	Password string `json:"password"`

	// Region is used for contacting the correct server. Required.
	Region string `json:"region"`

	// CountryCode is what the account was registered as. Required.
	// e.g. "+1" or "+61"
	CountryCode string `json:"countryCode"`

	// URL is optional, and is generated based on Region if unset.
	URL string `json:"-"`

	// AppID and AppSecret must be obtained from dev.ewelink.cc. Required.
	AppID     string `json:"appid"`
	AppSecret string `json:"appsecret"`

	// Token is the cached bearer credential.
	tokens struct {
		Token        string `json:"at,omitempty"`
		RefreshToken string `json:"rt"`
		lastRefresh  time.Time
	}
}

type envelope struct {
	Error   int    `json:"error"`
	Message string `json:"msg"`
	Data    any    `json:"data"`
}

var DefaultEwelink Ewelink

type Device struct {
	// DeviceID is the eWeLink-specific identifier for this device. Required.
	DeviceID string `json:"deviceid"`

	Outlet *Index `json:"outlet"`

	device.Base
	*Ewelink `json:"-"`
}

type Index uint64

func (e *Ewelink) region() string {
	if e.Region != "" {
		return e.Region
	}
	return "us"
}

func (e *Ewelink) url() string {
	if e.URL != "" {
		return strings.TrimRight(e.URL, "/")
	}
	tld := "cc"
	if e.region() == "cn" {
		tld = "cn"
	}
	return fmt.Sprintf("https://%s-apia.coolkit.%s", e.region(), tld)
}

// Token retrieves the current token, or calls login if missing.
func (e *Ewelink) Token(ctx context.Context) (token string, err error) {
	if e.tokens.Token == "" || e.maybeRefresh(ctx) != nil {
		err := e.Login(ctx)
		if err != nil {
			return "", err
		}
	}
	return e.tokens.Token, nil
}

// maybeRefresh gets a new token if the current one is too old using RefreshToken.
func (e *Ewelink) maybeRefresh(ctx context.Context) error {
	if time.Since(e.tokens.lastRefresh) < 20*time.Hour {
		return nil
	}
	if e.tokens.RefreshToken == "" {
		return fmt.Errorf("RefreshToken is not set")
	}
	e.tokens.Token = ""
	req, err := e.NewRequest(ctx, "/v2/user/refresh", &e.tokens)
	if err != nil {
		return err
	}
	err = e.ApiCall(req, e.tokens)
	if err != nil {
		return err
	}
	e.tokens.lastRefresh = time.Now()
	return nil
}

// Login gets a new token from the API and updates tokens.
func (e *Ewelink) Login(ctx context.Context) error {
	if e.Email == "" || e.Password == "" || e.Region == "" || e.CountryCode == "" {
		return fmt.Errorf("Ewelink not fully configured: %#v", e)
	}
	req, err := e.NewPreAuthRequest(ctx, "/v2/user/login", e)
	if err != nil {
		return err
	}
	err = e.ApiCall(req, &e.tokens)
	if err != nil {
		return err
	}
	e.tokens.lastRefresh = time.Now()
	return nil
}

func (e *Ewelink) NewRequest(ctx context.Context, url string, payload any) (*http.Request, error) {
	return e.newRequestInternal(ctx, url, payload, false)
}

func (e *Ewelink) NewPreAuthRequest(ctx context.Context, url string, payload any) (*http.Request, error) {
	return e.newRequestInternal(ctx, url, payload, true)
}

// newRequestInternal is used for constructing a HTTP request including auth headers and JSON payload.
func (e *Ewelink) newRequestInternal(ctx context.Context, url string, payload any, preauth bool) (*http.Request, error) {
	var buf *bytes.Buffer = nil
	if e.AppID == "" || e.AppSecret == "" {
		return nil, errors.New("AppID and AppSecret must be set")
	}
	method := "GET"
	if payload != nil {
		buf = new(bytes.Buffer)
		err := json.NewEncoder(buf).Encode(payload)
		if err != nil {
			return nil, err
		}
		method = "POST"
	}
	req, err := http.NewRequestWithContext(ctx,
		method,
		e.url()+url,
		buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-ck-appid", e.AppID)
	if preauth {
		sig := CalcSignature(buf.Bytes(), []byte(e.AppSecret))
		req.Header.Set("Authorization", "Sign "+sig)
	} else {
		token, err := e.Token(ctx)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

// ApiCall sends a prepared http.Request, returns an error if the relevant code
// was set in the envelope, and unmarshals the data into target (if not nil).
func (e *Ewelink) ApiCall(req *http.Request, target any) (err error) {
	if debug.Debug {
		log.Printf("> %s %s %+v", req.Method, req.URL, req.Header)
		if body, err := req.GetBody(); err == nil && body != nil {
			io.Copy(log.Writer(), body)
			body.Close()
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if resp.StatusCode == http.StatusUnauthorized {
			e.tokens.Token = ""
		}
		return err
	}
	defer resp.Body.Close()
	if debug.Debug {
		// Wrapping in a lambda is needed to access
		// the post-return err state.
		defer func() {
			log.Printf("< %s %+v", resp.Status, resp.Header)
			if err != nil {
				log.Printf("E: %+v", err)
			}
		}()
	}
	env := &envelope{
		Data: target,
	}
	err = json.NewDecoder(resp.Body).Decode(env)
	if err != nil {
		return err
	}
	if env.Error > 0 {
		// Bizarrely, on invalid token, the API returns 200 OK with
		// a JSON {"error":401} response, rather than a proper 401:
		if env.Error == 401 {
			e.tokens.Token = "" // apparently we have an invalid token
		}
		return fmt.Errorf("API error (%d): %s", env.Error, env.Message)
	}
	return nil
}

func (d *Device) ewelink() *Ewelink {
	if d.Ewelink == nil {
		return &DefaultEwelink
	}
	return d.Ewelink
}

type PowerRequest struct {
	Type   int                `json:"type"`
	ID     string             `json:"id"`
	Params PowerRequestParams `json:"params"`
}
type Switches struct {
	Switch string `json:"switch"`
	Outlet Index  `json:"outlet"`
}
type PowerRequestParams struct {
	Switch   string     `json:"switch,omitempty"`
	Switches []Switches `json:"switches,omitempty"`
}

func (d *Device) Power(ctx context.Context, on bool) error {
	var onstr string
	switch on {
	case true:
		onstr = "on"
	default:
		onstr = "off"
	}
	var params PowerRequestParams
	if d.Outlet != nil {
		params.Switches = []Switches{
			{Switch: onstr, Outlet: *d.Outlet},
		}
	} else {
		params.Switch = onstr
	}
	payload := &PowerRequest{
		Type:   1, // 1 = Device, 2 = Group
		ID:     d.DeviceID,
		Params: params,
	}
	req, err := d.ewelink().NewRequest(ctx, "/v2/device/thing/status", payload)
	if err != nil {
		return err
	}
	err = d.ewelink().ApiCall(req, nil)
	return err
}

func CalcSignature(message, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(message)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
