package handlers

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
)

// HandleCompleteEnvSetup shows a form to enter the DB password when it couldn't be extracted
func HandleCompleteEnvSetup(w http.ResponseWriter, r *http.Request) {
	envName := r.URL.Query().Get("name")
	if envName == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Handle POST — save the password
	if r.Method == http.MethodPost {
		dbPassword := r.FormValue("db_password")
		if strings.TrimSpace(dbPassword) == "" {
			renderCompleteSetupPage(w, envName, "Please enter the database password.")
			return
		}

		// Update the environment with the real password
		_, err := db.Exec(`UPDATE environments SET app_dbpass = ? WHERE name = ?`, dbPassword, envName)
		if err != nil {
			renderCompleteSetupPage(w, envName, "Error saving password: "+err.Error())
			return
		}

		// Now try to fetch base_url and cluster nodes with the real password
		var dbHost, dbPort, dbUser, dbName, dbDriver, app, serverUser, serverPassword, ip string
		db.QueryRow(`SELECT app_dbhost, app_dbport, app_dbuser, app_dbname, db_driver, app, server_user, server_password, ip FROM environments WHERE name = ?`, envName).Scan(
			&dbHost, &dbPort, &dbUser, &dbName, &dbDriver, &app, &serverUser, &serverPassword, &ip)

		dbType, _ := mapDBDriverToDBType(dbDriver)

		// Try to get base URL now that we have the password
		if dbType != "" && dbHost != "" {
			var baseURL string
			switch app {
			case "Jira":
				baseURL, _ = GetBaseURL(dbType, dbHost, serverUser, serverPassword, dbHost, dbPort, dbUser, dbPassword, dbName)
			case "Confluence":
				baseURL, _ = GetBaseURLConfluence(dbType, dbHost, serverUser, serverPassword, dbHost, dbPort, dbUser, dbPassword, dbName)
			}
			if baseURL != "" {
				db.Exec(`UPDATE environments SET base_url = ? WHERE name = ?`, baseURL, envName)
			}

			// Try cluster nodes for Jira
			if app == "Jira" {
				retrieveAndSaveClusterNodes(dbType, dbHost, dbPort, dbUser, dbPassword, dbName, serverUser, serverPassword, envName, db)
			}
		}

		log.Printf("Environment %s setup completed with user-provided DB password", envName)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// GET — show the form
	renderCompleteSetupPage(w, envName, "")
}

func renderCompleteSetupPage(w http.ResponseWriter, envName, errorMessage string) {
	// Fetch what we know about this environment
	var app, dbHost, dbPort, dbUser, dbName, dbDriver string
	db.QueryRow(`SELECT app, app_dbhost, app_dbport, app_dbuser, app_dbname, db_driver FROM environments WHERE name = ?`, envName).Scan(
		&app, &dbHost, &dbPort, &dbUser, &dbName, &dbDriver)

	errorHTML := ""
	if errorMessage != "" {
		errorHTML = fmt.Sprintf(`<div class="ads-banner ads-banner-warning" style="margin-bottom:20px;">
            <span style="color:var(--color-danger);">%s</span></div>`, errorMessage)
	}

	content := fmt.Sprintf(`
        <div class="ads-breadcrumbs"><a href="/">Environments</a> → Complete Setup</div>
        <div style="max-width:600px; margin:0 auto;">
            <div class="ads-card-flat" style="margin-top:16px;">
                <div class="ads-card-header">
                    <span class="ads-card-title">🔐 Database Password Required</span>
                </div>
                <p style="padding:0 24px; color:var(--color-text-subtle); font-size:14px;">
                    The database password for <strong>%s</strong> is encrypted in the configuration file (<code>{ATL_SECURED}</code>).
                    Please enter the database password manually to complete the setup.
                </p>
                %s
                <div style="padding:0 24px 8px;">
                    <table class="ads-table ads-table-params">
                        <tr><th>Application</th><td>%s</td></tr>
                        <tr><th>DB Host</th><td><code>%s</code></td></tr>
                        <tr><th>DB Port</th><td><code>%s</code></td></tr>
                        <tr><th>DB Name</th><td><code>%s</code></td></tr>
                        <tr><th>DB User</th><td><code>%s</code></td></tr>
                        <tr><th>DB Driver</th><td><code>%s</code></td></tr>
                    </table>
                </div>
                <form method="POST" action="/complete-env-setup?name=%s" style="padding:0 24px 24px;">
                    <div class="ads-form-group">
                        <label class="ads-form-label" for="db_password">Database password *</label>
                        <input class="ads-input" type="password" id="db_password" name="db_password" 
                               placeholder="Enter the database password for user %s" required autofocus>
                    </div>
                    <div class="ads-action-bar" style="margin-top:16px;">
                        <button type="submit" class="ads-button ads-button-primary">Save &amp; Complete Setup</button>
                        <a href="/update-environment?name=%s" class="ads-button ads-button-default" style="margin-left:8px;">Edit All Parameters</a>
                    </div>
                </form>
            </div>
        </div>
    `, envName, errorHTML, app, dbHost, dbPort, dbName, dbUser, dbDriver, envName, dbUser, envName)

	RenderPage(w, PageData{
		Title:   "Complete Environment Setup",
		IsAdmin: true,
		Content: template.HTML(content),
	})
}
