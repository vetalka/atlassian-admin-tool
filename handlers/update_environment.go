package handlers

import (
    "fmt"
    "html/template"
    "log"
    "net/http"
    "strings"
)

func HandleUpdateEnvironmentForm(w http.ResponseWriter, r *http.Request) {
    username, err := GetCurrentUsername(r)
    if err != nil { http.Error(w, "Unauthorized", http.StatusUnauthorized); return }
    isAdmin, err := IsAdminUser(username)
    if err != nil { http.Error(w, "Failed to check user permissions", http.StatusInternalServerError); return }

    environmentName := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/environment/edit/"))

    query := `SELECT name, ip, server_user, server_password, install_dir, home_dir, sharedhome_dir,
                     app_dbname, app_dbuser, app_dbpass, app_dbport, app_dbhost, db_driver,
                     eazybi_dbname, eazybi_dbuser, eazybi_dbpass, eazybi_dbport, eazybi_dbhost, base_url,
                     COALESCE(db_connection_type, 'ssh'), COALESCE(db_server_user, ''), COALESCE(db_server_password, '')
              FROM environments WHERE name = ?`
    var name, ip, serverUser, serverPassword, installDir, homeDir, sharedHomeDir, dbName, dbUser, dbPass, dbPort, dbHost, dbDriver, eazybiDBName, eazybiDBUser, eazybiDBPass, eazybiDBPort, eazybiDBHost, baseUrl, dbConnType, dbServerUser, dbServerPass string
    err = db.QueryRow(query, environmentName).Scan(&name, &ip, &serverUser, &serverPassword, &installDir, &homeDir, &sharedHomeDir, &dbName, &dbUser, &dbPass, &dbPort, &dbHost, &dbDriver, &eazybiDBName, &eazybiDBUser, &eazybiDBPass, &eazybiDBPort, &eazybiDBHost, &baseUrl, &dbConnType, &dbServerUser, &dbServerPass)
    if err != nil { log.Printf("Failed to fetch env: %v", err); http.Error(w, "Environment not found", http.StatusNotFound); return }

    sshSelected := ""; winrmSelected := ""
    if dbConnType == "winrm" { winrmSelected = "selected" } else { sshSelected = "selected" }

    extraHead := template.HTML(`<script>
        function toggleDBFields() {
            var ct = document.getElementById('dbConnType').value;
            document.getElementById('db-server-edit-fields').style.display = ct === 'winrm' ? 'block' : 'none';
        }
        document.addEventListener('DOMContentLoaded', toggleDBFields);
    </script>`)

    content := fmt.Sprintf(`
        <div class="ads-page-centered">
            <div class="ads-page-content" style="max-width: 800px;">
                <div class="ads-page-header">
                    <div class="ads-breadcrumb">
                        <a href="/">Environments</a><span class="ads-breadcrumb-separator">/</span>
                        <a href="/environment/%s">%s</a><span class="ads-breadcrumb-separator">/</span>
                        <span class="ads-breadcrumb-current">Edit</span>
                    </div>
                    <h1>Edit Environment</h1>
                </div>
                <form method="POST" action="/environment/update">
                    <input type="hidden" name="environmentName" value="%s">
                    <div class="ads-card-flat">
                        <div class="ads-card-header"><span class="ads-card-title">Server Configuration</span></div>
                        <table class="ads-table ads-table-params">
                            <tr><th>Cluster Nodes</th><td><input class="ads-input" type="text" name="ip" value="%s"></td></tr>
                            <tr><th>Server User</th><td><input class="ads-input" type="text" name="serverUser" value="%s"></td></tr>
                            <tr><th>Server Password</th><td><input class="ads-input" type="password" name="serverPassword" placeholder="Leave empty to keep current"></td></tr>
                            <tr><th>Installation Directory</th><td><input class="ads-input" type="text" name="installDir" value="%s"></td></tr>
                            <tr><th>Home Directory</th><td><input class="ads-input" type="text" name="homeDir" value="%s"></td></tr>
                            <tr><th>Shared Home Directory</th><td><input class="ads-input" type="text" name="sharedHomeDir" value="%s"></td></tr>
                            <tr><th>Base URL</th><td><input class="ads-input" type="text" name="baseUrl" value="%s"></td></tr>
                        </table>
                    </div>
                    <div class="ads-card-flat">
                        <div class="ads-card-header"><span class="ads-card-title">Application Database</span></div>
                        <table class="ads-table ads-table-params">
                            <tr><th>DB Name</th><td><input class="ads-input" type="text" name="dbName" value="%s"></td></tr>
                            <tr><th>DB User</th><td><input class="ads-input" type="text" name="dbUser" value="%s"></td></tr>
                            <tr><th>DB Password</th><td><input class="ads-input" type="text" name="dbPass" value="%s"></td></tr>
                            <tr><th>DB Port</th><td><input class="ads-input" type="text" name="dbPort" value="%s"></td></tr>
                            <tr><th>DB Host</th><td><input class="ads-input" type="text" name="dbHost" value="%s"></td></tr>
                            <tr><th>DB Driver</th><td><input class="ads-input" type="text" name="dbDriver" value="%s"></td></tr>
                        </table>
                    </div>
                    <div class="ads-card-flat">
                        <div class="ads-card-header"><span class="ads-card-title">EazyBI Database</span></div>
                        <table class="ads-table ads-table-params">
                            <tr><th>DB Name</th><td><input class="ads-input" type="text" name="eazybiDBName" value="%s"></td></tr>
                            <tr><th>DB User</th><td><input class="ads-input" type="text" name="eazybiDBUser" value="%s"></td></tr>
                            <tr><th>DB Password</th><td><input class="ads-input" type="text" name="eazybiDBPass" value="%s"></td></tr>
                            <tr><th>DB Port</th><td><input class="ads-input" type="text" name="eazybiDBPort" value="%s"></td></tr>
                            <tr><th>DB Host</th><td><input class="ads-input" type="text" name="eazybiDBHost" value="%s"></td></tr>
                        </table>
                    </div>
                    <div class="ads-card-flat">
                        <div class="ads-card-header"><span class="ads-card-title">Database Server Connection</span></div>
                        <table class="ads-table ads-table-params">
                            <tr><th>Connection Type</th><td>
                                <select class="ads-input ads-select" name="dbConnType" id="dbConnType" onchange="toggleDBFields()">
                                    <option value="ssh" %s>SSH (Linux / same server)</option>
                                    <option value="winrm" %s>WinRM (Windows SQL Server)</option>
                                </select>
                            </td></tr>
                        </table>
                        <div id="db-server-edit-fields">
                            <table class="ads-table ads-table-params">
                                <tr><th>DB Server User</th><td><input class="ads-input" type="text" name="dbServerUser" value="%s" placeholder="DOMAIN\user or administrator"></td></tr>
                                <tr><th>DB Server Password</th><td><input class="ads-input" type="password" name="dbServerPassword" placeholder="Leave empty to keep current"></td></tr>
                            </table>
                        </div>
                    </div>
                    <div class="ads-action-bar">
                        <button type="submit" class="ads-btn ads-btn-primary">Save changes</button>
                        <a href="/environment/show/%s" class="ads-btn ads-btn-default">Cancel</a>
                    </div>
                </form>
            </div>
        </div>`, name, name, name, ip, serverUser, installDir, homeDir, sharedHomeDir, baseUrl,
        dbName, dbUser, dbPass, dbPort, dbHost, dbDriver,
        eazybiDBName, eazybiDBUser, eazybiDBPass, eazybiDBPort, eazybiDBHost,
        sshSelected, winrmSelected, dbServerUser,
        name)

    RenderPage(w, PageData{Title: "Edit Environment", IsAdmin: isAdmin, ExtraHead: extraHead, Content: template.HTML(content)})
}

func HandleUpdateEnvironment(w http.ResponseWriter, r *http.Request) {
    environmentName := strings.TrimSpace(r.FormValue("environmentName"))
    if environmentName == "" { http.Error(w, "Environment name is missing", http.StatusBadRequest); return }

    var existingName string
    if err := db.QueryRow("SELECT name FROM environments WHERE name = ?", environmentName).Scan(&existingName); err != nil {
        http.Error(w, "Environment not found", http.StatusNotFound); return
    }

    _, err := db.Exec(`UPDATE environments SET ip=?, server_user=?, server_password=?, install_dir=?, home_dir=?, sharedhome_dir=?, app_dbname=?, app_dbuser=?, app_dbpass=?, app_dbport=?, app_dbhost=?, db_driver=?, eazybi_dbname=?, eazybi_dbuser=?, eazybi_dbpass=?, eazybi_dbport=?, eazybi_dbhost=?, base_url=?, db_connection_type=?, db_server_user=? WHERE name=?`,
        r.FormValue("ip"), r.FormValue("serverUser"), r.FormValue("serverPassword"),
        r.FormValue("installDir"), r.FormValue("homeDir"), r.FormValue("sharedHomeDir"),
        r.FormValue("dbName"), r.FormValue("dbUser"), r.FormValue("dbPass"),
        r.FormValue("dbPort"), r.FormValue("dbHost"), r.FormValue("dbDriver"),
        r.FormValue("eazybiDBName"), r.FormValue("eazybiDBUser"), r.FormValue("eazybiDBPass"),
        r.FormValue("eazybiDBPort"), r.FormValue("eazybiDBHost"), r.FormValue("baseUrl"),
        r.FormValue("dbConnType"), r.FormValue("dbServerUser"),
        environmentName)

    // Update DB server password only if provided (non-empty)
    if dbServerPass := r.FormValue("dbServerPassword"); dbServerPass != "" {
        db.Exec(`UPDATE environments SET db_server_password=? WHERE name=?`, dbServerPass, environmentName)
    }
    if err != nil { log.Printf("Failed to update: %v", err); http.Error(w, "Failed to update", http.StatusInternalServerError); return }
    http.Redirect(w, r, "/environment/show/"+environmentName, http.StatusSeeOther)
}
