package handlers

import (
    "database/sql"
    "fmt"
    "html/template"
    "log"
    "net/http"
    "golang.org/x/crypto/bcrypt"
    "encoding/base64"
    "errors"
    "crypto/x509"
    "encoding/pem"
    "github.com/russellhaering/gosaml2"
    "github.com/russellhaering/goxmldsig"
)

type User struct {
    Username string
}

func HandleLogin(w http.ResponseWriter, r *http.Request) {
    errorMessage := r.URL.Query().Get("error")

    if r.Method == http.MethodGet {
        var usernamePasswordEnabled bool
        err := db.QueryRow("SELECT enabled FROM auth_methods WHERE method_name = 'Username and Password'").Scan(&usernamePasswordEnabled)
        if err != nil && err != sql.ErrNoRows {
            log.Printf("Error checking Username and Password status: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }

        var ssoEnabled bool
        err = db.QueryRow("SELECT enabled FROM auth_methods WHERE method_name = 'SAML SSO'").Scan(&ssoEnabled)
        if err != nil && err != sql.ErrNoRows {
            log.Printf("Error checking SSO status: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }

        content := `
            <div class="ads-login-wrapper">
                <div class="ads-login-card">
                    <img src="/static/methoda.png" class="ads-login-logo" alt="Logo" />
                    <h2 class="ads-login-title">Log in</h2>
                    <p class="ads-login-subtitle">Atlassian Admin Tool</p>`

        if errorMessage != "" {
            content += fmt.Sprintf(`<div class="ads-alert ads-alert-error">%s</div>`, errorMessage)
        }

        if usernamePasswordEnabled {
            content += `
                    <div class="ads-login-form">
                        <form action="/login" method="POST">
                            <div class="ads-form-group">
                                <label class="ads-form-label" for="username">Username</label>
                                <input class="ads-input" type="text" id="username" name="username" placeholder="Enter your username" required>
                            </div>
                            <div class="ads-form-group">
                                <label class="ads-form-label" for="password">Password</label>
                                <input class="ads-input" type="password" id="password" name="password" placeholder="Enter your password" required>
                            </div>
                            <button type="submit" class="ads-btn ads-btn-primary">Log in</button>
                        </form>
                    </div>`
        }

        if ssoEnabled && usernamePasswordEnabled {
            content += `<div class="ads-login-divider">or</div>`
        }

        if ssoEnabled {
            content += `
                    <div class="ads-login-form">
                        <form action="/sso-login" method="GET">
                            <button type="submit" class="ads-btn ads-btn-default" style="width:100%; height:40px; font-size:16px;">
                                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"></path><polyline points="10 17 15 12 10 7"></polyline><line x1="15" y1="12" x2="3" y2="12"></line></svg>
                                Connect via SSO
                            </button>
                        </form>
                    </div>`
        }

        content += `</div></div>`

        RenderLoginPage(w, PageData{
            Title:   "Log in",
            Content: template.HTML(content),
        })
    } else if r.Method == http.MethodPost {
        username := r.FormValue("username")
        password := r.FormValue("password")

        var passwordHash string
        err := db.QueryRow("SELECT password FROM users WHERE username = ?", username).Scan(&passwordHash)
        if err != nil {
            if err == sql.ErrNoRows {
                http.Redirect(w, r, "/login?error=Invalid+username+or+password", http.StatusSeeOther)
                return
            }
            log.Printf("Login error: %v", err)
            http.Redirect(w, r, "/login?error=Internal+server+error", http.StatusSeeOther)
            return
        }

        if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
            http.Redirect(w, r, "/login?error=Invalid+username+or+password", http.StatusSeeOther)
            return
        }

        session, _ := store.Get(r, "session-name")
        session.Values["authenticated"] = true
        session.Values["username"] = username
        err = session.Save(r, w)
        if err != nil {
            log.Printf("Session save error: %v", err)
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }

        log.Printf("User %s authenticated, redirecting to home page.", username)
        http.Redirect(w, r, "/", http.StatusSeeOther)
    }
}

func HandleSSOLogin(w http.ResponseWriter, r *http.Request) {
    var ssoLoginUrl string
    err := db.QueryRow(`SELECT sso_login_url FROM sso_configuration ORDER BY CASE WHEN config_name = 'default' THEN 0 ELSE 1 END, id ASC LIMIT 1`).Scan(&ssoLoginUrl)
    if err == sql.ErrNoRows {
        log.Println("No SSO configuration found")
        http.Error(w, "SSO not configured", http.StatusInternalServerError)
        return
    } else if err != nil {
        log.Printf("Error fetching SSO configuration: %v", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }
    log.Printf("Redirecting to IdP Login URL: %s", ssoLoginUrl)
    http.Redirect(w, r, ssoLoginUrl, http.StatusFound)
}

func HandleSSOCallback(w http.ResponseWriter, r *http.Request) {
    samlResponse := r.FormValue("SAMLResponse")
    if samlResponse == "" {
        log.Println("Missing SAML Response")
        http.Redirect(w, r, "/login?error=Missing SAML Response", http.StatusSeeOther)
        return
    }

    user, err := ParseSAMLResponse(samlResponse)
    if err != nil {
        log.Printf("Error parsing SAML Response: %v", err)
        http.Redirect(w, r, fmt.Sprintf("/login?error=%v", err), http.StatusSeeOther)
        return
    }

    session, _ := store.Get(r, "session-name")
    session.Values["authenticated"] = true
    session.Values["username"] = user.Username
    err = session.Save(r, w)
    if err != nil {
        log.Printf("Session save error: %v", err)
        http.Redirect(w, r, "/login?error=Session save error", http.StatusSeeOther)
        return
    }

    log.Printf("User %s authenticated via SSO, redirecting to home page.", user.Username)
    http.Redirect(w, r, "/", http.StatusSeeOther)
}

func HandleSSOLogout(w http.ResponseWriter, r *http.Request) {
    session, _ := store.Get(r, "session-name")
    session.Options.MaxAge = -1
    err := session.Save(r, w)
    if err != nil {
        log.Printf("Session save error during logout: %v", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

    var ssoLogoutUrl string
    err = db.QueryRow("SELECT sso_logout_url FROM sso_configuration WHERE config_name = ?", "default").Scan(&ssoLogoutUrl)
    if err == sql.ErrNoRows {
        log.Println("No SSO configuration found for logout")
        http.Error(w, "SSO not configured for logout", http.StatusInternalServerError)
        return
    } else if err != nil {
        log.Printf("Error fetching SSO configuration for logout: %v", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }

    http.Redirect(w, r, ssoLogoutUrl, http.StatusFound)
}

func ParseSAMLResponse(samlResponse string) (*User, error) {
    sp, err := loadSAMLServiceProvider()
    if err != nil {
        return nil, err
    }

    decodedResponse, err := base64.StdEncoding.DecodeString(samlResponse)
    if err != nil {
        return nil, errors.New("failed to decode SAML response")
    }

    assertion, err := sp.RetrieveAssertionInfo(string(decodedResponse))
    if err != nil {
        log.Printf("Failed to retrieve SAML assertion info: %v", err)
        return nil, errors.New("invalid SAML response")
    }

    if assertion.WarningInfo.InvalidTime {
        return nil, errors.New("saml assertion has expired or is not yet valid")
    }
    if assertion.NameID == "" {
        return nil, errors.New("missing NameID in SAML assertion")
    }

    username := assertion.NameID
    var existingUserID int
    err = db.QueryRow("SELECT id FROM users WHERE username = ?", username).Scan(&existingUserID)
    if err == sql.ErrNoRows {
        _, err = db.Exec("INSERT INTO users (username, password, directory, groups) VALUES (?, ?, ?, ?)", username, "", "SSO", "users")
        if err != nil {
            log.Printf("Error adding new SSO user to database: %v", err)
            return nil, errors.New("failed to add new SSO user to database")
        }
        log.Printf("New user %s added to the users table via SSO", username)
    } else if err != nil {
        log.Printf("Error checking user existence in database: %v", err)
        return nil, errors.New("failed to check user existence")
    } else {
        log.Printf("User %s already exists in the users table", username)
    }

    return &User{Username: username}, nil
}

func loadSAMLServiceProvider() (*saml2.SAMLServiceProvider, error) {
    var (
        configName  = "default"
        ssoLoginUrl, entityID, certificate, acsUrl, audienceUrl string
    )

    err := db.QueryRow(`SELECT sso_login_url, entity_id, certificate, acs_url, audience_url FROM sso_configuration WHERE config_name = ?`, configName).Scan(&ssoLoginUrl, &entityID, &certificate, &acsUrl, &audienceUrl)
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, errors.New("SSO not configured")
        }
        return nil, err
    }

    block, _ := pem.Decode([]byte(certificate))
    if block == nil {
        return nil, errors.New("failed to parse certificate")
    }

    cert, err := x509.ParseCertificate(block.Bytes)
    if err != nil {
        return nil, err
    }

    certStore := dsig.MemoryX509CertificateStore{Roots: []*x509.Certificate{cert}}

    sp := &saml2.SAMLServiceProvider{
        IdentityProviderSSOURL:      ssoLoginUrl,
        IdentityProviderIssuer:      audienceUrl,
        AssertionConsumerServiceURL: acsUrl,
        ServiceProviderIssuer:       entityID,
        SignAuthnRequests:           true,
        IDPCertificateStore:         &certStore,
        SkipSignatureValidation:     false,
    }
    return sp, nil
}
