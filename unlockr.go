package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"jeremy.visser.name/go/unlockr/auth"
	"jeremy.visser.name/go/unlockr/auth/guest"
	"jeremy.visser.name/go/unlockr/debug"
	"jeremy.visser.name/go/unlockr/index"
)

var (
	configPath = flag.String("config", "config.json", "Path to configuration file")
	listen     = flag.String("listen", "[::1]:8080", "Listen address for HTTP server")
	debugFlag  = flag.Bool("debug", false, "enable debug logging (warning: may log secret tokens)")
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

	if *debugFlag {
		debug.Enable()
	}

	var cfg Config
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

		if cfg.Guest.Enabled() {
			authHandler = &guest.Handler{
				Passthru:     authHandler,
				Handler:      &authMux,
				SessionStore: ss,
				Config:       cfg.Guest,
			}
		}
	}

	// Register authenticated paths with auth handler:
	dl := cfg.GetDevices()
	idx := &index.Index{DL: dl}
	authMux.Handle("/api/index", idx)
	authMux.Handle("/api/device/", dl)
	authMux.HandleFunc("/api/user", auth.ServeUser)
	if gh, ok := authHandler.(*guest.Handler); ok {
		authMux.HandleFunc("/api/guest/token", gh.ServeGuestNew)
	}

	// No caching on /api/:
	authHandler = HeaderAdder{
		Handler: authHandler,
		AddHeaders: http.Header{
			"Cache-Control": []string{"no-store"},
		},
	}

	// Register pre-auth handlers:
	http.Handle("/api/", authHandler)
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

	idleDone := make(chan struct{})
	graceTime := 15 * time.Second
	go func() {
		defer close(idleDone)
		sc := make(chan os.Signal, 1)
		signal.Notify(sc, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
		switch <-sc {
		case syscall.SIGHUP:
			log.Println("SIGHUP received, re-execing...")
			log.Fatal(syscall.Exec(os.Args[0], os.Args, os.Environ()))
		default:
			ctx, cancel := context.WithTimeout(context.Background(), graceTime)
			defer cancel()
			server.Shutdown(ctx)
		}
	}()
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
	<-idleDone
}
