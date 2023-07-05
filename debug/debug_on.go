//go:build debug

// To build debug version:
//   go build -tags debug .

package debug

import (
	"log"
	"net/http"
	"net/http/httputil"
)

const Debug = true

func init() {
	if Debug {
		log.Println("Debugging enabled")
	}

	http.DefaultTransport = &TransportLogger{http.DefaultTransport}
}

type TransportLogger struct {
	http.RoundTripper
}

func (t *TransportLogger) RoundTrip(r *http.Request) (*http.Response, error) {
	buf, _ := httputil.DumpRequestOut(r, true)
	log.Printf("> %s", buf)

	resp, err := t.RoundTripper.RoundTrip(r)

	buf, _ = httputil.DumpResponse(resp, true)
	log.Printf("< %s", buf)

	return resp, err
}
