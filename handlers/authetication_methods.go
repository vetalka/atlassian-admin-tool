package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

func HandleToggleAuthMethod(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	method := r.FormValue("method")
	action := r.FormValue("action") // "enable" or "disable"

	// Update the enabled state of the specified method
	enabled := 0
	if action == "enable" {
		enabled = 1
	}

	_, err := db.Exec("UPDATE auth_methods SET enabled = ? WHERE method_name = ?", enabled, method)
	if err != nil {
		http.Error(w, "Failed to update authentication method", http.StatusInternalServerError)
		return
	}

	// Redirect to the authentication methods page
	http.Redirect(w, r, "/settings/auth-methods", http.StatusSeeOther)
}

func HandleAuthMethodsPage(w http.ResponseWriter, r *http.Request) {
	// Fetch authentication methods from the database
	rows, err := db.Query("SELECT method_name, description, enabled FROM auth_methods")
	if err != nil {
		http.Error(w, "Failed to load authentication methods", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var methods []map[string]interface{}
	for rows.Next() {
		var name, description string
		var enabled bool
		rows.Scan(&name, &description, &enabled)
		methods = append(methods, map[string]interface{}{
			"name":        name,
			"description": description,
			"enabled":     enabled,
		})
	}

	html := `
    <!DOCTYPE html>
    <html lang="en">
    <head>
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>Authentication Methods</title>
        <link rel="stylesheet" href="/static/styles.css">
        
    </head>
    <body>
        <div class="auth-container">
            <h1>Authentication Methods</h1>
            <h2>Login options</h2>`

	for _, method := range methods {
		enabledText := "Disable"
		btnClass := "toggle-btn disable"
		if !method["enabled"].(bool) {
			enabledText = "Enable"
			btnClass = "toggle-btn"
		}
		html += fmt.Sprintf(`
            <div class="login-options">
                <span>%s</span>
                <span>%s</span>
                <form action="/settings/auth-methods/toggle" method="POST">
                    <input type="hidden" name="method" value="%s">
                    <input type="hidden" name="action" value="%s">
                    <button class="%s">%s</button>
                </form>
            </div>`, method["name"], method["description"], method["name"], strings.ToLower(enabledText), btnClass, enabledText)
	}

	html += `
            <button class="add-config-btn" onclick="window.location.href='/settings/sso'">Add Configuration</button>
        </div>
    </body>
    </html>`
	fmt.Fprintln(w, html)
}

// HandleAuthMethods renders the Authentication Methods management page
func HandleAuthMethods(w http.ResponseWriter, r *http.Request) {
	// Fetch the authentication methods from the database
	rows, err := db.Query("SELECT id, method_name, description, enabled FROM auth_methods")
	if err != nil {
		http.Error(w, "Failed to load authentication methods", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Build HTML table rows dynamically
	var authRows string
	for rows.Next() {
		var id int
		var methodName, description string
		var enabled bool
		if err := rows.Scan(&id, &methodName, &description, &enabled); err != nil {
			http.Error(w, "Failed to read authentication methods", http.StatusInternalServerError)
			return
		}

		// Checkbox state based on `enabled` field
		enabledChecked := ""
		if enabled {
			enabledChecked = "checked"
		}

		authRows += fmt.Sprintf(`
        <tr>
            <td>%s</td>
            <td>%s</td>
            <td><input type="checkbox" name="enabled-%d" %s></td>
        </tr>`, methodName, description, id, enabledChecked)
	}

	// Render the HTML
	html := fmt.Sprintf(`
    <!DOCTYPE html>
    <html lang="en">
    <head>
        <meta charset="UTF-8">
        <title>Authentication Methods</title>
        <link rel="stylesheet" href="/static/styles.css">
        
    </head>
    <body>
        <div class="auth-methods-container">
            <h1>Authentication Methods</h1>
            <form action="/settings/auth-methods/update" method="POST">
                <table>
                    <thead>
                        <tr>
                            <th>Method Name</th>
                            <th>Description</th>
                            <th>Enabled</th>
                        </tr>
                    </thead>
                    <tbody>
                        %s
                    </tbody>
                </table>
                <button type="submit" class="save-button">Save Changes</button>
            </form>
        </div>
    </body>
    </html>`, authRows)

	// Write the HTML to the response
	fmt.Fprintln(w, html)
}

// retryExec is a helper function to attempt database execution with retries in case of SQLITE_BUSY errors.
func retryExec(query string, args ...interface{}) error {
	var err error
	for i := 0; i < 5; i++ { // Increase retries to 5 times
		_, err = db.Exec(query, args...)
		if err == nil {
			return nil
		}

		if strings.Contains(err.Error(), "database is locked") { // Check for SQLite lock error
			log.Printf("Database is busy, retrying... (%d/5)", i+1)
			time.Sleep(300 * time.Millisecond) // Increase delay to 300ms between retries
		} else {
			return err
		}
	}
	return err
}

// HandleUpdateAuthMethods updates the authentication methods based on the form submission
func HandleUpdateAuthMethods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Start a transaction
	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Failed to begin transaction", http.StatusInternalServerError)
		return
	}

	// Get all authentication method IDs from the database
	rows, err := tx.Query("SELECT id FROM auth_methods")
	if err != nil {
		http.Error(w, "Failed to load authentication methods", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			http.Error(w, "Failed to read authentication methods", http.StatusInternalServerError)
			return
		}

		enabled := r.FormValue(fmt.Sprintf("enabled-%d", id)) == "on"

		// Use transaction for update
		_, err := tx.Exec("UPDATE auth_methods SET enabled = ? WHERE id = ?", enabled, id)
		if err != nil {
			tx.Rollback() // Roll back in case of failure
			http.Error(w, "Failed to update authentication method", http.StatusInternalServerError)
			return
		}
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		http.Error(w, "Failed to commit changes", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
}
