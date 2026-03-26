package handlers

import "net/http"

// RenderHeader is a no-op for backward compatibility.
// The header is rendered by the base template in templates.go.
func RenderHeader(w http.ResponseWriter, isAdmin bool) {}

// RenderHeaderHTML returns empty string — header is in the base template.
func RenderHeaderHTML(isAdmin bool) string { return "" }
