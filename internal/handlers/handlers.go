// Package handlers contains all HTTP request handlers and routing.
package handlers

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"

	"splitwise-quic/internal/models"
	"splitwise-quic/internal/realtime"
	"splitwise-quic/internal/render"
	"splitwise-quic/internal/server"
	"splitwise-quic/internal/store"
)

type ctxKey int

const userKey ctxKey = iota

// Handlers bundles every dependency the HTTP layer needs.
type Handlers struct {
	store     *store.Store
	render    *render.Renderer
	hub       *realtime.Hub
	srv       *server.Server // for WebTransport upgrades + cert hash
	certHash  string
	uploadDir string // where receipt images are stored on disk
}

// New constructs the handler set (srv is wired in Routes).
func New(s *store.Store, r *render.Renderer, h *realtime.Hub, uploadDir string) *Handlers {
	return &Handlers{store: s, render: r, hub: h, uploadDir: uploadDir}
}

// Routes wires every endpoint and returns the top-level handler.
// It takes the *server.Server so the WebTransport endpoint can upgrade.
func (h *Handlers) Routes(srv *server.Server) http.Handler {
	h.srv = srv
	h.certHash = srv.CertHashB64()

	mux := http.NewServeMux()
	mux.Handle("GET /static/", render.StaticHandler())
	mux.HandleFunc("GET /healthz", h.healthz)

	// Auth
	mux.HandleFunc("GET /login", h.loginPage)
	mux.HandleFunc("POST /login", h.doLogin)
	mux.HandleFunc("POST /signup", h.doSignup)
	mux.HandleFunc("GET /logout", h.doLogout)

	// App (auth-required)
	mux.Handle("GET /{$}", h.auth(h.dashboard))
	mux.Handle("POST /groups", h.auth(h.createGroup))
	mux.Handle("GET /g/{id}", h.auth(h.groupPage))
	mux.Handle("POST /g/{id}/members", h.auth(h.addMember))
	mux.Handle("POST /g/{id}/expenses", h.auth(h.createExpense))
	mux.Handle("GET /g/{id}/expenses", h.auth(h.expensesPartial))
	mux.Handle("GET /g/{id}/expense-form", h.auth(h.createExpenseForm))
	mux.Handle("GET /g/{id}/expenses/{eid}/edit", h.auth(h.editExpenseForm))
	mux.Handle("POST /g/{id}/expenses/{eid}", h.auth(h.updateExpense))
	mux.Handle("POST /g/{id}/expenses/{eid}/delete", h.auth(h.deleteExpense))
	mux.Handle("GET /g/{id}/expenses/{eid}/receipt", h.auth(h.serveReceipt))
	mux.Handle("POST /g/{id}/expenses/{eid}/comments", h.auth(h.addComment))
	mux.Handle("GET /g/{id}/balances", h.auth(h.balancesPartial))
	mux.Handle("GET /g/{id}/activity", h.auth(h.activityPartial))
	mux.Handle("POST /g/{id}/settle", h.auth(h.settle))
	mux.Handle("GET /g/{id}/export.csv", h.auth(h.exportCSV))
	mux.Handle("GET /g/{id}/export.pdf", h.auth(h.exportPDF))
	mux.Handle("GET /g/{id}/events", h.auth(h.sse))

	// WebTransport (QUIC datagrams) - auth via session cookie inside handler.
	mux.HandleFunc("/g/{id}/wt", h.webTransport) // per-group live channel
	mux.HandleFunc("/wt", h.userWebTransport)    // per-user push channel

	return mux
}

// --- auth middleware + helpers -------------------------------------------

func (h *Handlers) auth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := h.currentUser(r)
		if u == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func (h *Handlers) currentUser(r *http.Request) *models.User {
	c, err := r.Cookie("sid")
	if err != nil {
		return nil
	}
	u, err := h.store.UserBySession(c.Value)
	if err != nil {
		return nil
	}
	return u
}

func userFrom(r *http.Request) *models.User {
	u, _ := r.Context().Value(userKey).(*models.User)
	return u
}

// healthz is an unauthenticated liveness/readiness probe for containers + k8s.
func (h *Handlers) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// view is the base data every page template needs.
type view struct {
	Title    string
	User     *models.User
	CertHash string
	Flash    string // one-shot error/notice banner shown at the top of the page
}

func (h *Handlers) base(w http.ResponseWriter, r *http.Request, title string) view {
	return view{Title: title, User: userFrom(r), CertHash: h.certHash, Flash: popFlash(w, r)}
}

// --- flash messages (one-shot, cookie-backed) ----------------------------

// setFlash stores a short-lived message to show after a redirect.
func setFlash(w http.ResponseWriter, msg string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "flash",
		Value:    url.QueryEscape(msg),
		Path:     "/",
		MaxAge:   30,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// popFlash reads and immediately clears the flash message (if any).
func popFlash(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie("flash")
	if err != nil || c.Value == "" {
		return ""
	}
	http.SetCookie(w, &http.Cookie{Name: "flash", Value: "", Path: "/", MaxAge: -1})
	msg, _ := url.QueryUnescape(c.Value)
	return msg
}

// flashRedirect shows msg on the next page load at dest (PRG pattern). Use this
// for user-fixable validation errors so they stay inside the app UI.
func (h *Handlers) flashRedirect(w http.ResponseWriter, r *http.Request, msg, dest string) {
	setFlash(w, msg)
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// httpError renders a themed, self-contained HTML error page instead of the
// stark default plain-text page - so failures stay on-brand and readable.
func httpError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, errorPageHTML(code, msg))
}

func errorTitle(code int) string {
	switch code {
	case http.StatusBadRequest:
		return "That didn't work"
	case http.StatusUnauthorized:
		return "Please log in"
	case http.StatusForbidden:
		return "Access denied"
	case http.StatusNotFound:
		return "Not found"
	default:
		return "Something went wrong"
	}
}

func errorPageHTML(code int, msg string) string {
	return fmt.Sprintf(`<!doctype html><html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>%d - Splitwise-QUIC</title>
<script>try{var t=localStorage.getItem('theme');if(t==='dark'||(!t&&matchMedia('(prefers-color-scheme: dark)').matches))document.documentElement.classList.add('dark');}catch(e){}</script>
<style>
:root{color-scheme:light dark}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:grid;place-items:center;font-family:Inter,system-ui,-apple-system,Segoe UI,Roboto,sans-serif;
  background:radial-gradient(1000px 600px at 80%% -10%%,rgba(16,185,129,.10),transparent 60%%),linear-gradient(180deg,#f8fafc,#eef2ff);color:#1e293b}
.dark body{background:radial-gradient(1000px 600px at 85%% -10%%,rgba(16,185,129,.18),transparent 55%%),linear-gradient(180deg,#0b1020,#0a0a1a);color:#e2e8f0}
.card{max-width:30rem;margin:1rem;padding:2rem;border-radius:1rem;background:rgba(255,255,255,.85);border:1px solid rgba(148,163,184,.3);box-shadow:0 10px 30px rgba(2,6,23,.08);backdrop-filter:blur(8px)}
.dark .card{background:rgba(15,23,42,.7);border-color:rgba(255,255,255,.1)}
.code{font-size:3rem;font-weight:800;letter-spacing:-.02em;background:linear-gradient(90deg,#10b981,#2dd4bf);-webkit-background-clip:text;background-clip:text;color:transparent;line-height:1}
h1{font-size:1.25rem;margin:.5rem 0 .25rem}
p{color:#64748b;margin:.25rem 0 1.25rem;word-break:break-word}
.dark p{color:#94a3b8}
.row{display:flex;gap:.5rem;flex-wrap:wrap}
a{display:inline-flex;align-items:center;gap:.4rem;padding:.55rem 1rem;border-radius:.7rem;font-weight:600;font-size:.9rem;text-decoration:none}
.primary{background:#10b981;color:#fff}
.ghost{background:rgba(16,185,129,.12);color:#047857}
.dark .ghost{color:#6ee7b7}
</style></head>
<body><div class="card">
<div class="code">%d</div>
<h1>%s</h1>
<p>%s</p>
<div class="row">
<a class="primary" href="/">Back to dashboard</a>
<a class="ghost" href="javascript:history.back()">Go back</a>
</div>
</div></body></html>`, code, code, errorTitle(code), html.EscapeString(msg))
}
