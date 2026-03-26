package handlers

import (
    "fmt"
    "html"
    "html/template"
    "net/http"
	"log"
	"database/sql"
)

func HandleSSOList(w http.ResponseWriter, r *http.Request) {
    log.Println("HandleSSOList was called")

    username, err := GetCurrentUsername(r)
    if err != nil {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }
    isAdmin, _ := IsAdminUser(username)

    // Fetch all SSO configurations from the database
    rows, err := db.Query("SELECT id, config_name FROM sso_configuration")
    if err != nil {
        http.Error(w, "Failed to fetch SSO configurations", http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    // Build table rows
    tableRows := ""
    configurationsExist := false
    for rows.Next() {
        var id int
        var configName string
        err := rows.Scan(&id, &configName)
        if err != nil {
            http.Error(w, "Failed to parse SSO configurations", http.StatusInternalServerError)
            return
        }
        configurationsExist = true
        tableRows += fmt.Sprintf(`
            <tr>
                <td>
                    <div style="display:flex; align-items:center; gap:10px;">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--color-success)" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect><path d="M7 11V7a5 5 0 0 1 10 0v4"></path></svg>
                        %s
                    </div>
                </td>
                <td>
                    <a href="/settings/sso/edit?id=%d" class="ads-btn ads-btn-default" style="font-size:13px; padding:4px 12px;">Edit</a>
                </td>
            </tr>`, html.EscapeString(configName), id)
    }

    bodyContent := ""
    if configurationsExist {
        bodyContent = fmt.Sprintf(`
            <table class="ads-table" style="width:100%%;">
                <thead><tr><th>Configuration Name</th><th style="width:100px;">Actions</th></tr></thead>
                <tbody>%s</tbody>
            </table>`, tableRows)
    } else {
        bodyContent = `
            <div style="text-align:center; padding:40px 20px;">
                <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="var(--color-text-subtlest)" stroke-width="1.5" style="margin-bottom:16px;"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect><path d="M7 11V7a5 5 0 0 1 10 0v4"></path></svg>
                <p style="color:var(--color-text-subtle); margin-bottom:8px;">No SSO configurations found.</p>
                <p style="font-size:13px; color:var(--color-text-subtlest);">Create a SAML configuration to enable single sign-on for your users.</p>
            </div>`
    }

    content := fmt.Sprintf(`
    <div class="ads-page-centered"><div class="ads-page-content">
        <div class="ads-breadcrumbs">
            <a href="/settings/users">User Management</a> &rarr; SSO Configurations
        </div>
        <div class="ads-card-flat" style="margin-top:16px;">
            <div class="ads-card-header">
                <div style="width:48px; height:48px; background:linear-gradient(135deg, #006644, #36B37E); border-radius:12px; display:flex; align-items:center; justify-content:center; flex-shrink:0;">
                    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect><path d="M7 11V7a5 5 0 0 1 10 0v4"></path></svg>
                </div>
                <div style="flex:1;">
                    <span class="ads-card-title" style="font-size:18px;">SAML SSO Configurations</span>
                    <div style="font-size:13px; color:var(--color-text-subtle); margin-top:2px;">Manage single sign-on identity providers</div>
                </div>
                <a href="/settings/sso/create" class="ads-btn ads-btn-primary" style="flex-shrink:0;">+ New Configuration</a>
            </div>
            <div style="padding:0 16px 24px;">
                %s
                <div style="margin-top:16px; padding-top:16px; border-top:1px solid var(--color-border);">
                    <a href="/settings/users" class="ads-button ads-button-default">&larr; Back to User Management</a>
                </div>
            </div>
        </div>
    </div></div>`, bodyContent)

    RenderPage(w, PageData{
        Title:   "SSO Configurations",
        IsAdmin: isAdmin,
        Content: template.HTML(content),
    })
}

func getBaseURL(r *http.Request) string {
    // Extract the base URL dynamically from the request
    scheme := "http"
    if r.TLS != nil {
        scheme = "https"
    }
    return fmt.Sprintf("%s://%s", scheme, r.Host)
}

// renderSSOFormContent generates SSO form HTML content for both create and edit
func renderSSOFormContent(title, action, id, configName, ssoLoginUrl, ssoLogoutUrl, certificate, usernameMapping, acsUrl, audienceUrl string, jitProvisioning, rememberUserLogins bool, loginButtonText string) string {
    hiddenID := ""
    if id != "" {
        hiddenID = fmt.Sprintf(`<input type="hidden" name="id" value="%s">`, html.EscapeString(id))
    }
    jitChecked := ""
    if jitProvisioning {
        jitChecked = "checked"
    }
    rememberChecked := ""
    if rememberUserLogins {
        rememberChecked = "checked"
    }
    buttonText := "Save Configuration"
    if id != "" {
        buttonText = "Save Changes"
    }

    return fmt.Sprintf(`
    <div class="ads-page-centered"><div class="ads-page-content">
        <div class="ads-breadcrumbs">
            <a href="/settings/users">User Management</a> &rarr;
            <a href="/settings/sso">SSO</a> &rarr;
            %s
        </div>
        <div class="ads-card-flat" style="margin-top:16px;">
            <div class="ads-card-header">
                <div style="width:48px; height:48px; background:linear-gradient(135deg, #006644, #36B37E); border-radius:12px; display:flex; align-items:center; justify-content:center; flex-shrink:0;">
                    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"></rect><path d="M7 11V7a5 5 0 0 1 10 0v4"></path></svg>
                </div>
                <div>
                    <span class="ads-card-title" style="font-size:18px;">%s</span>
                    <div style="font-size:13px; color:var(--color-text-subtle); margin-top:2px;">Configure SAML 2.0 single sign-on for your identity provider</div>
                </div>
            </div>
            <form action="%s" method="POST" style="padding:0 16px 24px;">
                %s
                <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(340px, 1fr)); gap:24px;">
                    <div>
                        <h4 style="margin-bottom:16px; color:var(--color-text);">SAML Settings</h4>
                        <div style="margin-bottom:14px;">
                            <label style="font-weight:600; font-size:13px; color:var(--color-text-subtle); display:block; margin-bottom:4px;">Configuration Name</label>
                            <input type="text" name="configName" value="%s" class="ads-input" style="width:100%%;" required>
                        </div>
                        <div style="margin-bottom:14px;">
                            <label style="font-weight:600; font-size:13px; color:var(--color-text-subtle); display:block; margin-bottom:4px;">SSO Login URL</label>
                            <input type="text" name="ssoLoginUrl" value="%s" class="ads-input" style="width:100%%;" required placeholder="https://idp.example.com/saml/login">
                        </div>
                        <div style="margin-bottom:14px;">
                            <label style="font-weight:600; font-size:13px; color:var(--color-text-subtle); display:block; margin-bottom:4px;">SSO Logout URL</label>
                            <input type="text" name="ssoLogoutUrl" value="%s" class="ads-input" style="width:100%%;" placeholder="https://idp.example.com/saml/logout">
                        </div>
                        <div style="margin-bottom:14px;">
                            <label style="font-weight:600; font-size:13px; color:var(--color-text-subtle); display:block; margin-bottom:4px;">X.509 Certificate (PEM Format)</label>
                            <textarea name="certificate" class="ads-input" style="width:100%%; min-height:120px; font-family:monospace; font-size:12px;" placeholder="-----BEGIN CERTIFICATE-----&#10;...&#10;-----END CERTIFICATE-----">%s</textarea>
                        </div>
                        <div style="margin-bottom:14px;">
                            <label style="font-weight:600; font-size:13px; color:var(--color-text-subtle); display:block; margin-bottom:4px;">Username Mapping</label>
                            <input type="text" name="usernameMapping" value="%s" class="ads-input" style="width:100%%;" required>
                        </div>
                    </div>
                    <div>
                        <h4 style="margin-bottom:16px; color:var(--color-text);">Identity Provider URLs</h4>
                        <div style="padding:16px; background:var(--color-bg); border-radius:8px; border:1px solid var(--color-border); margin-bottom:16px;">
                            <p style="font-size:13px; color:var(--color-text-subtle); margin-bottom:12px;">Provide these URLs to your identity provider:</p>
                            <div style="margin-bottom:14px;">
                                <label style="font-weight:600; font-size:13px; color:var(--color-text-subtle); display:block; margin-bottom:4px;">Assertion Consumer Service (ACS) URL</label>
                                <input type="text" name="acsUrl" value="%s" class="ads-input" style="width:100%%; background:var(--color-bg-card);" readonly required>
                            </div>
                            <div>
                                <label style="font-weight:600; font-size:13px; color:var(--color-text-subtle); display:block; margin-bottom:4px;">Audience URL (Entity ID)</label>
                                <input type="text" name="audienceUrl" value="%s" class="ads-input" style="width:100%%; background:var(--color-bg-card);" readonly required>
                            </div>
                        </div>
                        <h4 style="margin-bottom:16px; color:var(--color-text);">Options</h4>
                        <label style="display:flex; align-items:center; gap:10px; padding:10px 0; cursor:pointer;">
                            <input type="checkbox" name="jitProvisioning" %s style="width:18px; height:18px;">
                            <span style="font-size:14px;">Enable Just-in-Time (JIT) Provisioning</span>
                        </label>
                        <label style="display:flex; align-items:center; gap:10px; padding:10px 0; cursor:pointer;">
                            <input type="checkbox" name="rememberUserLogins" %s style="width:18px; height:18px;">
                            <span style="font-size:14px;">Remember User Logins</span>
                        </label>
                        <div style="margin-top:14px;">
                            <label style="font-weight:600; font-size:13px; color:var(--color-text-subtle); display:block; margin-bottom:4px;">Login Button Text</label>
                            <input type="text" name="loginButtonText" value="%s" class="ads-input" style="width:100%%;">
                        </div>
                    </div>
                </div>
                <div style="margin-top:20px; padding-top:16px; border-top:1px solid var(--color-border); display:flex; align-items:center; gap:12px;">
                    <button type="submit" class="ads-btn ads-btn-primary">%s</button>
                    <a href="/settings/users" class="ads-button ads-button-default">&larr; Back to User Management</a>
                </div>
            </form>
        </div>
    </div></div>`,
        title, title, action, hiddenID,
        html.EscapeString(configName), html.EscapeString(ssoLoginUrl), html.EscapeString(ssoLogoutUrl),
        html.EscapeString(certificate), html.EscapeString(usernameMapping),
        html.EscapeString(acsUrl), html.EscapeString(audienceUrl),
        jitChecked, rememberChecked, html.EscapeString(loginButtonText), buttonText)
}

func HandleSSOForm(w http.ResponseWriter, r *http.Request) {
    // Get the current logged-in username to check if the user is an admin
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

    baseURL := getBaseURL(r)
    acsURL := fmt.Sprintf("%s/plugins/servlet/saml/consumer", baseURL)
    audienceURL := baseURL

    content := renderSSOFormContent("SAML SSO Configuration", "/settings/save-sso", "",
        "", "", "", "", "${NameID}", acsURL, audienceURL, false, false, "Continue with IdP")

    RenderPage(w, PageData{
        Title:   "SAML SSO Configuration",
        IsAdmin: isAdmin,
        Content: template.HTML(content),
    })
}

func HandleSaveSSO(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // Parse form values
    configName := r.FormValue("configName") // Note: Added configName to the form
    ssoLoginUrl := r.FormValue("ssoLoginUrl")
    ssoLogoutUrl := r.FormValue("ssoLogoutUrl")
    certificate := r.FormValue("certificate")
    acsUrl := r.FormValue("acsUrl")
    audienceUrl := r.FormValue("audienceUrl")
    usernameMapping := r.FormValue("usernameMapping")
    jitProvisioning := r.FormValue("jitProvisioning") == "on"
    rememberUserLogins := r.FormValue("rememberUserLogins") == "on"
    loginButtonText := r.FormValue("loginButtonText")

    // Log the parsed form values for debugging
    log.Printf("Saving SSO Configuration: configName=%s, ssoLoginUrl=%s, acsUrl=%s, audienceUrl=%s, jitProvisioning=%t, rememberUserLogins=%t",
        configName, ssoLoginUrl, acsUrl, audienceUrl, jitProvisioning, rememberUserLogins)

    // Check if the configuration with the given configName already exists
    var existingID int
    err := db.QueryRow(`SELECT id FROM sso_configuration WHERE config_name = ?`, configName).Scan(&existingID)
    if err != nil && err != sql.ErrNoRows {
        log.Printf("Error checking existing SSO configuration: %v", err)
        http.Error(w, "Failed to check SSO configuration", http.StatusInternalServerError)
        return
    }

    if err == sql.ErrNoRows {
        // If no existing record, insert a new one
        _, err := db.Exec(`
            INSERT INTO sso_configuration (config_name, sso_login_url, sso_logout_url, certificate, acs_url, audience_url, username_mapping, jit_provisioning, remember_user_logins, login_button_text) 
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
            configName, ssoLoginUrl, ssoLogoutUrl, certificate, acsUrl, audienceUrl, usernameMapping, jitProvisioning, rememberUserLogins, loginButtonText)
        if err != nil {
            log.Printf("Error saving new SSO configuration: %v", err)
            http.Error(w, "Failed to save SSO configuration", http.StatusInternalServerError)
            return
        }
    } else {
        // If a record already exists, update it
        _, err := db.Exec(`
            UPDATE sso_configuration SET 
                sso_login_url = ?, 
                sso_logout_url = ?, 
                certificate = ?, 
                acs_url = ?, 
                audience_url = ?, 
                username_mapping = ?, 
                jit_provisioning = ?, 
                remember_user_logins = ?, 
                login_button_text = ? 
            WHERE config_name = ?`,
            ssoLoginUrl, ssoLogoutUrl, certificate, acsUrl, audienceUrl, usernameMapping, jitProvisioning, rememberUserLogins, loginButtonText, configName)
        if err != nil {
            log.Printf("Error updating existing SSO configuration: %v", err)
            http.Error(w, "Failed to update SSO configuration", http.StatusInternalServerError)
            return
        }
    }

    // Update the auth_methods table to ensure SAML SSO is enabled
    _, err = db.Exec(`
        INSERT INTO auth_methods (method_name, description, enabled) 
        VALUES ('SAML SSO', 'SAML-based Single Sign-On', 1)
        ON CONFLICT(method_name) DO UPDATE SET enabled=1`)
    if err != nil {
        log.Printf("Error updating auth_methods: %v", err)
        http.Error(w, "Failed to save authentication method", http.StatusInternalServerError)
        return
    }

    // Redirect to the SSO page with a success message
    http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
}

func HandleEditSSOForm(w http.ResponseWriter, r *http.Request) {
	// Get the current logged-in username to check if the user is an admin
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

    // Extract the ID from the URL query
    id := r.URL.Query().Get("id")
    if id == "" {
        http.Error(w, "Missing SSO configuration ID", http.StatusBadRequest)
        return
    }

    // Fetch the SSO configuration from the database
    var configName, ssoLoginUrl, ssoLogoutUrl, certificate, usernameMapping, acsUrl, audienceUrl, loginButtonText string
    var jitProvisioning, rememberUserLogins bool
    err = db.QueryRow(`SELECT config_name, sso_login_url, sso_logout_url, certificate, username_mapping, acs_url, audience_url, jit_provisioning, remember_user_logins, login_button_text 
                        FROM sso_configuration WHERE id = ?`, id).Scan(&configName, &ssoLoginUrl, &ssoLogoutUrl, &certificate, &usernameMapping, &acsUrl, &audienceUrl, &jitProvisioning, &rememberUserLogins, &loginButtonText)
    if err != nil {
        log.Printf("Error fetching SSO configuration with ID %s: %v", id, err)
        http.Error(w, "Failed to fetch SSO configuration", http.StatusInternalServerError)
        return
    }

    content := renderSSOFormContent("Edit SAML SSO Configuration", "/settings/update-sso", id,
        configName, ssoLoginUrl, ssoLogoutUrl, certificate, usernameMapping, acsUrl, audienceUrl,
        jitProvisioning, rememberUserLogins, loginButtonText)

    RenderPage(w, PageData{
        Title:   "Edit SSO: " + configName,
        IsAdmin: isAdmin,
        Content: template.HTML(content),
    })
}

// Helper function to add 'checked' attribute if the boolean value is true
func checkboxChecked(value bool) string {
    if value {
        return "checked"
    }
    return ""
}

func HandleUpdateSSO(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // Parse form values
    id := r.FormValue("id")
    configName := r.FormValue("configName")
    ssoLoginUrl := r.FormValue("ssoLoginUrl")
    ssoLogoutUrl := r.FormValue("ssoLogoutUrl")
    certificate := r.FormValue("certificate")
    usernameMapping := r.FormValue("usernameMapping")
    acsUrl := r.FormValue("acsUrl")
    audienceUrl := r.FormValue("audienceUrl")
    jitProvisioning := r.FormValue("jitProvisioning") == "on"
    rememberUserLogins := r.FormValue("rememberUserLogins") == "on"
    loginButtonText := r.FormValue("loginButtonText")

    // Update the record in the database
    _, err := db.Exec(`
        UPDATE sso_configuration 
        SET config_name = ?, sso_login_url = ?, sso_logout_url = ?, certificate = ?, 
            username_mapping = ?, acs_url = ?, audience_url = ?, 
            jit_provisioning = ?, remember_user_logins = ?, login_button_text = ?
        WHERE id = ?`,
        configName, ssoLoginUrl, ssoLogoutUrl, certificate, usernameMapping, acsUrl, audienceUrl,
        jitProvisioning, rememberUserLogins, loginButtonText, id,
    )
    if err != nil {
        log.Printf("Error updating SSO configuration with ID %s: %v", id, err)
        http.Error(w, "Failed to update SSO configuration", http.StatusInternalServerError)
        return
    }

    // Redirect to the SSO list page or show a success message
    http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
}
