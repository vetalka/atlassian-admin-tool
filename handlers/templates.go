package handlers

import (
	"fmt"
	"html"
	"html/template"
	"net/http"
)

// PageData holds all data needed to render a page with the base layout
type PageData struct {
	Title     string
	IsAdmin   bool
	ExtraHead template.HTML // additional <head> content (scripts, etc.)
	Content   template.HTML // the page body content
}

// Base layout template string - single source of truth for all pages
const baseLayoutTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} — Atlassian Admin Tool</title>
    <link rel="icon" href="/static/favicon.png" type="image/png">
    <link rel="stylesheet" href="/static/styles.css?v=20260224d">
    <script>
        (function(){var t=localStorage.getItem('theme');if(t==='dark')document.documentElement.setAttribute('data-theme','dark');})();
    </script>
    {{.ExtraHead}}
</head>
<body>
    <div class="ads-header">
        <div class="ads-header-left">
            <img src="/static/methoda.png" class="ads-header-logo" onclick="window.location.href='/'" alt="Logo" />
            <div class="ads-header-divider"></div>
            <span class="ads-header-title">Atlassian Admin Tool</span>
        </div>
        <div class="ads-header-right">
            <div class="ads-header-dropdown" id="settingsDropdown">
                <button class="ads-header-icon-btn" onclick="document.getElementById('settingsDropdown').classList.toggle('open')" title="Settings">
                    <span style="font-size:24px; line-height:1;">&#x2699;</span>
                </button>
                <div class="ads-dropdown-menu">
                    <a href="/my-account" class="ads-dropdown-item">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"></path><circle cx="12" cy="7" r="4"></circle></svg>
                        My Account
                    </a>
                    {{if .IsAdmin}}
                    <div class="ads-dropdown-divider"></div>
                    <a href="/settings/users" class="ads-dropdown-item">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"></path><circle cx="9" cy="7" r="4"></circle></svg>
                        User Management
                    </a>
                    <a href="/settings/updatelicense" class="ads-dropdown-item">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.78 7.78 5.5 5.5 0 0 1 7.78-7.78zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"></path></svg>
                        License
                    </a>
                    <a href="/cron/policies" class="ads-dropdown-item">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>
                        Backup Policies
                    </a>
                    <div class="ads-dropdown-divider"></div>
                    {{end}}
                    <button class="ads-dropdown-item" onclick="toggleTheme(); event.stopPropagation();">
                        <svg id="theme-icon-sun" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="display:none;"><circle cx="12" cy="12" r="5"></circle><line x1="12" y1="1" x2="12" y2="3"></line><line x1="12" y1="21" x2="12" y2="23"></line><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line><line x1="1" y1="12" x2="3" y2="12"></line><line x1="21" y1="12" x2="23" y2="12"></line><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line></svg>
                        <svg id="theme-icon-moon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path></svg>
                        <span id="theme-label">Dark mode</span>
                    </button>
                    <div class="ads-dropdown-divider"></div>
                    <a href="/logout" class="ads-dropdown-item ads-dropdown-item-danger">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"></path><polyline points="16 17 21 12 16 7"></polyline><line x1="21" y1="12" x2="9" y2="12"></line></svg>
                        Log out
                    </a>
                </div>
            </div>
        </div>
    </div>
    {{.Content}}
    <script>
        function toggleTheme() {
            var html = document.documentElement;
            var isDark = html.getAttribute('data-theme') === 'dark';
            if (isDark) {
                html.removeAttribute('data-theme');
                localStorage.setItem('theme', 'light');
            } else {
                html.setAttribute('data-theme', 'dark');
                localStorage.setItem('theme', 'dark');
            }
            updateThemeIcon();
        }
        function updateThemeIcon() {
            var isDark = document.documentElement.getAttribute('data-theme') === 'dark';
            var sun = document.getElementById('theme-icon-sun');
            var moon = document.getElementById('theme-icon-moon');
            var label = document.getElementById('theme-label');
            if (sun && moon) {
                sun.style.display = isDark ? 'block' : 'none';
                moon.style.display = isDark ? 'none' : 'block';
            }
            if (label) label.textContent = isDark ? 'Light mode' : 'Dark mode';
        }
        updateThemeIcon();
        // Close dropdown on outside click
        document.addEventListener('click', function(e) {
            var dd = document.getElementById('settingsDropdown');
            if (dd && !dd.contains(e.target)) dd.classList.remove('open');
        });
    </script>
</body>
</html>`

// Login page has no header - separate layout
const loginLayoutTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} — Atlassian Admin Tool</title>
    <link rel="icon" href="/static/favicon.png" type="image/png">
    <link rel="stylesheet" href="/static/styles.css?v=20260224d">
    <script>
        (function(){var t=localStorage.getItem('theme');if(t==='dark')document.documentElement.setAttribute('data-theme','dark');})();
    </script>
</head>
<body>
    {{.Content}}
</body>
</html>`

var (
	baseTmpl  *template.Template
	loginTmpl *template.Template
)

func init() {
	baseTmpl = template.Must(template.New("base").Parse(baseLayoutTmpl))
	loginTmpl = template.Must(template.New("login").Parse(loginLayoutTmpl))
}

// RenderPage renders a full page with the base layout (header + CSS)
func RenderPage(w http.ResponseWriter, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := baseTmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Template rendering failed", http.StatusInternalServerError)
	}
}

// RenderLoginPage renders a page without the header (for login/setup pages)
func RenderLoginPage(w http.ResponseWriter, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := loginTmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Template rendering failed", http.StatusInternalServerError)
	}
}

// RenderErrorPage renders a styled error page with a message and back link
func RenderErrorPage(w http.ResponseWriter, r *http.Request, title, message, backURL, backLabel string, statusCode int) {
	w.WriteHeader(statusCode)
	isAdmin := false
	if u, err := GetCurrentUsername(r); err == nil {
		isAdmin, _ = IsAdminUser(u)
	}
	content := fmt.Sprintf(`
		<div class="ads-page-centered"><div class="ads-page-content" style="max-width:600px;">
			<div class="ads-card-flat" style="margin-top:40px;">
				<div class="ads-card-header">
					<div style="width:48px; height:48px; background:linear-gradient(135deg, #DE350B, #FF5630); border-radius:12px; display:flex; align-items:center; justify-content:center; flex-shrink:0;">
						<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><line x1="15" y1="9" x2="9" y2="15"></line><line x1="9" y1="9" x2="15" y2="15"></line></svg>
					</div>
					<div>
						<span class="ads-card-title" style="font-size:18px;">%s</span>
					</div>
				</div>
				<div style="padding:0 24px 24px;">
					<div class="ads-banner ads-banner-warning" style="margin-bottom:20px;">%s</div>
					<a href="%s" class="ads-button ads-button-default">&larr; %s</a>
				</div>
			</div>
		</div></div>
	`, html.EscapeString(title), html.EscapeString(message), backURL, html.EscapeString(backLabel))
	RenderPage(w, PageData{Title: title, IsAdmin: isAdmin, Content: template.HTML(content)})
}
