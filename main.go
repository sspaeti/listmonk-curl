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
	"net/http"
	"net/mail"
	"os"
	"strings"
	"time"
)

var (
	listUUID   = os.Getenv("SUB_LIST_UUID")
	listID     = os.Getenv("SUB_LIST_ID")
	apiUser    = os.Getenv("SUB_API_USER")
	apiToken   = os.Getenv("SUB_API_TOKEN")
	listmonkURL = envOr("SUB_LISTMONK_URL", "https://list.ssp.sh")
	formURL    = envOr("SUB_FORM_URL", "https://list.ssp.sh/subscription/form")
	httpClient = &http.Client{Timeout: 10 * time.Second}
)

const usage = `
  ssp.sh newsletter — data engineering, second brain, learning in public
  -----------------------------------------------------------------------

  Short form (no flags):

    curl sub.ssp.sh/you@example.com

  Standard form:

    curl -d "email=you@example.com" https://sub.ssp.sh

  Subscriber count:   curl sub.ssp.sh/count
  Why self-hosted?    curl sub.ssp.sh/why
  Prefer a browser?   %s

`

const manifesto = `
  ╔══════════════════════════════════════════════════════════╗
  ║            Why self-host your newsletter?                ║
  ╚══════════════════════════════════════════════════════════╝

   1. Your list belongs to you.
      Not Mailchimp. Not Substack. Not Beehiiv.
      Export it at midnight on a Sunday.
      Switch tools without asking permission.

   2. You know exactly what happens to their data.
      No tracking pixels you didn't install.
      No selling segments to advertisers.
      Just you, your readers, and an SMTP server.

   3. The economics are embarrassing.
      listmonk is free and open source.
      Railway hobby plan: a few dollars a month.
      Substack's 10% cut on $1k MRR: $100/month.

   4. The gimmick is the message.
      "My newsletter is subscribable via curl"
      is a post that writes itself on HN and Bluesky.
      It filters for exactly your audience.

   5. If it breaks, you fix it.
      That's the deal. That's also the fun.

  ──────────────────────────────────────────────────────────

  Subscribe:   curl sub.ssp.sh/you@example.com
  Read more:   https://ssp.sh/brain
  The book:    https://ssp.sh/book

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

func subscribe(w http.ResponseWriter, email, name string) {
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
		"Almost there — check your inbox to confirm.\n\n"+
			"(Double opt-in. No spam, ever.)\n\n"+
			"Why self-hosted? curl sub.ssp.sh/why\n"+
			"While you wait: https://ssp.sh/brain\n")
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	// Short form: GET /you@example.com
	if r.Method == http.MethodGet && strings.Contains(path, "@") {
		subscribe(w, path, "")
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
		subscribe(w, email, name)

	default:
		plain(w, http.StatusMethodNotAllowed, "Only GET and POST.")
	}
}

func handleCount(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=300")

	if listID == "" || apiUser == "" || apiToken == "" {
		plain(w, http.StatusServiceUnavailable, "count not configured")
		return
	}

	req, _ := http.NewRequest("GET",
		listmonkURL+"/api/subscribers?list_id="+listID+"&subscription_status=confirmed&per_page=1",
		nil)
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
