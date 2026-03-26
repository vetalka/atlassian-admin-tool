package handlers

import (
    "fmt"
    "html/template"
    "log"
    "net/http"
    "strings"
)

// Check if the current user is in the administrators group
func IsAdminUser(username string) (bool, error) {
    var group string
    err := db.QueryRow("SELECT groups FROM users WHERE username = ?", username).Scan(&group)
    if err != nil {
        return false, err
    }
    return group == "administrators", nil
}

func HandleHome(w http.ResponseWriter, r *http.Request) {
    if !IsLicenseSetUp() {
        http.Redirect(w, r, "/license-setup", http.StatusSeeOther)
        return
    }

    username, err := GetCurrentUsername(r)
    if err != nil {
        log.Printf("Failed to get current user: %v", err)
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    isAdmin, err := IsAdminUser(username)
    if err != nil {
        log.Printf("Failed to check if user is admin: %v", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

    rows, err := db.Query("SELECT app, name FROM environments")
    if err != nil {
        log.Printf("Failed to query environments: %v", err)
        http.Error(w, "Failed to load environments", http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    environmentsByApp := make(map[string][]string)
    for rows.Next() {
        var app, name string
        if err := rows.Scan(&app, &name); err != nil {
            log.Printf("Failed to scan environment row: %v", err)
            continue
        }
        environmentsByApp[app] = append(environmentsByApp[app], name)
    }

    content := `
        <div class="ads-page-centered">
            <div class="ads-page-content">
                <div class="ads-page-header">
                    <h1>Environments</h1>
                    <p class="ads-page-header-description">Select an environment to manage, or add a new one.</p>
                </div>`

    for app, envs := range environmentsByApp {
        appLower := strings.ToLower(app)
        appInitial := string(app[0])

        content += fmt.Sprintf(`
                <div class="ads-env-section">
                    <div class="ads-env-section-title">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"></rect><line x1="8" y1="21" x2="16" y2="21"></line><line x1="12" y1="17" x2="12" y2="21"></line></svg>
                        %s
                    </div>
                    <div class="ads-env-grid">`, app)

        for _, name := range envs {
            content += fmt.Sprintf(`
                        <a href="/environment/%s" class="ads-env-card">
                            <div class="ads-env-card-icon %s">%s</div>
                            <div>
                                <div class="ads-env-card-name">%s</div>
                                <div class="ads-env-card-type">%s</div>
                            </div>
                        </a>`, name, appLower, appInitial, name, app)
        }
        content += `</div></div>`
    }

    if len(environmentsByApp) == 0 {
        content += `
                <div class="ads-card-flat" style="text-align:center; padding: 64px 24px;">
                    <h3 style="color: var(--color-text-subtle); margin-bottom: 8px;">No environments configured</h3>
                    <p style="color: var(--color-text-subtlest);">Add your first Atlassian environment to get started.</p>
                </div>`
    }

    content += `
                <div class="ads-action-bar">`

    if isAdmin {
        content += `
                    <a href="/add-environment" class="ads-btn ads-btn-primary">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"></line><line x1="5" y1="12" x2="19" y2="12"></line></svg>
                        Add environment
                    </a>
                    <a href="/delete-environment" class="ads-btn ads-btn-danger">Delete environment</a>
                    <a href="/cleanup-backups" class="ads-btn ads-btn-warning">Cleanup backups</a>`
    }

    content += `
                </div>
            </div>
        </div>`

    RenderPage(w, PageData{
        Title:   "Home",
        IsAdmin: isAdmin,
        Content: template.HTML(content),
    })
}

// GetCurrentUsername retrieves the username from the session
func GetCurrentUsername(r *http.Request) (string, error) {
    session, _ := store.Get(r, "session-name")
    username, ok := session.Values["username"].(string)
    if !ok || username == "" {
        return "", fmt.Errorf("username not found in session")
    }
    return username, nil
}
