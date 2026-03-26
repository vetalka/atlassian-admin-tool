package handlers

import (
    "context"
    "fmt"
    "html"
    "html/template"
    "log"
    "net/http"
//    "net/url"
    "os"
    "strings"
    "path/filepath"
    "time"
    "database/sql" 
    _ "github.com/lib/pq" // PostgreSQL driver
    _ "github.com/go-sql-driver/mysql" // MySQL driver
    _ "github.com/microsoft/go-mssqldb" // SQL Server driver
)

var restoreCancelFuncs = make(map[string]context.CancelFunc)
var restoreStatus = map[string]struct {
    Progress int    // Percentage of completion
    Message  string // Status message
}{}

func CheckAndRedirectProgressRestore(w http.ResponseWriter, r *http.Request, workingEnvironment string) bool {
    // Check if a restore process is already running for this environment
    status, exists := restoreStatus[workingEnvironment]
    if exists && status.Progress > 0 && status.Progress < 100 {
        // Log the redirection event for debugging
        log.Printf("Redirecting to restore progress page for environment: %s", workingEnvironment)
        // Redirect to the progress page if a restore is already in progress
        http.Redirect(w, r, fmt.Sprintf("/environment/restore-progress/%s", workingEnvironment), http.StatusSeeOther)
        return true
    }
    return false
}

// Function to extract the app based on the environment
func getAppForEnvironment(environment string) (string, error) {
    var app string
    err := db.QueryRow("SELECT app FROM environments WHERE name = ?", environment).Scan(&app)
    if err != nil {
        return "", fmt.Errorf("failed to retrieve app for environment %s: %v", environment, err)
    }
    return app, nil
}

// HandleRestorePage displays the restore page with the current environment and allows the user to select a restore environment.
func HandleRestorePage(w http.ResponseWriter, r *http.Request) {
    // Get the working environment from the URL (this is the environment where the restore will be performed)
    workingEnvironment := extractEnvironmentNameWork(r.URL.Path)
    log.Printf("Working environment extracted from URL: %s", workingEnvironment)

    // Check and redirect if there is a restore in progress
    if CheckAndRedirectProgressRestore(w, r, workingEnvironment) {
        return
    }

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

    // Render the header
    RenderHeader(w, isAdmin)

    // Fetch the app for the working environment
    app, err := getAppForEnvironment(workingEnvironment)
    if err != nil {
        log.Printf("Failed to retrieve app for environment %s: %v", workingEnvironment, err)
        http.Error(w, "Failed to retrieve app for the environment", http.StatusInternalServerError)
        return
    }

    // Fetch the available environments with the same `db_driver`
    environments, err := getEnvironmentsByDBType(workingEnvironment, app)
    if err != nil {
        log.Printf("Failed to retrieve environments: %v", err)
        http.Error(w, "Failed to load environments", http.StatusInternalServerError)
        return
    }

    // Check if eazyBI exists for this environment
    var eazybiExists bool
    query := "SELECT CASE WHEN eazybi_dbname IS NOT NULL AND eazybi_dbname <> '' THEN 1 ELSE 0 END FROM environments WHERE name = ?"
    err = db.QueryRow(query, workingEnvironment).Scan(&eazybiExists)
    if err != nil && err != sql.ErrNoRows {
        log.Printf("Failed to check eazyBI existence for environment %s: %v", workingEnvironment, err)
        http.Error(w, "Failed to load restore options. Check logs for details.", http.StatusInternalServerError)
        return
    }

    // Check if NFS (shared home) is available for the environment
    var sharedHomeDir string
    query = "SELECT sharedhome_dir FROM environments WHERE name = ?"
    err = db.QueryRow(query, workingEnvironment).Scan(&sharedHomeDir)
    if err != nil && err != sql.ErrNoRows {
        log.Printf("Failed to check NFS (shared home) existence for environment %s: %v", workingEnvironment, err)
        http.Error(w, "Failed to load restore options. Check logs for details.", http.StatusInternalServerError)
        return
    }

    // Get selected backup date and time from form values
    selectedDate := r.FormValue("backupDate")
    selectedTime := r.FormValue("backupTime")

    // Render the page with working environment, filtered environments, and eazyBI section
    renderRestorePage(w, workingEnvironment, environments, eazybiExists, sharedHomeDir, selectedDate != "" && selectedTime != "", isAdmin)
}


// Function to extract environment name from URL (if provided)
func extractEnvironmentName1(urlPath string) string {
    pathSegments := strings.Split(urlPath, "/")
    if len(pathSegments) >= 4 {
        return pathSegments[3]
    }
    return ""
}

// Function to extract environment name from URL (if provided)
func extractEnvironmentNameWork(urlPath string) string {
    pathSegments := strings.Split(urlPath, "/")
    if len(pathSegments) >= 4 {
        return pathSegments[3]
    }
    return ""
}

// Retrieve environments from the database
func getEnvironments() ([]string, error) {
    var environments []string
    rows, err := db.Query("SELECT name FROM environments WHERE app = 'Jira'")
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    for rows.Next() {
        var env string
        if err := rows.Scan(&env); err != nil {
            return nil, err
        }
        environments = append(environments, env)
    }
    return environments, nil
}

// buildEnvironmentOptions generates HTML <option> elements for the environments dropdown
func buildEnvironmentOptions(environments []string) string {
    var environmentOptions strings.Builder
    for _, env := range environments {
        // Each environment is added as an option in the dropdown
        environmentOptions.WriteString(fmt.Sprintf(`<option value="%s">%s</option>`, html.EscapeString(env), html.EscapeString(env)))
    }
    return environmentOptions.String()
}

// Retrieve environments from the database based on the same db_driver as the current environment
func getEnvironmentsByDBType(currentEnvironment, app string) ([]string, error) {
    var currentDBType string
    // Fetch the db_driver of the current environment
    err := db.QueryRow("SELECT db_driver FROM environments WHERE name = ?", currentEnvironment).Scan(&currentDBType)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve db_driver for environment %s: %v", currentEnvironment, err)
    }

    var environments []string
    // Fetch environments with the same db_driver and app
    query := "SELECT name FROM environments WHERE db_driver = ? AND app = ?"
    rows, err := db.Query(query, currentDBType, app)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve environments by db_driver: %v", err)
    }
    defer rows.Close()

    for rows.Next() {
        var env string
        if err := rows.Scan(&env); err != nil {
            return nil, fmt.Errorf("failed to scan environment: %v", err)
        }
        environments = append(environments, env)
    }
    return environments, nil
}

// Render the restore page with the current environment and a dropdown of environments
func renderRestorePage(w http.ResponseWriter, currentEnvironment string, environments []string, eazybiExists bool, sharedHomeDir string, dateTimeSelected bool, isAdmin bool) {
    sanitizedEnv := html.EscapeString(currentEnvironment)

    envOptions := ""
    for _, env := range environments {
        envOptions += fmt.Sprintf(`<option value="%s">%s</option>`, html.EscapeString(env), html.EscapeString(env))
    }

    eazybiOptionHTML := ""
    if eazybiExists {
        eazybiOptionHTML = `
                    <label style="display:flex; align-items:center; gap:14px; padding:16px 20px; background:rgba(0,82,204,0.04); border:2px solid #6554C0; border-radius:8px; cursor:pointer; transition:all 0.15s;" class="bc">
                        <input type="checkbox" name="restoreEazyBICheckbox" value="Yes" checked style="width:18px; height:18px; accent-color:#6554C0;"
                               onchange="var l=this.closest('.bc');l.style.borderColor=this.checked?'#6554C0':'var(--color-border)';l.style.background=this.checked?'rgba(0,82,204,0.04)':'var(--color-bg-card)';document.getElementById('restoreEazyBI').value=this.checked?'Yes':'No'">
                        <input type="hidden" id="restoreEazyBI" name="restoreEazyBI" value="Yes">
                        <div style="width:40px; height:40px; background:linear-gradient(135deg, #6554C0, #6554C0cc); border-radius:10px; display:flex; align-items:center; justify-content:center; flex-shrink:0;">
                            <span style="font-size:20px; color:white;">&#x1F4CA;</span>
                        </div>
                        <div style="flex:1;">
                            <div style="font-weight:600; font-size:14px;">eazyBI Database</div>
                            <div style="font-size:12px; color:var(--color-text-subtle); margin-top:2px;">eazyBI analytics database</div>
                        </div>
                    </label>`
    }

    nfsOptionHTML := ""
    if sharedHomeDir != "" {
        nfsOptionHTML = `
                    <label style="display:flex; align-items:center; gap:14px; padding:16px 20px; background:rgba(0,82,204,0.04); border:2px solid #FF991F; border-radius:8px; cursor:pointer; transition:all 0.15s;" class="bc">
                        <input type="checkbox" name="restoreNFSCheckbox" value="Yes" checked style="width:18px; height:18px; accent-color:#FF991F;"
                               onchange="var l=this.closest('.bc');l.style.borderColor=this.checked?'#FF991F':'var(--color-border)';l.style.background=this.checked?'rgba(0,82,204,0.04)':'var(--color-bg-card)';document.getElementById('restoreNFS').value=this.checked?'Yes':'No'">
                        <input type="hidden" id="restoreNFS" name="restoreNFS" value="Yes">
                        <div style="width:40px; height:40px; background:linear-gradient(135deg, #FF991F, #FF991Fcc); border-radius:10px; display:flex; align-items:center; justify-content:center; flex-shrink:0;">
                            <span style="font-size:20px; color:white;">&#x1F4C1;</span>
                        </div>
                        <div style="flex:1;">
                            <div style="font-weight:600; font-size:14px;">NFS / Shared Home</div>
                            <div style="font-size:12px; color:var(--color-text-subtle); margin-top:2px;">Shared filesystem data</div>
                        </div>
                    </label>`
    }

    extraHead := template.HTML(fmt.Sprintf(`<script>
        document.addEventListener('DOMContentLoaded', function() {
            const restoreForm = document.getElementById('restoreForm');
            const workingEnvironment = '%s';
            restoreForm.addEventListener('submit', function(e) {
                this.action = '/environment/start-restore/' + encodeURIComponent(workingEnvironment);
            });
        });
        let selectedEnvironment = '';
        function loadBackupDates() {
            selectedEnvironment = document.getElementById("restoreEnvironment").value;
            const backupDateSelect = document.getElementById("backupDate");
            if (!selectedEnvironment) { backupDateSelect.innerHTML = '<option value="">Select environment first</option>'; backupDateSelect.disabled = true; return; }
            backupDateSelect.innerHTML = '<option value="">Loading...</option>'; backupDateSelect.disabled = true;
            fetch('/environment/restore/get-backup-dates?environment=' + encodeURIComponent(selectedEnvironment))
                .then(r => r.text()).then(data => { backupDateSelect.innerHTML = data; backupDateSelect.disabled = false;
                    if (backupDateSelect.options.length === 1) { backupDateSelect.selectedIndex < 1; loadBackupTimes(); }
                }).catch(() => { backupDateSelect.innerHTML = '<option value="">Error loading dates</option>'; backupDateSelect.disabled = true; });
        }
        function loadBackupTimes() {
            const date = document.getElementById("backupDate").value;
            const timeSelect = document.getElementById("backupTime");
            if (!selectedEnvironment || !date) { timeSelect.innerHTML = '<option value="">Select date first</option>'; timeSelect.disabled = true; return; }
            timeSelect.innerHTML = '<option value="">Loading...</option>'; timeSelect.disabled = true;
            fetch('/environment/restore/get-backup-times?environment=' + encodeURIComponent(selectedEnvironment) + '&date=' + encodeURIComponent(date))
                .then(r => r.text()).then(data => { timeSelect.innerHTML = data;
                    const opts = timeSelect.getElementsByTagName('option'); if (opts.length === 1) timeSelect.selectedIndex = 0; timeSelect.disabled = opts.length < 1;
                }).catch(() => { timeSelect.innerHTML = '<option value="">Error loading times</option>'; timeSelect.disabled = true; });
        }
    </script>`, sanitizedEnv))

    content := fmt.Sprintf(`
        <div class="ads-page-centered"><div class="ads-page-content">
        <div class="ads-breadcrumbs">
            <a href="/">Environments</a> &rarr;
            <a href="/environment/%s">%s</a> &rarr; Restore
        </div>

        <div class="ads-card-flat" style="margin-top:16px;">
            <div class="ads-card-header">
                <div style="width:48px; height:48px; background:linear-gradient(135deg, #0747A6, #0065FF); border-radius:12px; display:flex; align-items:center; justify-content:center;">
                    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2"><polyline points="1 4 1 10 7 10"></polyline><path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10"></path></svg>
                </div>
                <div style="flex:1;">
                    <span class="ads-card-title" style="font-size:18px;">Restore Options</span>
                    <div style="margin-top:4px;">
                        <span style="font-size:13px; color:var(--color-text-subtle);">Target environment: <strong>%s</strong></span>
                    </div>
                </div>
            </div>
            <form id="restoreForm" action="/environment/start-restore/" method="POST" style="padding:0 24px 24px;">

                <div style="font-size:14px; font-weight:600; margin-bottom:12px;">Source Backup</div>
                <div style="background:var(--color-bg); border:1px solid var(--color-border); border-radius:8px; padding:16px; margin-bottom:20px;">
                    <div style="margin-bottom:14px;">
                        <label style="font-weight:600; font-size:13px; color:var(--color-text-subtle); display:block; margin-bottom:4px;">Environment to restore from</label>
                        <select class="ads-input" id="restoreEnvironment" name="restoreEnvironment" required onchange="loadBackupDates()" style="width:100%%;">
                            <option value="">Select Environment</option>%s
                        </select>
                    </div>
                    <div style="display:grid; grid-template-columns:1fr 1fr; gap:16px;">
                        <div>
                            <label style="font-weight:600; font-size:13px; color:var(--color-text-subtle); display:block; margin-bottom:4px;">Backup Date</label>
                            <select class="ads-input" id="backupDate" name="backupDate" required disabled onchange="loadBackupTimes()" style="width:100%%;">
                                <option value="">Select environment first</option>
                            </select>
                        </div>
                        <div>
                            <label style="font-weight:600; font-size:13px; color:var(--color-text-subtle); display:block; margin-bottom:4px;">Backup Time</label>
                            <select class="ads-input" id="backupTime" name="backupTime" required disabled style="width:100%%;">
                                <option value="">Select date first</option>
                            </select>
                        </div>
                    </div>
                </div>

                <div style="font-size:14px; font-weight:600; margin-bottom:12px;">Restore Components</div>
                <div style="display:grid; grid-template-columns:1fr 1fr; gap:12px; margin-bottom:20px;">
                    <label style="display:flex; align-items:center; gap:14px; padding:16px 20px; background:rgba(0,82,204,0.04); border:2px solid #0052CC; border-radius:8px; cursor:pointer; transition:all 0.15s;" class="bc">
                        <input type="checkbox" name="restoreAttachmentsCheckbox" value="Yes" checked style="width:18px; height:18px; accent-color:#0052CC;"
                               onchange="var l=this.closest('.bc');l.style.borderColor=this.checked?'#0052CC':'var(--color-border)';l.style.background=this.checked?'rgba(0,82,204,0.04)':'var(--color-bg-card)';document.getElementById('restoreAttachments').value=this.checked?'Yes':'No'">
                        <input type="hidden" id="restoreAttachments" name="restoreAttachments" value="Yes">
                        <div style="width:40px; height:40px; background:linear-gradient(135deg, #0052CC, #0052CCcc); border-radius:10px; display:flex; align-items:center; justify-content:center; flex-shrink:0;">
                            <span style="font-size:20px; color:white;">&#x1F4CE;</span>
                        </div>
                        <div style="flex:1;">
                            <div style="font-weight:600; font-size:14px;">Attachments</div>
                            <div style="font-size:12px; color:var(--color-text-subtle); margin-top:2px;">Application data files and uploads</div>
                        </div>
                    </label>

                    <label style="display:flex; align-items:center; gap:14px; padding:16px 20px; background:rgba(0,82,204,0.04); border:2px solid #00875A; border-radius:8px; cursor:pointer; transition:all 0.15s;" class="bc">
                        <input type="checkbox" name="restoreDatabaseCheckbox" value="Yes" checked style="width:18px; height:18px; accent-color:#00875A;"
                               onchange="var l=this.closest('.bc');l.style.borderColor=this.checked?'#00875A':'var(--color-border)';l.style.background=this.checked?'rgba(0,82,204,0.04)':'var(--color-bg-card)';document.getElementById('restoreDatabase').value=this.checked?'Yes':'No'">
                        <input type="hidden" id="restoreDatabase" name="restoreDatabase" value="Yes">
                        <div style="width:40px; height:40px; background:linear-gradient(135deg, #00875A, #00875Acc); border-radius:10px; display:flex; align-items:center; justify-content:center; flex-shrink:0;">
                            <span style="font-size:20px; color:white;">&#x1F5C4;</span>
                        </div>
                        <div style="flex:1;">
                            <div style="font-weight:600; font-size:14px;">Database</div>
                            <div style="font-size:12px; color:var(--color-text-subtle); margin-top:2px;">Full database restore</div>
                        </div>
                    </label>
                    %s %s
                </div>

                <div style="padding-top:16px; border-top:1px solid var(--color-border); display:flex; gap:8px; align-items:center;">
                    <button type="submit" class="ads-button ads-button-primary" style="padding:10px 24px; font-size:14px;">
                        &#x25B6; Start Restore
                    </button>
                    <a href="/environment/%s" class="ads-button ads-button-default">&larr; Cancel</a>
                </div>
            </form>
        </div>
        </div></div>
    `, sanitizedEnv, sanitizedEnv, sanitizedEnv, envOptions, eazybiOptionHTML, nfsOptionHTML, sanitizedEnv)

    RenderPage(w, PageData{
        Title:     "Restore - " + currentEnvironment,
        IsAdmin:   isAdmin,
        ExtraHead: extraHead,
        Content:   template.HTML(content),
    })
}


// checkForEazyBISQL checks if the eazyBI SQL file exists for the given environment and backup
func checkForEazyBISQL(baseDir, appName, environmentName, backupDate, backupTime string) bool {
    // Expected eazyBI file name, based on the environment
    eazybiFileName := fmt.Sprintf("%s.sql", environmentName)
    eazybiFilePath := filepath.Join(baseDir, appName, environmentName, backupDate, backupTime, eazybiFileName)

    log.Printf("Checking for eazyBI file: %s", eazybiFilePath)  // Add debug log

    // Check if the file exists
    _, err := os.Stat(eazybiFilePath)
    if err != nil {
        if os.IsNotExist(err) {
            log.Printf("eazyBI SQL file not found: %s", eazybiFilePath)
            return false
        }
        log.Printf("Error checking for eazyBI SQL file: %v", err)
        return false
    }
    log.Printf("eazyBI SQL file exists: %s", eazybiFilePath)
    return true
}

func getBackupBaseDir(app string) string {
    switch app {
    case "Jira":
        return "/adminToolBackupDirectory/Jira/"
    case "Confluence":
        return "/adminToolBackupDirectory/Confluence/"
    default:
        return "/adminToolBackupDirectory/Other/"
    }
}

// HandleGetBackupDates retrieves the available backup dates for a selected environment
func HandleGetBackupDates(w http.ResponseWriter, r *http.Request) {
    environmentName := r.URL.Query().Get("environment")
    if environmentName == "" {
        http.Error(w, "Environment name is required", http.StatusBadRequest)
        return
    }

    // Fetch the app for the environment to determine the base directory
    app, err := getAppForEnvironment(environmentName)
    if err != nil {
        log.Printf("Failed to retrieve app for environment %s: %v", environmentName, err)
        http.Error(w, "Failed to retrieve app for the environment", http.StatusInternalServerError)
        return
    }

    // Use the correct base directory based on the app
    baseDir := getBackupBaseDir(app)
    backupDir := filepath.Join(baseDir, environmentName)

    log.Printf("Constructed backup directory path: %s", backupDir)


    log.Printf("Constructed backup directory path: %s", backupDir)

    // Read the directory to get the available backup dates (folder names)
    dates, err := os.ReadDir(backupDir)
    if err != nil {
        log.Printf("Failed to read backup directory: %v", err)
        http.Error(w, "Failed to load backup dates", http.StatusInternalServerError)
        return
    }

    var dateOptions strings.Builder
    for _, date := range dates {
        if date.IsDir() {
            fmt.Fprintf(&dateOptions, `<option value="%s">%s</option>`, date.Name(), date.Name())
        }
    }

    if dateOptions.Len() == 0 {
        dateOptions.WriteString(`<option value="">No backup dates available</option>`)
    }

    // Return the generated options to the client
    fmt.Fprintln(w, dateOptions.String())
}

// HandleGetBackupTimes retrieves the available backup times for a selected environment and date
func HandleGetBackupTimes(w http.ResponseWriter, r *http.Request) {
    environmentName := r.URL.Query().Get("environment")
    date := r.URL.Query().Get("date")
    if environmentName == "" || date == "" {
        http.Error(w, "Environment and date are required", http.StatusBadRequest)
        return
    }

    // Fetch the app for the environment to determine the base directory
    app, err := getAppForEnvironment(environmentName)
    if err != nil {
        log.Printf("Failed to retrieve app for environment %s: %v", environmentName, err)
        http.Error(w, "Failed to retrieve app for the environment", http.StatusInternalServerError)
        return
    }

    // Use the correct base directory based on the app
    baseDir := getBackupBaseDir(app)
    backupDir := filepath.Join(baseDir, environmentName, date)

    log.Printf("Constructed backup times directory path: %s", backupDir)

    // Read the directory to get the available backup times (folder names)
    times, err := os.ReadDir(backupDir)
    if err != nil {
        log.Printf("Failed to read backup times directory: %v", err)
        http.Error(w, "Failed to load backup times", http.StatusInternalServerError)
        return
    }

    var timeOptions strings.Builder
    for _, t := range times {
        if t.IsDir() {
            fmt.Fprintf(&timeOptions, `<option value="%s">%s</option>`, t.Name(), t.Name())
        }
    }

    if timeOptions.Len() == 0 {
        timeOptions.WriteString(`<option value="">No backup times available</option>`)
    }

    // Return the generated options to the client
    fmt.Fprintln(w, timeOptions.String())
}

// HandleStartRestore handles the form submission and starts the restore process
func HandleStartRestore(w http.ResponseWriter, r *http.Request) {

    // Extract the chosen environment from the form (this is where backups will be stored)
    chosenEnvironment := r.FormValue("restoreEnvironment")
    log.Printf("Extracted chosen environment name (for stored backups): %s", chosenEnvironment)

    // Extract the working environment from the URL or session data
    workingEnvironment := extractEnvironmentNameWork(r.URL.Path)
    log.Printf("Using working environment for restore: %s", workingEnvironment)

    // Check and redirect if there is a restore in progress
    if CheckAndRedirectProgressRestore(w, r, workingEnvironment) {
        return
    }

    // If the working environment is empty, return an error
    if workingEnvironment == "" {
        log.Printf("Working environment is empty. Please check the configuration.")
        http.Error(w, "Invalid working environment", http.StatusBadRequest)
        return
    }

    // Fetch environment details for the working environment
    var app, name, dbHost, dbUser, dbPass, dbName, dbPort, dbDriver, eazybiDbName, eazybiDbUser, eazybiDbPass, eazybiDbPort, eazybiDbHost, sharedHomeDir, homeDir, installDir, serverUser, serverIPs, serverPassword, baseUrl string
    query := `
        SELECT app, name, app_dbhost, app_dbuser, app_dbpass, app_dbname, app_dbport, db_driver, 
               eazybi_dbname, eazybi_dbuser, eazybi_dbpass, eazybi_dbport, eazybi_dbhost, sharedhome_dir, 
               home_dir, install_dir, server_user, ip, server_password, base_url 
        FROM environments WHERE name = ?`

    log.Printf("Running query to fetch environment details for working environment: %s", workingEnvironment)
    err := db.QueryRow(query, workingEnvironment).Scan(&app, &name, &dbHost, &dbUser, &dbPass, &dbName, &dbPort, &dbDriver, &eazybiDbName, &eazybiDbUser, &eazybiDbPass, &eazybiDbPort, &eazybiDbHost, &sharedHomeDir, &homeDir, &installDir, &serverUser, &serverIPs, &serverPassword, &baseUrl)

    if err != nil {
        log.Printf("Failed to retrieve environment details for %s: %v", workingEnvironment, err)
        http.Error(w, "Failed to retrieve environment details. Check logs for details.", http.StatusInternalServerError)
        return
    }

    // Get form values from the submitted restore form
    backupDate := r.FormValue("backupDate")
    backupTime := r.FormValue("backupTime")
    restoreAttachments := r.FormValue("restoreAttachments") == "Yes"
    restoreDatabase := r.FormValue("restoreDatabase") == "Yes"
    restoreEazyBI := r.FormValue("restoreEazyBI") == "Yes"
    nfsRestoreOption := r.FormValue("restoreNFS") == "Yes"

    // Validate that both backupDate and backupTime are provided
    if backupDate == "" || backupTime == "" {
        log.Printf("Backup date or time is missing for environment %s", chosenEnvironment)
        http.Error(w, "Please select a valid backup date and time", http.StatusBadRequest)
        return
    }

    // Construct paths for restore operation using chosen environment for backup files but working environment for the restore operation
    baseRestoreFolder := "/adminToolBackupDirectory"
    tempRestoreFolder := filepath.Join(baseRestoreFolder, app, chosenEnvironment, backupDate, backupTime)
    remoteTempFolder := filepath.Join(baseRestoreFolder, app, workingEnvironment, "DB_TMP")
    dataFolder := filepath.Join(baseRestoreFolder, app, chosenEnvironment)
    currentTime := time.Now().Format("20060102_150405")
    dbTmp := fmt.Sprintf("%s_%s_tmp", dbName, currentTime)
    dbTmp2 := fmt.Sprintf("%s_%s", dbName, currentTime)
    dbTmpEazybi := fmt.Sprintf("%s_%s", eazybiDbName, currentTime)
    backupDirHome := fmt.Sprintf("%s.%s", homeDir, currentTime)
    dataCurrentBackup := fmt.Sprintf("%s/Data_Backup_%s", sharedHomeDir, currentTime)
    dataRestoreFile := fmt.Sprintf("%s/Attachments.tar.gz", dataFolder)
    nfsRestoreFile := fmt.Sprintf("%s/NFS.tar.gz", dataFolder)

    var eazybiDbFilePath string

    // Based on the database type, detect and update the eazyBI file name
    switch dbDriver {
    case "org.postgresql.Driver":
        eazybiDbFilePath = filepath.Join(tempRestoreFolder, fmt.Sprintf("%s.sql", eazybiDbName))
    case "mysql":
        log.Println("MySQL restoration is not yet implemented.")
        http.Error(w, "Unsupported database type: MySQL", http.StatusNotImplemented)
        return
    case "com.microsoft.sqlserver.jdbc.SQLServerDriver":
        eazybiDbFilePath = filepath.Join(tempRestoreFolder, fmt.Sprintf("%s.bak", eazybiDbName))
    case "oracle":
        log.Println("Oracle restoration is not yet implemented.")
        http.Error(w, "Unsupported database type: Oracle", http.StatusNotImplemented)
        return
    default:
        log.Printf("Unsupported database type: %s", dbDriver)
        http.Error(w, fmt.Sprintf("Unsupported database type: %s", dbDriver), http.StatusBadRequest)
        return
    }

    log.Printf("Vitaly Test eazybi %s: %v", eazybiDbFilePath, err)


    ctx, cancel := context.WithCancel(context.Background())
    restoreCancelFuncs[workingEnvironment] = cancel

    // Start the restore process asynchronously
    go func() {
        defer delete(restoreCancelFuncs, workingEnvironment)

        // Perform the restore tasks and track progress
        err := performRestoreTasks(ctx, workingEnvironment, app, dbHost, dbUser, dbPass, dbName, dbPort, dbDriver,
            eazybiDbHost, eazybiDbName, eazybiDbUser, eazybiDbPass, eazybiDbPort, sharedHomeDir, homeDir, installDir, serverUser,
            serverIPs, serverPassword, baseUrl, dbTmp, dbTmp2, dbTmpEazybi, eazybiDbFilePath, remoteTempFolder, tempRestoreFolder,
            backupDirHome, dataCurrentBackup, dataRestoreFile, nfsRestoreFile, restoreAttachments, restoreDatabase, dataFolder,
            restoreEazyBI, nfsRestoreOption) // Ensure all required parameters are passed correctly.
        if err != nil {
            log.Printf("Restore failed for %s: %v", workingEnvironment, err)
            UpdateRestoreStatus(workingEnvironment, 0, "Restore failed: "+err.Error(), true)
        }
    }()

    // Redirect the user to the restore progress page
    http.Redirect(w, r, fmt.Sprintf("/environment/restore-progress/%s", workingEnvironment), http.StatusSeeOther)
}

func performRestoreTasks(ctx context.Context, workingEnvironment, app, dbHost, dbUser, dbPass, dbName, dbPort, dbDriver,
    eazybiDbHost, eazybiDbName, eazybiDbUser, eazybiDbPass, eazybiDbPort, sharedHomeDir, homeDir, installDir, serverUser,
    serverIPs, serverPassword, baseUrl, dbTmp, dbTmp2, dbTmpEazybi, eazybiDbFilePath, remoteTempFolder, tempRestoreFolder,
    backupDirHome, dataCurrentBackup, dataRestoreFile, nfsRestoreFile string, restoreAttachments, restoreDatabase bool,
    dataFolder string, restoreEazyBI, nfsRestoreOption bool) error {
           
    // Step 0: Check server and database connections
    log.Printf("Checking server connections...")
    if err := checkServerConnections(strings.Split(serverIPs, " "), serverUser, serverPassword); err != nil {
        UpdateRestoreStatus(workingEnvironment, 0, fmt.Sprintf("Server connection check failed: %v", err), true)
        return err
    }
    log.Printf("All server connections successful.")

    log.Printf("Checking database connections...")
    if err := checkDatabaseConnections(dbHost, dbPort, dbUser, dbPass,dbName, dbDriver, eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName, dbDriver, restoreDatabase, restoreEazyBI); err != nil {
        UpdateRestoreStatus(workingEnvironment, 0, fmt.Sprintf("Database connection check failed: %v", err), true)
        return err
    }
    log.Printf("All database connections successful.")

    restoreStatus[workingEnvironment] = struct{ Progress int; Message string }{Progress: 0, Message: "Starting restore..."}
    UpdateRestoreStatus(workingEnvironment, 0, "Starting restore process..." ,false)

    // Step 1: Check Version Compliance
    if ctx.Err() != nil {
        UpdateRestoreStatus(workingEnvironment, 0, "Restore cancelled", true)
        return ctx.Err()
    }
    UpdateRestoreStatus(workingEnvironment, 5, "Checking version compliance", false)
    log.Printf("Checking Version Compliance")
    err := HandleRestore(
        dbDriver,
        dbHost,
        dbUser,
        dbPass,
        dbName,
        dbPort,
        dbTmp,
        tempRestoreFolder,
        tempRestoreFolder,
        serverUser,
        serverPassword,
        app,
        eazybiDbName,
    )
    if err != nil {
        log.Printf("Restore process failed during version compliance check: %v", err)
        UpdateRestoreStatus(workingEnvironment, 10, "Restore failed during version compliance check.", false)
        return err
    }

    if ctx.Err() != nil {
        UpdateRestoreStatus(workingEnvironment, 10, "Restore cancelled", true)
        return ctx.Err()
    }
    UpdateRestoreStatus(workingEnvironment, 10, "Version compliance check complete.", false)

    // Step 2: Stop all apps (Jira or Confluence) on all nodes
    if ctx.Err() != nil {
        UpdateRestoreStatus(workingEnvironment, 10, "Restore cancelled", true)
        return ctx.Err()
    }
    UpdateRestoreStatus(workingEnvironment, 20, fmt.Sprintf("Stopping %s applications on all nodes", app), false)
    log.Printf("Stopping %s applications on all nodes", app)
    err = StopAppOnServers(workingEnvironment, app, serverUser, serverPassword)
    if err != nil {
        log.Printf("Failed to stop %s apps for environment %s: %v", app, workingEnvironment, err)
        UpdateRestoreStatus(workingEnvironment, 30, fmt.Sprintf("Failed to stop %s apps", app), false)
        return err
    }

    if ctx.Err() != nil {
        UpdateRestoreStatus(workingEnvironment, 30, "Restore cancelled", true)
        return ctx.Err()
    }
    UpdateRestoreStatus(workingEnvironment, 30, fmt.Sprintf("Successfully stopped %s applications.", app), false)

    // Step 3: Restore the Database and eazyBI (if selected)
    if restoreDatabase {
        if ctx.Err() != nil {
            UpdateRestoreStatus(workingEnvironment, 30, "Restore cancelled", true)
            return ctx.Err()
        }
        UpdateRestoreStatus(workingEnvironment, 40, "Restoring database", false)
        log.Printf("Restoring databases for environment: %s", workingEnvironment)
        err = RestoreDatabase(
            workingEnvironment,
            dbDriver,
            dbHost,
            dbPort,
            dbUser,
            dbPass,
            dbName,
            dbTmp,
            dbTmp2,
            tempRestoreFolder,
            serverUser,
            serverPassword,
            baseUrl,
            remoteTempFolder,
            app,
        )
        if err != nil {
            log.Printf("Failed to restore databases for %s: %v", workingEnvironment, err)
            UpdateRestoreStatus(workingEnvironment, 50, "Failed to restore databases.", false)
            return err
        }

        if ctx.Err() != nil {
            UpdateRestoreStatus(workingEnvironment, 50, "Restore cancelled", true)
            return ctx.Err()
        }
        log.Printf("Databases restored successfully for environment: %s", workingEnvironment)
        UpdateRestoreStatus(workingEnvironment, 50, "Database restore complete.", false)
    }else {
        log.Printf("Dropping temporary database %s as user chose not to restore the database", dbTmp)
        err = DropDatabase(dbDriver, dbHost, dbName, dbUser, dbPass, dbPort, dbTmp, serverUser, serverPassword)
        if err != nil {
            log.Printf("Failed to drop temporary database %s: %v", dbTmp, err)
            UpdateRestoreStatus(workingEnvironment, 50, "Database restore complete.", false)
            if ctx.Err() != nil {
                UpdateRestoreStatus(workingEnvironment, 50, "Restore cancelled", true)
                return ctx.Err()
            }
         }
    }

    // Step 4: Restore eazyBI (if selected)
    if restoreEazyBI {
        if ctx.Err() != nil {
            UpdateRestoreStatus(workingEnvironment, 50, "Restore cancelled", true)
            return ctx.Err()
        }
        UpdateRestoreStatus(workingEnvironment, 60, "Restoring eazyBI", false)
        log.Printf("Restoring eazyBI for environment: %s", workingEnvironment)
        err := RestoreEazyBI(
            workingEnvironment,
            dbDriver,
            eazybiDbHost,
            eazybiDbPort,
            eazybiDbUser,
            eazybiDbPass,
            eazybiDbName,
            dbTmpEazybi,
            tempRestoreFolder,
            serverUser,
            serverPassword,
            remoteTempFolder,
            eazybiDbFilePath,
        )
        if err != nil {
            log.Printf("Failed to restore eazyBI for %s: %v", workingEnvironment, err)
            UpdateRestoreStatus(workingEnvironment, 70, "Failed to restore eazyBI.", false)
            return err
        }

        if ctx.Err() != nil {
            UpdateRestoreStatus(workingEnvironment, 70, "Restore cancelled", true)
            return ctx.Err()
        }
        log.Printf("eazyBI restored successfully for environment: %s", workingEnvironment)
        UpdateRestoreStatus(workingEnvironment, 70, "Restore eazyBI complete successfully.", false)
    }

    // Step 5: Restore Local Home Directory
    if ctx.Err() != nil {
        UpdateRestoreStatus(workingEnvironment, 70, "Restore cancelled", true)
        return ctx.Err()
    }
    log.Printf("Restoring Local Home Directory for environment: %s", workingEnvironment)
    UpdateRestoreStatus(workingEnvironment, 75, "Restoring local home directory", false)
    err = RestoreLocalHomeDir(
        app,
        homeDir,
        backupDirHome,
        installDir,
        serverUser,
        serverIPs,
        serverPassword,
        tempRestoreFolder,
    )
    if err != nil {
        log.Printf("Failed to restore local home directory for %s: %v", workingEnvironment, err)
        UpdateRestoreStatus(workingEnvironment, 80, "Failed to restore local home directory.", false)
        return err
    }

    if ctx.Err() != nil {
        UpdateRestoreStatus(workingEnvironment, 80, "Restore cancelled", true)
        return ctx.Err()
    }
    log.Printf("Local home directory restored successfully for environment: %s", workingEnvironment)
    UpdateRestoreStatus(workingEnvironment, 80, "Restore local home directory complete successfully!", false)

    // Step 6: Restore Attachments
    if restoreAttachments {
        if ctx.Err() != nil {
            UpdateRestoreStatus(workingEnvironment, 80, "Restore cancelled", true)
            return ctx.Err()
        }
        if sharedHomeDir != "" {
            log.Printf("Restoring Attachments in shared home directory for environment: %s", workingEnvironment)
            UpdateRestoreStatus(workingEnvironment, 85, "Restoring attachments in shared home directory", false)
            err := RestoreSharedHomeDir(
                app,
                sharedHomeDir,
                installDir,
                dataRestoreFile,
                dataCurrentBackup,
                dataFolder,
                serverUser,
                serverIPs,
                serverPassword,
            )
            if err != nil {
                log.Printf("Failed to restore Attachments shared home directory for %s: %v", workingEnvironment, err)
                UpdateRestoreStatus(workingEnvironment, 90, "Failed to restore Attachments shared home directory.", true)
                return err
            }
            log.Printf("Attachments shared home directory restored successfully for environment: %s", workingEnvironment)
            UpdateRestoreStatus(workingEnvironment, 90, "Attachments restore on shared home directory complete successfully.", false)
        } else {
            log.Printf("Restoring attachments on each server for environment: %s", workingEnvironment)
            err := RestoreAttachments(
                app,
                homeDir,
                installDir,
                dataRestoreFile,
                serverUser,
                serverIPs,
                serverPassword,
            )
            if err != nil {
                log.Printf("Failed to restore attachments on servers for %s: %v", workingEnvironment, err)
                UpdateRestoreStatus(workingEnvironment, 90, "Failed to restore attachments on servers.", true)
                return err
            }
            log.Printf("Attachments restored successfully on servers for environment: %s", workingEnvironment)
            UpdateRestoreStatus(workingEnvironment, 90, "Attachments restore on servers complete successfully.", false)
        }
    }

    // Step 7: Restore NFS
    if nfsRestoreOption {
        if ctx.Err() != nil {
            UpdateRestoreStatus(workingEnvironment, 90, "Restore cancelled", true)
            return ctx.Err()
        }
        log.Printf("Restoring NFS for environment: %s", workingEnvironment)
        UpdateRestoreStatus(workingEnvironment, 90, "Restoring Shared home directory.", false)
        err := RestoreNFSDir(
            app,
            sharedHomeDir,
            installDir,
            nfsRestoreFile,
            dataCurrentBackup,
            dataFolder,
            serverUser,
            serverIPs,
            serverPassword,
        )
        if err != nil {
            log.Printf("Failed to restore shared home directory for %s: %v", workingEnvironment, err)
            UpdateRestoreStatus(workingEnvironment, 95, "Failed to restore Shared home directory.", true)
            return err
        }
        log.Printf("Shared home directory restored successfully for environment: %s", workingEnvironment)
        UpdateRestoreStatus(workingEnvironment, 95, "Shared home directory restored successfully.", false)
    }

    // Step 8: Start all (Jira or Confluence) apps on all nodes
    if ctx.Err() != nil {
        UpdateRestoreStatus(workingEnvironment, 95, "Restore cancelled", true)
        return ctx.Err()
    }
    log.Printf("Starting %s applications on all nodes", app)
    err = StartAppOnServers(workingEnvironment, app, serverUser, serverPassword)
    if err != nil {
        log.Printf("Failed to start %s apps for %s: %v", app, workingEnvironment, err)
        UpdateRestoreStatus(workingEnvironment, 97, fmt.Sprintf("Failed to start %s apps.", app), true)
        return err
    }
    log.Printf("Successfully started all %s apps for environment: %s", app, workingEnvironment)
    UpdateRestoreStatus(workingEnvironment, 97, fmt.Sprintf("Successfully started all %s apps for environment: %s", app, workingEnvironment), false)

    if ctx.Err() != nil {
        UpdateRestoreStatus(workingEnvironment, 97, "Restore cancelled", true)
        return ctx.Err()
    }

    UpdateRestoreStatus(workingEnvironment, 100, "Restore completed successfully.", false)

    return nil
}      

func checkServerConnections(serverIPs []string, serverUser, serverPassword string) error {
    for _, serverIP := range serverIPs {
        client, err := connectToServer(serverIP, serverUser, serverPassword)
        if err != nil {
            return fmt.Errorf("failed to connect to server at IP %s: %v", serverIP, err)
        }
        defer client.Close() // Close the connection after verifying each server
        log.Printf("Successfully connected to server: %s", serverIP)
    }
    return nil
}

func checkDatabaseConnections(dbHost, dbPort, dbUser, dbPass, dbName, dbDriver string, eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName, eazybiDriver string, restoreDatabase, restoreEazyBI bool) error {
    // Check primary database connection
    if err := checkPrimaryDatabaseConnection(dbHost, dbPort, dbUser, dbPass, dbName, dbDriver, restoreDatabase); err != nil {
        return fmt.Errorf("primary database connection check failed: %v", err)
    }

    // Check eazyBI database connection
    if err := checkEazyBIDatabaseConnection(eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName, eazybiDriver, restoreEazyBI); err != nil {
        return fmt.Errorf("eazyBI database connection check failed: %v", err)
    }

    return nil
}

func checkPrimaryDatabaseConnection(dbHost, dbPort, dbUser, dbPass, dbName, dbDriver string, restoreDatabase bool) error {
    const dbTestQuery = "SELECT 1"

    if !restoreDatabase {
        log.Printf("Skipping primary database connection check as restoreDatabase is set to false.")
        return nil
    }

    log.Printf("Attempting to connect to primary database at %s:%s", dbHost, dbPort)

    var dbConnStr string
    var dbDriverName string

    switch dbDriver {
    case "org.postgresql.Driver":
        dbDriverName = "postgres"
        dbConnStr = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", dbHost, dbPort, dbUser, dbPass, dbName)
    case "mysql":
        dbDriverName = "mysql"
        dbConnStr = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", dbUser, dbPass, dbHost, dbPort, dbName)
    case "com.microsoft.sqlserver.jdbc.SQLServerDriver":
        dbDriverName = "sqlserver"
        dbConnStr = fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s", dbUser, dbPass, dbHost, dbPort, dbName)
    case "oracle":
        log.Println("Oracle connection checking is not yet implemented.")
        return fmt.Errorf("unsupported database type: Oracle")
    default:
        return fmt.Errorf("unsupported database type: %s", dbDriver)
    }

    db, err := sql.Open(dbDriverName, dbConnStr)
    if err != nil {
        return fmt.Errorf("failed to connect to primary database: %v", err)
    }
    defer db.Close()

    // Execute a test query
    if _, err := db.Exec(dbTestQuery); err != nil {
        return fmt.Errorf("primary database connection test failed: %v", err)
    }

    log.Printf("Successfully connected to primary database at %s:%s", dbHost, dbPort)
    return nil
}

/*
func checkEazyBIDatabaseConnection(eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName, eazybiDriver string, restoreEazyBI bool) error {
    const dbTestQuery = "SELECT 1"

    if !restoreEazyBI {
        log.Printf("Skipping eazyBI database connection check as restoreEazyBI is set to false.")
        return nil
    }

    log.Printf("Attempting to connect to eazyBI database at %s:%s", eazybiDbHost, eazybiDbPort)

    var eazybiConnStr string
    var eazybiDriverName string

    switch eazybiDriver {
    case "org.postgresql.Driver":
        eazybiDriverName = "postgres"
        eazybiConnStr = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName)
    case "mysql":
        eazybiDriverName = "mysql"
        eazybiConnStr = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", eazybiDbUser, eazybiDbPass, eazybiDbHost, eazybiDbPort, eazybiDbName)
    case "com.microsoft.sqlserver.jdbc.SQLServerDriver":
        eazybiDriverName = "sqlserver"
        eazybiConnStr := fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s", 
            url.QueryEscape(eazybiDbUser), 
            url.QueryEscape(eazybiDbPass), 
            eazybiDbHost, 
            eazybiDbPort, 
            eazybiDbName)
        log.Printf("eazyBI connection string: %s", eazybiConnStr)

    case "oracle":
        log.Println("Oracle connection checking is not yet implemented.")
        return fmt.Errorf("unsupported database type: Oracle for eazyBI")
    default:
        return fmt.Errorf("unsupported eazyBI database type: %s", eazybiDriver)
    }

    eazybiDB, err := sql.Open(eazybiDriverName, eazybiConnStr)
    if err != nil {
        log.Printf("Failed connection string: %s", eazybiConnStr)
        return fmt.Errorf("failed to connect to eazyBI database: %v", err)
    }
    defer eazybiDB.Close()

    // Execute a test query
    if _, err := eazybiDB.Exec(dbTestQuery); err != nil {
        return fmt.Errorf("eazyBI database connection test failed: %v", err)
    }

    log.Printf("Successfully connected to eazyBI database at %s:%s", eazybiDbHost, eazybiDbPort)
    return nil
}
*/

func checkEazyBIDatabaseConnection(eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName, eazybiDriver string, restoreEazyBI bool) error {
    const dbTestQuery = "SELECT 1"

    if !restoreEazyBI {
        log.Printf("Skipping eazyBI database connection check as restoreEazyBI is set to false.")
        return nil
    }

    log.Printf("Attempting to connect to eazyBI database at %s:%s using driver %s", eazybiDbHost, eazybiDbPort, eazybiDriver)

    var connString string
    var driverName string

    // Construct connection string based on the driver
    switch eazybiDriver {
    case "org.postgresql.Driver":
        driverName = "postgres"
        connString = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
            eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName)
    case "mysql":
        driverName = "mysql"
        connString = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
            eazybiDbUser, eazybiDbPass, eazybiDbHost, eazybiDbPort, eazybiDbName)
    case "com.microsoft.sqlserver.jdbc.SQLServerDriver":
        driverName = "sqlserver"
        connString = fmt.Sprintf("server=%s;port=%s;user id=%s;password=%s;database=%s;encrypt=disable",
            eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName)
    case "oracle":
        log.Println("Oracle connection checking is not yet implemented.")
        return fmt.Errorf("unsupported database type: Oracle for eazyBI")
    default:
        return fmt.Errorf("unsupported eazyBI database type: %s", eazybiDriver)
    }

    log.Printf("eazyBI connection string: %s", connString)

    // Open the database connection
    db, err := sql.Open(driverName, connString)
    if err != nil {
        return fmt.Errorf("failed to connect to eazyBI database: %v", err)
    }
    defer db.Close()

    // Execute a test query
    if _, err := db.Exec(dbTestQuery); err != nil {
        return fmt.Errorf("eazyBI database connection test failed: %v", err)
    }

    log.Printf("Successfully connected to eazyBI database at %s:%s using driver %s", eazybiDbHost, eazybiDbPort, eazybiDriver)
    return nil
}



func UpdateRestoreStatus(workingEnvironment string, progress int, message string, isError bool) {
    restoreStatus[workingEnvironment] = struct{ Progress int; Message string }{Progress: progress, Message: message}
    if isError {
        log.Printf("Restore Error for %s: %s", workingEnvironment, message)
    } else {
        log.Printf("Restore Progress for %s: %s", workingEnvironment, message)
    }
}

func HandleCancelRestore(w http.ResponseWriter, r *http.Request) {
    environmentName := extractEnvironmentName(r.URL.Path)
    if cancelFunc, exists := restoreCancelFuncs[environmentName]; exists {
        cancelFunc() // Trigger cancellation
        delete(restoreCancelFuncs, environmentName) // Cleanup
        restoreStatus[environmentName] = struct{ Progress int; Message string }{Progress: 0, Message: "Restore cancelled by user."}
        http.Redirect(w, r, fmt.Sprintf("/environment/restore-progress/%s", environmentName), http.StatusSeeOther)
    } else {
        http.Error(w, "No ongoing restore found to cancel.", http.StatusBadRequest)
    }
}

func CheckAndRedirectRestoreProgress(w http.ResponseWriter, r *http.Request, environmentName string) bool {
    status, exists := restoreStatus[environmentName]
    if exists && status.Progress > 0 && status.Progress < 100 {
        http.Redirect(w, r, fmt.Sprintf("/environment/restore-progress/%s", environmentName), http.StatusSeeOther)
        return true
    }
    return false
}