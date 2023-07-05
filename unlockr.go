package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"jeremy.visser.name/unlockr/auth"
	"jeremy.visser.name/unlockr/ewelink"
	"jeremy.visser.name/unlockr/index"
)

var (
	configPath = flag.String("config", "config.json", "Path to configuration file")
	listen     = flag.String("listen", "[::1]:8080", "Listen address for HTTP server")
)

type LogHandler struct {
	http.Handler
}

func (l *LogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
	l.Handler.ServeHTTP(w, r)
}

func main() {
	flag.Parse()

	var cfg Config
	cfg.Credentials.Ewelink = &ewelink.DefaultEwelink
	if err := cfg.Load(*configPath); err != nil {
		log.Fatal(err,
			"\nPlease create config.json and set -config=/path/to/config.json",
			"\nSample config:\n",
			configSample)
	}
	os.Chdir(filepath.Dir(*configPath)) // for relative paths within config

	// Choose between OAuth or Password auth:
	if cfg.Auth == nil {
		log.Fatal("Please specify an auth method in config.json.\n",
			"Sample config:\n", configSample)
	}
	var authHandler http.Handler = cfg.Auth.Handler
	var authMux http.ServeMux
	if us, ss, err := cfg.GetDataStores(); err != nil {
		log.Fatal(err, "\nSample config:\n", configSample)
	} else {
		switch ah := authHandler.(type) {
		case *auth.PasswordAuthHandler:
			ah.UserStore = us
			ah.SessionStore = ss
			ah.Handler = &authMux
		case *auth.OAuthHandler:
			// UserStore is unused here
			ah.SessionStore = ss
			ah.Handler = &authMux
		}
	}

	// Register authenticated paths with auth handler:
	dl := cfg.GetDevices()
	idx := &index.Index{DL: dl}
	authMux.Handle("/api/index", idx)
	authMux.Handle("/api/device/", dl)
	authMux.HandleFunc("/api/user", auth.ServeUser)

	// No caching on /api/:
	apiHandler := HeaderAdder{
		Handler: authHandler,
		AddHeaders: http.Header{
			"Cache-Control": []string{"no-store"},
		},
	}

	// Register pre-auth handlers:
	http.Handle("/api/", apiHandler)
	http.Handle("/", staticHandler)

	// Listen using DefaultServeMux:
	log.Println("Listening on", *listen)
	server := &http.Server{
		Addr:         *listen,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		Handler:      &LogHandler{http.DefaultServeMux},
	}
	http.DefaultClient.Timeout = 15 * time.Second
	log.Fatal(server.ListenAndServe())
}
