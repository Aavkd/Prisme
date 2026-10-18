// observation-pilot is an isolated local failure fixture, never a production service.
package main

import (
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"time"
)

func main() {
	fail := flag.Bool("fail", false, "make the local fixture fail")
	recover := flag.Bool("recover", false, "recover the local fixture")
	flag.Parse()
	if *fail {
		if os.WriteFile("/control/failed", []byte("local exercise"), 0600) != nil {
			os.Exit(1)
		}
		return
	}
	if *recover {
		if err := os.Remove("/control/failed"); err != nil && !os.IsNotExist(err) {
			os.Exit(1)
		}
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		_, err := os.Stat("/control/failed")
		status := 200
		level := "info"
		message := "local pilot healthy"
		if err == nil {
			status = 503
			level = "error"
			message = "controlled local pilot outage"
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"level": level, "message": message, "status": status, "time": time.Now().UTC()})
		w.WriteHeader(status)
	})
	server := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second}
	if server.ListenAndServe() != nil {
		os.Exit(1)
	}
}
