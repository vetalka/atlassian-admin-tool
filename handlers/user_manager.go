package handlers

import (
	"fmt"
	"html/template"
	"net/http"
)

func HandleUserManagement(w http.ResponseWriter, r *http.Request) {
	username, err := GetCurrentUsername(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	isAdmin, err := IsAdminUser(username)
	if err != nil {
		http.Error(w, "Failed to check user permissions", http.StatusInternalServerError)
		return
	}

	extraHead := template.HTML(`<script>
        function loadContent(url) {
            var xhr = new XMLHttpRequest();
            xhr.open("GET", url, true);
            xhr.setRequestHeader("X-Requested-With", "XMLHttpRequest");
            xhr.onreadystatechange = function() {
                if (xhr.readyState === 4 && xhr.status === 200) {
                    document.getElementById('content-section').innerHTML = xhr.responseText;
                    // Execute any script tags in the loaded content
                    var scripts = document.getElementById('content-section').querySelectorAll('script');
                    scripts.forEach(function(oldScript) {
                        var newScript = document.createElement('script');
                        newScript.textContent = oldScript.textContent;
                        oldScript.parentNode.replaceChild(newScript, oldScript);
                    });
                } else if (xhr.readyState === 4 && xhr.status === 404) {
                    document.getElementById('content-section').innerHTML = "<p>Page not found.</p>";
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
    </script>`)

	content := fmt.Sprintf(`
        <div style="position:fixed; top:56px; left:0; right:0; z-index:99;">
            <div class="ads-settings-bar">
                <a href="/settings/users" class="active">User management</a>
                <a href="/settings/updatelicense">License</a>
            </div>
        </div>
        <div class="ads-page-with-sidebar" style="margin-top: 100px;">
            <div class="ads-sidebar" style="top: 100px; height: calc(100vh - 100px);">
                <div class="ads-sidebar-section">
                    <div class="ads-sidebar-section-title">Directory</div>
                    <a href="/settings/all-users" class="ads-sidebar-item active">
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
                    <a href="/settings/updatelicense" class="ads-sidebar-item">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.78 7.78 5.5 5.5 0 0 1 7.78-7.78zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"></path></svg>
                        License</a>
                </div>
            </div>
            <div class="ads-main-content">
                <div id="content-section">
                    <div class="ads-page-header"><h1>User Directories</h1></div>
                    <div class="ads-card-flat">
                        <table class="ads-table">
                            <thead><tr><th>Directory Name</th><th>Type</th></tr></thead>
                            <tbody><tr><td>Local Directory</td><td><span class="ads-lozenge ads-lozenge-info">Internal</span></td></tr></tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>`)

	RenderPage(w, PageData{Title: "Settings", IsAdmin: isAdmin, ExtraHead: extraHead, Content: template.HTML(content)})
}

func HandleSettingsRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
}
