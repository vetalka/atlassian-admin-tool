package handlers

import (
	"database/sql"
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"
)

func HandleDisplayLicense(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		displayLicenseForm(w, r)
	} else if r.Method == http.MethodPost {
		updateLicense(w, r)
	}
}

func displayLicenseForm(w http.ResponseWriter, r *http.Request) {
	var expiryDate string

	username, err := GetCurrentUsername(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	isAdmin, _ := IsAdminUser(username)

	err = db.QueryRow("SELECT expiry_date FROM license ORDER BY id DESC LIMIT 1").Scan(&expiryDate)
	if err != nil {
		if err == sql.ErrNoRows {
			expiryDate = "N/A"
		} else {
			log.Printf("Error querying license: %v", err)
			http.Error(w, "Failed to load current license", http.StatusInternalServerError)
			return
		}
	}

	// Calculate days remaining and status
	statusClass := "ads-lozenge-success"
	statusText := "ACTIVE"
	daysRemaining := ""
	statusBarColor := "#36B37E"
	statusBarWidth := "100"

	if expiryDate != "N/A" {
		if t, parseErr := time.Parse("2006-01-02", expiryDate); parseErr == nil {
			days := int(time.Until(t).Hours() / 24)
			if days < 0 {
				statusClass = "ads-lozenge-removed"
				statusText = "EXPIRED"
				daysRemaining = fmt.Sprintf("Expired %d days ago", -days)
				statusBarColor = "#DE350B"
				statusBarWidth = "100"
			} else if days < 30 {
				statusClass = "ads-lozenge-moved"
				statusText = "EXPIRING SOON"
				daysRemaining = fmt.Sprintf("%d days remaining", days)
				statusBarColor = "#FF991F"
				statusBarWidth = fmt.Sprintf("%d", days*100/365)
			} else {
				daysRemaining = fmt.Sprintf("%d days remaining", days)
				statusBarWidth = fmt.Sprintf("%d", min(days*100/365, 100))
			}
		}
	} else {
		statusClass = "ads-lozenge-default"
		statusText = "NO LICENSE"
		daysRemaining = "No license configured"
		statusBarColor = "#6B778C"
		statusBarWidth = "0"
	}

	safeExpiry := html.EscapeString(expiryDate)
	safeDays := html.EscapeString(daysRemaining)

	// Inner content - the license card only (used for AJAX sidebar loads)
	innerContent := fmt.Sprintf(`
                <div class="ads-page-header"><h1>License Management</h1></div>
                <div class="ads-card-flat">
                    <div class="ads-card-header">
                        <div style="width:48px; height:48px; background:linear-gradient(135deg, #6554C0, #8777D9); border-radius:12px; display:flex; align-items:center; justify-content:center;">
                            <span style="font-size:24px; color:white;">&#x1F511;</span>
                        </div>
                        <div style="flex:1;">
                            <span class="ads-card-title" style="font-size:18px;">License Status</span>
                            <div style="margin-top:4px;">
                                <span class="ads-lozenge %s" style="font-size:11px;">%s</span>
                            </div>
                        </div>
                    </div>
                    <div style="padding:0 24px 24px;">
                        <div style="display:grid; grid-template-columns:1fr 1fr; gap:16px; margin-bottom:24px;">
                            <div style="background:var(--color-bg); border:1px solid var(--color-border); border-radius:8px; padding:20px;">
                                <div style="font-size:11px; color:var(--color-text-subtle); text-transform:uppercase; letter-spacing:1px; margin-bottom:8px;">Expiration Date</div>
                                <div style="font-size:28px; font-weight:700; color:var(--color-text);">%s</div>
                            </div>
                            <div style="background:var(--color-bg); border:1px solid var(--color-border); border-radius:8px; padding:20px;">
                                <div style="font-size:11px; color:var(--color-text-subtle); text-transform:uppercase; letter-spacing:1px; margin-bottom:8px;">Status</div>
                                <div style="font-size:16px; font-weight:600; color:var(--color-text); margin-bottom:8px;">%s</div>
                                <div style="background:var(--color-border); border-radius:4px; height:6px; overflow:hidden;">
                                    <div style="background:%s; height:100%%; width:%s%%; border-radius:4px; transition:width 0.5s;"></div>
                                </div>
                            </div>
                        </div>
                        <div style="border-top:1px solid var(--color-border); margin-bottom:20px;"></div>
                        <div style="margin-bottom:8px; font-size:15px; font-weight:600; color:var(--color-text);">Update License Key</div>
                        <div style="font-size:13px; color:var(--color-text-subtle); margin-bottom:12px;">
                            Paste your new license key below to update the expiration date.
                        </div>
                        <form action="/settings/updatelicense" method="POST">
                            <div class="ads-form-group">
                                <textarea name="license" class="ads-input" rows="3" placeholder="Paste your license key here..." required
                                          style="font-family:monospace; font-size:12px; resize:vertical;"></textarea>
                            </div>
                            <div style="display:flex; gap:8px; margin-top:12px;">
                                <button type="submit" class="ads-button ads-button-primary">Update License</button>
                            </div>
                        </form>
                    </div>
                </div>
    `, statusClass, statusText, safeExpiry, safeDays,
		statusBarColor, statusBarWidth)

	// If AJAX request from sidebar, return only the inner content
	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		fmt.Fprintln(w, innerContent)
		return
	}

	// Full page load - wrap in settings layout with sidebar
	content := fmt.Sprintf(`
        <div style="position:fixed; top:56px; left:0; right:0; z-index:99;">
            <div class="ads-settings-bar">
                <a href="/settings/users">User management</a>
                <a href="/settings/updatelicense" class="active">License</a>
            </div>
        </div>
        <div class="ads-page-with-sidebar" style="margin-top: 100px;">
            <div class="ads-sidebar" style="top: 100px; height: calc(100vh - 100px);">
                <div class="ads-sidebar-section">
                    <div class="ads-sidebar-section-title">Directory</div>
                    <a href="/settings/all-users" class="ads-sidebar-item">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"></path><circle cx="9" cy="7" r="4"></circle></svg>
                        Users</a>
                    <a href="/settings/groups" class="ads-sidebar-item">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"></path><circle cx="9" cy="7" r="4"></circle></svg>
                        Groups</a>
                    <a href="/settings/local-ad" class="ads-sidebar-item">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7"></rect><rect x="14" y="3" width="7" height="7"></rect><rect x="14" y="14" width="7" height="7"></rect><rect x="3" y="14" width="7" height="7"></rect></svg>
                        Local Directory</a>
                </div>
                <div class="ads-sidebar-section">
                    <div class="ads-sidebar-section-title">Security</div>
                    <a href="/settings/auth-methods/toggle" class="ads-sidebar-item">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2"></rect><path d="M7 11V7a5 5 0 0 1 10 0v4"></path></svg>
                        Authentication</a>
                    <a href="/settings/sso" class="ads-sidebar-item">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"></path><polyline points="10 17 15 12 10 7"></polyline><line x1="15" y1="12" x2="3" y2="12"></line></svg>
                        SAML (SSO)</a>
                    <a href="/settings/user-directories" class="ads-sidebar-item">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path></svg>
                        User Directories</a>
                </div>
                <div class="ads-sidebar-section">
                    <div class="ads-sidebar-section-title">Backup</div>
                    <a href="/cron/policies" class="ads-sidebar-item" data-full="1">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>
                        Backup Policies</a>
                </div>
                <div class="ads-sidebar-section">
                    <div class="ads-sidebar-section-title">System</div>
                    <a href="/settings/updatelicense" class="ads-sidebar-item active">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.78 7.78 5.5 5.5 0 0 1 7.78-7.78zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"></path></svg>
                        License</a>
                </div>
            </div>
            <div class="ads-main-content">
                <div id="content-section">
                    %s
                </div>
            </div>
        </div>
        <script>
        function loadContent(url) {
            var xhr = new XMLHttpRequest();
            xhr.open("GET", url, true);
            xhr.setRequestHeader("X-Requested-With", "XMLHttpRequest");
            xhr.onreadystatechange = function() {
                if (xhr.readyState === 4 && xhr.status === 200) {
                    document.getElementById('content-section').innerHTML = xhr.responseText;
                    var scripts = document.getElementById('content-section').querySelectorAll('script');
                    scripts.forEach(function(oldScript) {
                        var newScript = document.createElement('script');
                        newScript.textContent = oldScript.textContent;
                        oldScript.parentNode.replaceChild(newScript, oldScript);
                    });
                }
            };
            xhr.send();
        }
        document.addEventListener('DOMContentLoaded', function() {
            document.querySelectorAll('.ads-sidebar-item').forEach(function(link) {
                link.addEventListener('click', function(event) {
                    if (this.dataset.full) return; // full-page nav — let browser handle it
                    event.preventDefault();
                    document.querySelectorAll('.ads-sidebar-item').forEach(function(l) { l.classList.remove('active'); });
                    this.classList.add('active');
                    loadContent(this.getAttribute('href'));
                });
            });
        });
        </script>
    `, innerContent)

	RenderPage(w, PageData{
		Title:   "License Management",
		IsAdmin: isAdmin,
		Content: template.HTML(content),
	})
}

func updateLicense(w http.ResponseWriter, r *http.Request) {
	newLicense := r.FormValue("license")
	newLicense = strings.TrimSpace(newLicense)

	valid, err := ValidateLicense(newLicense)
	if err != nil || !valid {
		log.Printf("Failed to update license: %v", err)
		http.Error(w, "Invalid license", http.StatusBadRequest)
		return
	}

	http.Redirect(w, r, "/settings/updatelicense", http.StatusSeeOther)
}
