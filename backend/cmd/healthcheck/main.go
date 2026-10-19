// healthcheck intentionally tests liveness; readiness remains a separate probe.
package main

import (
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	c := http.Client{Timeout: 2 * time.Second}
	r, e := c.Get("http://127.0.0.1:8080/health/live")
	if e != nil {
		os.Exit(1)
	}
	defer r.Body.Close()
	io.Copy(io.Discard, r.Body)
	if r.StatusCode != 200 {
		os.Exit(1)
	}
}
