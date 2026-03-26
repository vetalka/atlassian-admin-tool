package handlers

import (
    "fmt"
    "html/template"
    "log"
    "net/http"
    "strings"
)

func HandleShowEnvironment(w http.ResponseWriter, r *http.Request) {
    username, err := GetCurrentUsername(r)
    if err != nil { http.Error(w, "Unauthorized", http.StatusUnauthorized); return }
    isAdmin, err := IsAdminUser(username)
    if err != nil { http.Error(w, "Failed to check user permissions", http.StatusInternalServerError); return }

    environmentName := strings.TrimPrefix(r.URL.Path, "/environment/show/")
    environmentName = strings.TrimSpace(environmentName)

    query := `SELECT name, ip, server_user, server_password, install_dir, home_dir, sharedhome_dir,
                     app_dbname, app_dbuser, app_dbpass, app_dbport, app_dbhost, db_driver,
                     eazybi_dbname, eazybi_dbuser, eazybi_dbpass, eazybi_dbport, eazybi_dbhost, base_url,
                     COALESCE(db_connection_type, 'ssh'), COALESCE(db_server_user, ''), COALESCE(db_server_password, '')
              FROM environments WHERE name = ?`

    var (
        name, ip, serverUser, serverPasswordHash, installDir, homeDir, sharedHomeDir,
        dbName, dbUser, dbPass, dbPort, dbHost, dbDriver,
        eazybiDBName, eazybiDBUser, eazybiDBPass, eazybiDBPort, eazybiDBHost, baseUrl,
        dbConnType, dbServerUser, dbServerPass string
    )

    err = db.QueryRow(query, environmentName).Scan(&name, &ip, &serverUser, &serverPasswordHash, &installDir, &homeDir, &sharedHomeDir,
        &dbName, &dbUser, &dbPass, &dbPort, &dbHost, &dbDriver,
        &eazybiDBName, &eazybiDBUser, &eazybiDBPass, &eazybiDBPort, &eazybiDBHost, &baseUrl,
        &dbConnType, &dbServerUser, &dbServerPass)
    if err != nil {
        log.Printf("Failed to fetch environment details: %v", err)
        http.Error(w, "Environment not found", http.StatusNotFound)
        return
    }

    // Compute display values for DB server connection
    connTypeLozenge := "ads-lozenge-default"
    connTypeLabel := "SSH"
    if dbConnType == "winrm" {
        connTypeLozenge = "ads-lozenge-info"
        connTypeLabel = "WinRM"
    }
    dbServerUserDisplay := dbServerUser
    if dbServerUserDisplay == "" {
        dbServerUserDisplay = "—"
    }
    dbServerPassDisplay := "Not set"
    if dbServerPass != "" {
        dbServerPassDisplay = "Configured"
    }

    content := fmt.Sprintf(`
        <div class="ads-page-centered">
            <div class="ads-page-content" style="max-width: 800px;">
                <div class="ads-page-header">
                    <div class="ads-breadcrumb">
                        <a href="/">Environments</a><span class="ads-breadcrumb-separator">/</span>
                        <a href="/environment/%s">%s</a><span class="ads-breadcrumb-separator">/</span>
                        <span class="ads-breadcrumb-current">Parameters</span>
                    </div>
                    <h1>Environment Parameters</h1>
                </div>
                <div class="ads-card-flat">
                    <div class="ads-card-header"><span class="ads-card-title">Server Configuration</span></div>
                    <table class="ads-table ads-table-params">
                        <tr><th>Cluster Nodes</th><td>%s</td></tr>
                        <tr><th>Server User</th><td>%s</td></tr>
                        <tr><th>Server Password</th><td><span class="ads-lozenge ads-lozenge-default">Encrypted</span></td></tr>
                        <tr><th>Installation Directory</th><td>%s</td></tr>
                        <tr><th>Home Directory</th><td>%s</td></tr>
                        <tr><th>Shared Home Directory</th><td>%s</td></tr>
                        <tr><th>Base URL</th><td>%s</td></tr>
                    </table>
                </div>
                <div class="ads-card-flat">
                    <div class="ads-card-header"><span class="ads-card-title">Application Database</span></div>
                    <table class="ads-table ads-table-params">
                        <tr><th>DB Name</th><td>%s</td></tr>
                        <tr><th>DB User</th><td>%s</td></tr>
                        <tr><th>DB Password</th><td>%s</td></tr>
                        <tr><th>DB Port</th><td>%s</td></tr>
                        <tr><th>DB Host</th><td>%s</td></tr>
                        <tr><th>DB Driver</th><td>%s</td></tr>
                    </table>
                </div>
                <div class="ads-card-flat">
                    <div class="ads-card-header"><span class="ads-card-title">EazyBI Database</span></div>
                    <table class="ads-table ads-table-params">
                        <tr><th>DB Name</th><td>%s</td></tr>
                        <tr><th>DB User</th><td>%s</td></tr>
                        <tr><th>DB Password</th><td>%s</td></tr>
                        <tr><th>DB Port</th><td>%s</td></tr>
                        <tr><th>DB Host</th><td>%s</td></tr>
                    </table>
                </div>
                <div class="ads-card-flat">
                    <div class="ads-card-header"><span class="ads-card-title">Database Server Connection</span></div>
                    <table class="ads-table ads-table-params">
                        <tr><th>Connection Type</th><td><span class="ads-lozenge %s">%s</span></td></tr>
                        <tr><th>DB Server User</th><td>%s</td></tr>
                        <tr><th>DB Server Password</th><td><span class="ads-lozenge ads-lozenge-default">%s</span></td></tr>
                    </table>
                </div>
                <div class="ads-action-bar">
                    <a href="/environment/edit/%s" class="ads-btn ads-btn-primary">Edit parameters</a>
                    <a href="/environment/%s" class="ads-btn ads-btn-default">Back to environment</a>
                </div>
            </div>
        </div>`, name, name, ip, serverUser, installDir, homeDir, sharedHomeDir, baseUrl,
        dbName, dbUser, dbPass, dbPort, dbHost, dbDriver,
        eazybiDBName, eazybiDBUser, eazybiDBPass, eazybiDBPort, eazybiDBHost,
        connTypeLozenge, connTypeLabel, dbServerUserDisplay, dbServerPassDisplay,
        name, name)

    RenderPage(w, PageData{Title: "Parameters", IsAdmin: isAdmin, Content: template.HTML(content)})
}
