package handlers

import (
    "database/sql"
    "fmt"
    "html/template"
    "log"
    "net/http"
    "strings"
)

func handleApplication(app, ip, serverUser, serverPassword, name string) error {
    switch app {
    case "Jira":      return handleJiraEnvironment(ip, serverUser, serverPassword, name)
    case "Confluence": return handleConfluenceEnvironment(ip, serverUser, serverPassword, name)
    case "Bitbucket":  return handleBitbucketEnvironment(ip, serverUser, serverPassword, name)
    default:           return fmt.Errorf("unsupported application: %s", app)
    }
}

// isSecuredPassword checks if the password is an Atlassian secured/encrypted marker
func isSecuredPassword(pass string) bool {
    p := strings.TrimSpace(pass)
    return p == "" || strings.Contains(p, "{ATL_SECURED}") || strings.Contains(p, "{ENCRYPTED}")
}

func HandleAddEnvironment(w http.ResponseWriter, r *http.Request) {
    var errorMessage string
    if r.Method == http.MethodPost {
        app := r.FormValue("app"); name := r.FormValue("name"); ip := r.FormValue("ip")
        serverUser := r.FormValue("server_user"); serverPassword := r.FormValue("server_password")
        dbConnType := r.FormValue("db_connection_type")
        dbServerUser := r.FormValue("db_server_user")
        dbServerPassword := r.FormValue("db_server_password")

        if dbConnType == "" {
            dbConnType = "ssh"
        }

        var existingName string
        err := db.QueryRow("SELECT name FROM environments WHERE name = ?", name).Scan(&existingName)
        if err != nil && err != sql.ErrNoRows {
            errorMessage = "Error: Could not check for duplicate environment name."
        } else if existingName != "" {
            errorMessage = "Error: Environment with this name already exists."
        } else {
            if errorMessage == "" {
                err = handleApplication(app, ip, serverUser, serverPassword, name)
                if err != nil {
                    errStr := err.Error()
                    if strings.HasPrefix(errStr, "NEEDS_DB_PASSWORD:") {
                        // Environment saved but needs DB password — redirect to complete setup
                        envName := strings.TrimPrefix(errStr, "NEEDS_DB_PASSWORD:")
                        // Also save the WinRM/DB server settings
                        db.Exec(`UPDATE environments SET db_connection_type=?, db_server_user=?, db_server_password=? WHERE name=?`,
                            dbConnType, dbServerUser, dbServerPassword, envName)
                        http.Redirect(w, r, "/complete-env-setup?name="+envName, http.StatusSeeOther); return
                    }
                    log.Printf("Failed to process application: %v", err)
                    errorMessage = "Error: " + err.Error()
                } else {
                    // Save DB server credentials
                    _, err = db.Exec(`UPDATE environments SET db_connection_type=?, db_server_user=?, db_server_password=? WHERE name=?`,
                        dbConnType, dbServerUser, dbServerPassword, name)
                    if err != nil {
                        log.Printf("Warning: failed to save DB server credentials: %v", err)
                    }
                    http.Redirect(w, r, "/", http.StatusSeeOther); return
                }
            }
        }
    }

    errHtml := ""
    if errorMessage != "" {
        errHtml = fmt.Sprintf(`<div class="ads-alert ads-alert-error">%s</div>`, errorMessage)
    }

    content := fmt.Sprintf(`
        <div class="ads-page-centered">
            <div class="ads-page-content" style="max-width: 520px;">
                <div class="ads-page-header">
                    <div class="ads-breadcrumb">
                        <a href="/">Environments</a><span class="ads-breadcrumb-separator">/</span>
                        <span class="ads-breadcrumb-current">Add environment</span>
                    </div>
                    <h1>Add a new environment</h1>
                    <p class="ads-page-header-description">Connect to a remote Atlassian instance by providing its SSH details.</p>
                </div>
                %s
                <div class="ads-card-flat">
                    <form action="/add-environment" method="POST" class="ads-form-wide">
                        <div class="ads-form-group">
                            <label class="ads-form-label" for="app">Application *</label>
                            <select class="ads-input ads-select" id="app" name="app" required>
                                <option value="Jira">Jira</option><option value="Confluence">Confluence</option><option value="Bitbucket">Bitbucket</option>
                            </select>
                        </div>
                        <div class="ads-form-group">
                            <label class="ads-form-label" for="name">Environment name *</label>
                            <input class="ads-input" type="text" id="name" name="name" placeholder="e.g. jira-prod" required>
                        </div>
                        <div class="ads-form-group">
                            <label class="ads-form-label" for="ip">Server IP address *</label>
                            <input class="ads-input" type="text" id="ip" name="ip" placeholder="e.g. 192.168.1.100" required>
                        </div>
                        <div class="ads-form-group">
                            <label class="ads-form-label" for="server_user">Remote server user *</label>
                            <input class="ads-input" type="text" id="server_user" name="server_user" placeholder="SSH username" required>
                        </div>
                        <div class="ads-form-group">
                            <label class="ads-form-label" for="server_password">Remote server password *</label>
                            <input class="ads-input" type="password" id="server_password" name="server_password" placeholder="SSH password" required>
                        </div>

                        <div class="ads-card-header" style="margin-top:24px; padding-top:16px; border-top:1px solid var(--color-border);">
                            <span class="ads-card-title">Database Server Connection</span>
                        </div>
                        <p style="font-size:13px; color:var(--color-text-subtle); margin-bottom:16px;">
                            If your database runs on a separate Windows server (SQL Server), select WinRM and provide credentials below.
                            The DB host will be detected automatically from dbconfig.xml after the environment is added.
                            Leave as SSH if the database is on the same Linux server.
                        </p>
                        <div class="ads-form-group">
                            <label class="ads-form-label" for="db_connection_type">DB server connection type</label>
                            <select class="ads-input ads-select" id="db_connection_type" name="db_connection_type" onchange="toggleDBServerFields()">
                                <option value="ssh">SSH (Linux / same server)</option>
                                <option value="winrm">WinRM (Windows SQL Server)</option>
                            </select>
                        </div>
                        <div id="db-server-fields" style="display:none;">
                            <div class="ads-form-group">
                                <label class="ads-form-label" for="db_server_user">DB server Windows username</label>
                                <input class="ads-input" type="text" id="db_server_user" name="db_server_user" placeholder="e.g. DOMAIN\Administrator or administrator">
                            </div>
                            <div class="ads-form-group">
                                <label class="ads-form-label" for="db_server_password">DB server Windows password</label>
                                <input class="ads-input" type="password" id="db_server_password" name="db_server_password" placeholder="Windows password for WinRM">
                            </div>
                        </div>

                        <div class="ads-action-bar">
                            <button type="submit" class="ads-btn ads-btn-primary">Add environment</button>
                            <a href="/" class="ads-btn ads-btn-default">Cancel</a>
                        </div>
                    </form>
                </div>
            </div>
        </div>`, errHtml)

    extraHead := template.HTML(`<script>
        function toggleDBServerFields() {
            var connType = document.getElementById('db_connection_type').value;
            var fields = document.getElementById('db-server-fields');
            fields.style.display = connType === 'winrm' ? 'block' : 'none';
        }
    </script>`)

    RenderPage(w, PageData{Title: "Add Environment", IsAdmin: true, ExtraHead: extraHead, Content: template.HTML(content)})
}

func retrieveAndSaveClusterNodes(dbType, dbHost, dbPort, dbUser, dbPass, dbName, serverUser, serverPassword, envName string, db *sql.DB) error {
    var ips string; var err error
    switch dbType {
    case "postgresql": ips, err = retrieveClusterInfoPostgreSQLSSH(dbHost, serverUser, serverPassword, dbHost, dbPort, dbUser, dbPass, dbName)
    case "sqlserver":  ips, err = retrieveClusterInfoSQLServerSSH(dbHost, serverUser, serverPassword, dbHost, dbPort, dbUser, dbPass, dbName)
    default: return fmt.Errorf("unsupported database type: %s", dbType)
    }
    if err != nil { return fmt.Errorf("failed to retrieve cluster nodes: %v", err) }
    if ips != "" {
        err = saveIPsToEnvironment(envName, ips, db)
        if err != nil { return fmt.Errorf("failed to save IPs: %v", err) }
    }
    return nil
}

func mapDBDriverToDBType(dbDriver string) (string, error) {
    switch dbDriver {
    case "org.postgresql.Driver": return "postgresql", nil
    case "com.microsoft.sqlserver.jdbc.SQLServerDriver": return "sqlserver", nil
    default: return "", fmt.Errorf("unsupported database driver: %s", dbDriver)
    }
}

func handleJiraEnvironment(ip, serverUser, serverPassword, name string) error {
    if err := CheckSSHConnection(ip, serverUser, serverPassword); err != nil { return fmt.Errorf("SSH validation failed: %v", err) }
    installDir, dataPath, sharedHomeDir, err := getJiraInstallDir(ip, serverUser, serverPassword)
    if err != nil { return fmt.Errorf("Failed to retrieve Jira directories: %v", err) }
    dbHost, dbPort, dbName, dbUser, dbPass, dbDriver, err := executeDBParamsExtraction(ip, serverUser, serverPassword, dataPath, installDir)
    if err != nil { return fmt.Errorf("Failed to retrieve database parameters: %v", err) }
    dbType, err := mapDBDriverToDBType(dbDriver)
    if err != nil { return fmt.Errorf("Failed to map database driver: %v", err) }

    // Check if password is secured/encrypted — can't query DB without real password
    needsPassword := isSecuredPassword(dbPass)

    eazybiDBName, eazybiDBHost, eazybiDBPass, eazybiDBPort, eazybiDBUser := "", "", "", "", ""
    baseURL := ""

    if !needsPassword {
        eazybiDBName, eazybiDBHost, eazybiDBPass, eazybiDBPort, eazybiDBUser, _ = executeEazyBIDBParamsExtraction(ip, serverUser, serverPassword, dataPath, sharedHomeDir)
        baseURL, _ = GetBaseURL(dbType, dbHost, serverUser, serverPassword, dbHost, dbPort, dbUser, dbPass, dbName)
    }

    // Save environment — even with {ATL_SECURED} as password
    if needsPassword {
        dbPass = "" // Don't save the marker, save empty so user fills it in
    }
    _, err = db.Exec(`INSERT INTO environments (app, name, ip, server_user, server_password, install_dir, home_dir, sharedhome_dir, app_dbname, app_dbuser, app_dbpass, app_dbport, app_dbhost, db_driver, eazybi_dbname, eazybi_dbuser, eazybi_dbpass, eazybi_dbport, eazybi_dbhost, base_url) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        "Jira", name, ip, serverUser, serverPassword, installDir, dataPath, sharedHomeDir, dbName, dbUser, dbPass, dbPort, dbHost, dbDriver, eazybiDBName, eazybiDBUser, eazybiDBPass, eazybiDBPort, eazybiDBHost, baseURL)
    if err != nil { return fmt.Errorf("Failed to add environment: %v", err) }

    if needsPassword {
        return fmt.Errorf("NEEDS_DB_PASSWORD:%s", name)
    }

    return retrieveAndSaveClusterNodes(dbType, dbHost, dbPort, dbUser, dbPass, dbName, serverUser, serverPassword, name, db)
}

func handleConfluenceEnvironment(ip, serverUser, serverPassword, name string) error {
    if err := CheckSSHConnection(ip, serverUser, serverPassword); err != nil { return fmt.Errorf("SSH validation failed: %v", err) }
    installDir, homeDir, err := GetConfluenceInstallDir(ip, serverUser, serverPassword)
    if err != nil { return fmt.Errorf("Failed to retrieve Confluence directories: %v", err) }
    sharedHomeDir, err := extractSharedHomeDirConfluence(ip, serverUser, serverPassword, homeDir)
    if err != nil { return fmt.Errorf("Failed to retrieve shared home directory: %v", err) }
    dbHost, dbPort, dbName, dbUser, dbPass, dbDriver, err := extractConfluenceDBParams(ip, serverUser, serverPassword, homeDir)
    if err != nil { return fmt.Errorf("Failed to retrieve database parameters: %v", err) }
    dbType, err := mapDBDriverToDBType(dbDriver)
    if err != nil { return fmt.Errorf("Failed to map database driver: %v", err) }

    needsPassword := isSecuredPassword(dbPass)

    baseURL := ""
    clusterNodesStr := ip
    if !needsPassword {
        baseURL, _ = GetBaseURLConfluence(dbType, dbHost, serverUser, serverPassword, dbHost, dbPort, dbUser, dbPass, dbName)
        clusterNodes, err := getClusterNodes(ip, serverUser, serverPassword, homeDir+"/confluence.cfg.xml")
        if err == nil && len(clusterNodes) > 0 {
            clusterNodesStr = strings.Join(clusterNodes, " ")
        }
    }

    if needsPassword {
        dbPass = ""
    }
    _, err = db.Exec(`INSERT INTO environments (app, name, ip, server_user, server_password, install_dir, home_dir, sharedhome_dir, app_dbname, app_dbuser, app_dbpass, app_dbport, app_dbhost, db_driver, eazybi_dbname, eazybi_dbuser, eazybi_dbpass, eazybi_dbport, eazybi_dbhost, base_url) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        "Confluence", name, clusterNodesStr, serverUser, serverPassword, installDir, homeDir, sharedHomeDir, dbName, dbUser, dbPass, dbPort, dbHost, dbDriver, "", "", "", "", "", baseURL)
    if err != nil { return fmt.Errorf("Failed to insert environment: %v", err) }

    if needsPassword {
        return fmt.Errorf("NEEDS_DB_PASSWORD:%s", name)
    }
    return nil
}

func handleBitbucketEnvironment(ip, serverUser, serverPassword, name string) error { return nil }
