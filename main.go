// sub.ssp.sh — curl-friendly newsletter signup shim for listmonk
//
// Short form (email in path — no flags needed):
//
//	curl sub.ssp.sh/you@example.com
//
// Standard form:
//
//	curl -d "email=you@example.com" https://sub.ssp.sh
//
// Extra endpoints:
//
//	curl sub.ssp.sh/count    subscriber count
//	curl sub.ssp.sh/why      self-hosting manifesto
//
// Browser GET → redirects to the listmonk subscription form.
//
// Env vars:
//
//	SUB_LIST_UUID         public list UUID (listmonk admin → Lists)
//	SUB_LIST_ID           integer list ID (same place, visible in the URL)
//	SUB_API_USER          listmonk API username (for /count)
//	SUB_API_TOKEN         listmonk API token (for /count)
//	SUB_LISTMONK_URL      listmonk base URL (default https://list.ssp.sh)
//	SUB_FORM_URL          public form URL (default https://list.ssp.sh/subscription/form)
//	PORT                  listen port, set automatically by Railway (default 8080)
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	listUUID    = os.Getenv("SUB_LIST_UUID")
	apiUser     = os.Getenv("SUB_API_USER")
	apiToken    = os.Getenv("SUB_API_TOKEN")
	listmonkURL = envOr("SUB_LISTMONK_URL", "https://list.ssp.sh")
	formURL     = envOr("SUB_FORM_URL", "https://list.ssp.sh/subscription/form")
	httpClient  = &http.Client{Timeout: 10 * time.Second}
)

// Simple per-IP rate limiter: max 5 subscribe attempts per hour.
var limiter = struct {
	sync.Mutex
	hits map[string][]time.Time
}{hits: make(map[string][]time.Time)}

func rateLimit(r *http.Request) bool {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = strings.SplitN(fwd, ",", 2)[0]
	}
	now := time.Now()
	window := now.Add(-1 * time.Hour)

	limiter.Lock()
	defer limiter.Unlock()

	// Drop hits older than 1 hour.
	recent := limiter.hits[ip][:0]
	for _, t := range limiter.hits[ip] {
		if t.After(window) {
			recent = append(recent, t)
		}
	}
	limiter.hits[ip] = append(recent, now)
	return len(limiter.hits[ip]) > 5
}

const usage = `
  ssp.sh newsletter — data engineering, second brain, learning in public
  -----------------------------------------------------------------------

  Short form (no flags):

    curl https://sub.ssp.sh/you@example.com

  Standard form:

    curl -d "email=you@example.com" https://sub.ssp.sh

  Subscriber count:   curl https://sub.ssp.sh/count
  Why self-hosted?    curl https://sub.ssp.sh/why
  Prefer a browser?   %s

`

const manifesto = `
  ╔══════════════════════════════════════════════════════════╗
  ║            ssp.sh newsletter — why subscribe?            ║
  ╚══════════════════════════════════════════════════════════╝

  Simon Späti. Data engineer, author, freelance technical writer.
  20+ years in data. Learning in public since 2015.

   1. No schedule, I write when I have something worth saying.

   2. Notes and posts from 20+ years of building data pipelines,
      writing about open-source, and working with data teams.

   3. 1000+ public notes on data engineering, Obsidian, Neovim,
      writing, and more — you get a digest of what's new.

   4. «Patterns of Data Engineering» — new chapters land here 
      first.

   5. Books I finished. Things I'm thinking about. Philosophy, 
      productivity, deep life. Not just data.

   6. No tracking. No sponsors. Self-hosted on listmonk. Your 
      email stays yours.

  ──────────────────────────────────────────────────────────

  Subscribe:   'curl https://sub.ssp.sh/you@example.com'
                                     -> add your email above
  More:          newsletter.ssp.sh
  Website:       ssp.sh

  ──────────────────────────────────────────────────────────

`

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func isCurl(r *http.Request) bool {
	ua := strings.ToLower(r.UserAgent())
	return strings.Contains(ua, "curl") || strings.Contains(ua, "wget") || strings.Contains(ua, "httpie")
}

func plain(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintln(w, msg)
}

func subscribe(w http.ResponseWriter, r *http.Request, email, name string) {
	if rateLimit(r) {
		plain(w, http.StatusTooManyRequests, "Slow down. Max 5 attempts per hour per IP.")
		return
	}
	if _, err := mail.ParseAddress(email); err != nil {
		plain(w, http.StatusBadRequest,
			"That doesn't look like an email address.\n\nTry:\n  curl sub.ssp.sh/you@example.com")
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"email":      email,
		"name":       name,
		"list_uuids": []string{listUUID},
	})
	resp, err := httpClient.Post(listmonkURL+"/api/public/subscription", "application/json", bytes.NewReader(payload))
	if err != nil {
		plain(w, http.StatusBadGateway, "listmonk didn't answer. Try again in a minute.")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("listmonk error %d: %s", resp.StatusCode, body)
		plain(w, http.StatusBadGateway, "Subscription failed. If this persists, email me: simon@ssp.sh")
		return
	}

	plain(w, http.StatusOK,
		"Almost there -> CHECK YOUR INBOX TO CONFIRM.\n\n"+
		"------\n\n"+
			"Why subscribe? curl https://sub.ssp.sh/why\n")
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	// Short form: GET /you@example.com
	if r.Method == http.MethodGet && strings.Contains(path, "@") {
		subscribe(w, r, path, "")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if isCurl(r) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintf(w, usage, formURL)
			return
		}
		http.Redirect(w, r, formURL, http.StatusFound)

	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			plain(w, http.StatusBadRequest, "Could not parse form data.")
			return
		}
		email := strings.TrimSpace(r.PostFormValue("email"))
		name := strings.TrimSpace(r.PostFormValue("name"))
		subscribe(w, r, email, name)

	default:
		plain(w, http.StatusMethodNotAllowed, "Only GET and POST.")
	}
}

func handleCount(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=300")

	if apiUser == "" || apiToken == "" {
		plain(w, http.StatusServiceUnavailable, "count not configured")
		return
	}

	// No list_id filter → matches the "All subscribers" total in listmonk admin,
	// deduped across all lists (newsletter + book).
	req, _ := http.NewRequest("GET", listmonkURL+"/api/subscribers?per_page=1", nil)
	req.SetBasicAuth(apiUser, apiToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		plain(w, http.StatusBadGateway, "error")
		return
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		plain(w, http.StatusInternalServerError, "error")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "%d\n", result.Data.Total)
}

func handleWhy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, manifesto)
}

func main() {
	if listUUID == "" {
		log.Fatal("SUB_LIST_UUID is required (listmonk admin → Lists → UUID)")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/why", handleWhy)
	mux.HandleFunc("/count", handleCount)
	mux.HandleFunc("/", handleRoot)

	port := envOr("PORT", "8080")
	addr := ":" + port
	log.Printf("sub-subscribe listening on %s → %s", addr, listmonkURL)
	log.Fatal(http.ListenAndServe(addr, mux))
}
