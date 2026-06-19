// Package render owns HTML templates and static assets (embedded in the binary).
package render

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Renderer parses templates once and renders named pages/partials.
type Renderer struct {
	tmpl *template.Template
}

// New parses every template with shared helper funcs.
func New() (*Renderer, error) {
	funcs := template.FuncMap{
		"money":    Money,    // minor units -> "12.34"
		"absMoney": AbsMoney, // |minor units| -> "12.34"
	}
	t, err := template.New("").Funcs(funcs).ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Renderer{tmpl: t}, nil
}

// Page renders a full page (template that itself invokes "layout").
func (r *Renderer) Page(w io.Writer, name string, data any) error {
	return r.tmpl.ExecuteTemplate(w, name, data)
}

// Partial renders a named fragment (for HTMX swaps).
func (r *Renderer) Partial(w io.Writer, name string, data any) error {
	return r.tmpl.ExecuteTemplate(w, name, data)
}

// StaticHandler serves embedded static assets under /static/.
func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err) // embedded path is compile-time guaranteed
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}

// Money formats minor units (cents) as a human-readable decimal string.
func Money(minor int64) string {
	neg := minor < 0
	if neg {
		minor = -minor
	}
	s := fmt.Sprintf("%d.%02d", minor/100, minor%100)
	if neg {
		return "-" + s
	}
	return s
}

// AbsMoney formats the absolute value of minor units (no leading minus sign).
func AbsMoney(minor int64) string {
	if minor < 0 {
		minor = -minor
	}
	return Money(minor)
}
