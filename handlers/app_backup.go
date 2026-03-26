package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
)

var backupCancelFuncs = make(map[string]context.CancelFunc)
var backupStatus = map[string]struct {
	Progress int    // Percentage of completion
	Message  string // Status message
}{}

func CheckIfRestoreInProgress(environmentName string) bool {
	status, exists := restoreStatus[environmentName]
	return exists && status.Progress > 0 && status.Progress < 100
}

func CheckAndRedirectProgress(w http.ResponseWriter, r *http.Request, environmentName string) bool {
	// Check if a backup process is already running for this environment
	status, exists := backupStatus[environmentName]
	if exists && status.Progress > 0 && status.Progress < 100 {
		// Redirect to the progress page if a backup is already in progress
		http.Redirect(w, r, fmt.Sprintf("/environment/backup-progress/%s", environmentName), http.StatusSeeOther)
		return true
	}
	return false
}

// HandleBackupOptions renders the backup options page for the given environment
func HandleBackupOptions(w http.ResponseWriter, r *http.Request) {
	environmentName := extractEnvironmentName(r.URL.Path)

	// Check if a restore is in progress for this environment
	if CheckIfRestoreInProgress(environmentName) {
		content := fmt.Sprintf(`
            <div class="ads-page-centered"><div class="ads-page-content"><div class="ads-breadcrumbs"><a href="/">Environments</a> → <a href="/environment/%s">%s</a> → Backup</div>
            <div style="max-width:600px; margin:40px auto; text-align:center;">
                <div class="ads-banner ads-banner-warning" style="margin-bottom:20px;">
                    Cannot start a backup while a restore is in progress.
                </div>
                <a href="/environment/%s" class="ads-button ads-button-default">Back to Environment Actions</a>
            </div>
        </div></div>
        `, html.EscapeString(environmentName), html.EscapeString(environmentName), html.EscapeString(environmentName))
		RenderPage(w, PageData{Title: "Backup Not Allowed", IsAdmin: func() bool { u, _ := GetCurrentUsername(r); a, _ := IsAdminUser(u); return a }(), Content: template.HTML(content)})
		return
	}

	// Check and redirect if there is a backup in progress
	if CheckAndRedirectProgress(w, r, environmentName) {
		return
	}

	// Fetch the app type for the environment
	var appType string
	query := "SELECT app FROM environments WHERE name = ?"
	err := db.QueryRow(query, environmentName).Scan(&appType)
	if err != nil {
		log.Printf("Failed to get app type for environment %s: %v", environmentName, err)
		http.Error(w, "Failed to load backup options. Check logs for details.", http.StatusInternalServerError)
		return
	}

	// Render backup options based on the app type (Jira or Confluence)
	renderBackupOptionsPage(w, r, environmentName, appType, "", false)
}

func renderBackupOptionsPage(w http.ResponseWriter, r *http.Request, environmentName, appType, message string, isError bool) {
	sanitizedEnvironmentName := html.EscapeString(environmentName)

	var eazybiExists bool
	query := "SELECT CASE WHEN eazybi_dbname IS NOT NULL AND eazybi_dbname <> '' THEN 1 ELSE 0 END FROM environments WHERE name = ?"
	err := db.QueryRow(query, environmentName).Scan(&eazybiExists)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to check eazyBI existence for environment %s: %v", environmentName, err)
		http.Error(w, "Failed to load backup options.", http.StatusInternalServerError)
		return
	}

	var nfsExists bool
	queryNFS := "SELECT CASE WHEN sharedhome_dir IS NOT NULL AND sharedhome_dir <> '' THEN 1 ELSE 0 END FROM environments WHERE name = ?"
	err = db.QueryRow(queryNFS, environmentName).Scan(&nfsExists)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to check NFS existence for environment %s: %v", environmentName, err)
	}

	messageHTML := ""
	if message != "" {
		bannerClass := "ads-banner-success"
		if isError {
			bannerClass = "ads-banner-warning"
		}
		messageHTML = fmt.Sprintf(`<div class="ads-banner %s" style="margin-bottom:20px;">%s</div>`, bannerClass, html.EscapeString(message))
	}

	// Build component cards
	type backupComp struct {
		ID, Label, Desc, Icon, Color string
		Show                         bool
	}
	components := []backupComp{
		{"backupAttachments", "Attachments", "Application data files and uploads", "&#x1F4CE;", "#0052CC", true},
		{"backupDatabase", "Database", "Full database dump (" + html.EscapeString(appType) + ")", "&#x1F5C4;", "#00875A", true},
		{"backupEazyBI", "eazyBI Database", "eazyBI analytics database", "&#x1F4CA;", "#6554C0", eazybiExists && appType == "Jira"},
		{"backupNFS", "NFS / Shared Home", "Shared filesystem data", "&#x1F4C1;", "#FF991F", nfsExists},
	}

	cardsHTML := ""
	for _, c := range components {
		if !c.Show {
			continue
		}
		cardsHTML += fmt.Sprintf(`
            <label style="display:flex; align-items:center; gap:16px; padding:16px 20px; background:rgba(0,82,204,0.04);
                          border:2px solid %s; border-radius:8px; cursor:pointer; transition:all 0.15s;"
                   class="bc">
                <input type="checkbox" name="%s" value="Yes" checked style="width:18px; height:18px; accent-color:%s;"
                       onchange="var l=this.closest('.bc');l.style.borderColor=this.checked?'%s':'var(--color-border)';l.style.background=this.checked?'rgba(0,82,204,0.04)':'var(--color-bg-card)'">
                <div style="width:40px; height:40px; background:linear-gradient(135deg, %s, %scc); border-radius:10px; display:flex; align-items:center; justify-content:center; flex-shrink:0;">
                    <span style="font-size:20px; color:white;">%s</span>
                </div>
                <div style="flex:1;">
                    <div style="font-weight:600; font-size:14px;">%s</div>
                    <div style="font-size:12px; color:var(--color-text-subtle); margin-top:2px;">%s</div>
                </div>
            </label>`, c.Color, c.ID, c.Color, c.Color, c.Color, c.Color, c.Icon, c.Label, c.Desc)
	}

	content := fmt.Sprintf(`
        <div class="ads-page-centered"><div class="ads-page-content"><div class="ads-breadcrumbs">
            <a href="/">Environments</a> &rarr;
            <a href="/environment/%s">%s</a> &rarr; Backup
        </div>

        <div class="ads-card-flat" style="margin-top:16px;">
            <div class="ads-card-header">
                <div style="width:48px; height:48px; background:linear-gradient(135deg, #0052CC, #2684FF); border-radius:12px; display:flex; align-items:center; justify-content:center;">
                    <span style="font-size:22px; color:white;">&#x1F4BE;</span>
                </div>
                <div style="flex:1;">
                    <span class="ads-card-title" style="font-size:18px;">Backup Options</span>
                    <div style="margin-top:4px;">
                        <span class="ads-lozenge ads-lozenge-info">%s</span>
                        <span style="font-size:13px; color:var(--color-text-subtle); margin-left:8px;">Environment: <strong>%s</strong></span>
                    </div>
                </div>
            </div>
            %s
            <form action="/environment/start-backup/%s" method="POST" style="padding:0 24px 24px;">
                <div style="font-size:14px; font-weight:600; margin-bottom:12px;">Select components to back up:</div>
                <div style="display:grid; grid-template-columns:1fr 1fr; gap:12px;">
                    %s
                </div>
                <div style="margin-top:20px; padding-top:16px; border-top:1px solid var(--color-border); display:flex; gap:8px; align-items:center;">
                    <button type="submit" class="ads-button ads-button-primary" style="padding:10px 24px; font-size:14px;">
                        &#x25B6; Start Backup
                    </button>
                    <a href="/environment/%s" class="ads-button ads-button-default">&larr; Cancel</a>
                </div>
            </form>
        </div>
    </div></div>
    `, sanitizedEnvironmentName, sanitizedEnvironmentName,
		html.EscapeString(appType), sanitizedEnvironmentName,
		messageHTML, sanitizedEnvironmentName,
		cardsHTML, sanitizedEnvironmentName)

	username, _ := GetCurrentUsername(r)
	isAdmin, _ := IsAdminUser(username)

	RenderPage(w, PageData{
		Title:   "Backup - " + environmentName,
		IsAdmin: isAdmin,
		Content: template.HTML(content),
	})
}

// Extracts the environment name from the URL path
func extractEnvironmentName(path string) string {
	pathParts := strings.Split(path, "/")
	if len(pathParts) > 4 {
		return strings.Join(pathParts[3:], "/")
	}
	return pathParts[len(pathParts)-1]
}

func HandleStartBackup(w http.ResponseWriter, r *http.Request) {
	environmentName := extractEnvironmentName(r.URL.Path)

	// Check and redirect if there is a backup in progress
	if CheckAndRedirectProgress(w, r, environmentName) {
		return
	}

	// Create a new context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	backupCancelFuncs[environmentName] = cancel

	// Initialize task for backup
	AddTask(environmentName)

	// Retrieve environment details from the database
	var app, dbHost, dbUser, dbPass, dbName, dbPort, dbDriver, eazybiDbName, eazybiDbUser, eazybiDbPass, eazybiDbPort, eazybiDbHost, sharedHomeDir, homeDir, serverUser, serverPassword, serverIP string
	query := `SELECT app, app_dbhost, app_dbuser, app_dbpass, app_dbname, app_dbport, db_driver, 
              eazybi_dbname, eazybi_dbuser, eazybi_dbpass, eazybi_dbport, eazybi_dbhost, 
              sharedhome_dir, home_dir, server_user, server_password, ip FROM environments WHERE name = ?`
	err := db.QueryRow(query, environmentName).Scan(&app, &dbHost, &dbUser, &dbPass, &dbName, &dbPort, &dbDriver,
		&eazybiDbName, &eazybiDbUser, &eazybiDbPass, &eazybiDbPort, &eazybiDbHost, &sharedHomeDir, &homeDir,
		&serverUser, &serverPassword, &serverIP)
	if err != nil {
		log.Printf("Failed to retrieve environment details for %s: %v", environmentName, err)
		renderBackupOptionsPage(w, r, environmentName, app, "Failed to retrieve environment details. Check logs for details.", true)
		return
	}

	// Get user choices from the form submission
	backupAttachments := r.FormValue("backupAttachments") == "Yes"
	backupDatabase := r.FormValue("backupDatabase") == "Yes"
	backupEazyBI := app == "Jira" && r.FormValue("backupEazyBI") == "Yes"
	backupNFS := r.FormValue("backupNFS") == "Yes" && sharedHomeDir != ""

	// Split the IPs into a slice and choose the first IP
	ips := strings.Split(serverIP, " ")
	selectedIP := strings.TrimSpace(ips[0]) // Use the first IP from the list

	// Start the backup process asynchronously
	go func() {
		defer delete(backupCancelFuncs, environmentName) // Clean up the cancel function when done

		err := performBackupTasks(ctx, environmentName, app, selectedIP, backupAttachments, backupDatabase,
			backupEazyBI, backupNFS, dbHost, dbUser, dbPass, dbName, dbPort, dbDriver,
			eazybiDbName, eazybiDbUser, eazybiDbPass, eazybiDbPort, eazybiDbHost,
			sharedHomeDir, homeDir, serverUser, serverPassword)
		if err != nil {
			log.Printf("Backup failed for %s: %v", environmentName, err)
			updateBackupStatus(environmentName, 0, "Backup failed: "+err.Error(), true)
		}
	}()

	// Redirect the user to the backup progress page
	http.Redirect(w, r, fmt.Sprintf("/environment/backup-progress/%s", environmentName), http.StatusSeeOther)
}

func performBackupTasks(ctx context.Context, environmentName, app, selectedIP string, backupAttachments, backupDatabase, backupEazyBI, backupNFS bool, dbHost, dbUser, dbPass, dbName, dbPort, dbDriver, eazybiDbName, eazybiDbUser, eazybiDbPass, eazybiDbPort, eazybiDbHost, sharedHomeDir, homeDir, serverUser, serverPassword string) error {
	backupStatus[environmentName] = struct {
		Progress int
		Message  string
	}{Progress: 0, Message: "Starting backup..."}
	log.Printf("Starting backup for %s", environmentName)

	// Connect to server
	client, err := connectToServer(selectedIP, serverUser, serverPassword)
	if err != nil {
		updateBackupStatus(environmentName, 0, "Failed to connect via SSH", true)
		return err
	}
	defer client.Close()

	// Check if context is cancelled
	if ctx.Err() != nil {
		updateBackupStatus(environmentName, 0, "Backup cancelled", true)
		return ctx.Err()
	}

	updateBackupStatus(environmentName, 10, "Connected to server", false)

	// Create backup directory
	backupDir, err := CreateBackupDirectory(app, environmentName)
	if err != nil {
		updateBackupStatus(environmentName, 10, "Failed to create backup directory", true)
		return err
	}

	// Ensure that the created backup directory is cleaned up if an error occurs
	defer func() {
		if err != nil {
			// Clean up backup directory if an error occurred
			log.Printf("Cleaning up backup directory: %s due to error", backupDir)
			removeErr := RemoveBackupDirectory(backupDir)
			if removeErr != nil {
				log.Printf("Failed to remove backup directory: %s. Error: %v", backupDir, removeErr)
			}
		}
	}()
	if ctx.Err() != nil {
		updateBackupStatus(environmentName, 10, "Backup cancelled", true)
		return ctx.Err()
	}

	updateBackupStatus(environmentName, 20, "Backup directory created", false)

	// Perform home directory backup
	err = BackupHomeDirectory(client, homeDir, backupDir, app)
	if err != nil {
		updateBackupStatus(environmentName, 20, "Error backing up home directory", true)
		return err
	}

	// Check if context is cancelled
	if ctx.Err() != nil {
		updateBackupStatus(environmentName, 20, "Backup cancelled", true)
		return ctx.Err()
	}

	updateBackupStatus(environmentName, 40, "Home directory backup completed", false)

	// Create backup directory for attachments
	backupDirAttachments, err := CreateBackupDirectoryAttachments(app, environmentName)
	if err != nil {
		updateBackupStatus(environmentName, 41, "Failed to create attachments backup directory", true)
		return err
	}

	// Check if context is cancelled
	if ctx.Err() != nil {
		updateBackupStatus(environmentName, 41, "Backup cancelled", true)
		return ctx.Err()
	}

	// Backup NFS if selected
	if backupNFS {
		err = BackupNFS(client, sharedHomeDir, homeDir, backupDirAttachments, app)
		if err != nil {
			updateBackupStatus(environmentName, 42, "Error backing up NFS", true)
			return err
		}

		// Check if context is cancelled
		if ctx.Err() != nil {
			updateBackupStatus(environmentName, 42, "Backup cancelled", true)
			return ctx.Err()
		}

		updateBackupStatus(environmentName, 60, "NFS backup completed", false)
	}

	// Backup attachments if selected
	if backupAttachments {
		err = BackupAttachments(client, sharedHomeDir, homeDir, backupDirAttachments, app)
		if err != nil {
			updateBackupStatus(environmentName, 60, "Error backing up attachments", true)
			return err
		}

		// Check if context is cancelled
		if ctx.Err() != nil {
			updateBackupStatus(environmentName, 60, "Backup cancelled", true)
			return ctx.Err()
		}

		updateBackupStatus(environmentName, 70, "Attachments backup completed", false)
	}

	// Backup the database if selected
	if backupDatabase {
		log.Printf("Backing up database for %s to %s...", environmentName, backupDir)
		err = BackupDatabaseForEnv(environmentName, dbHost, dbUser, dbPass, dbName, dbPort, dbDriver, backupDir)
		if err != nil {
			updateBackupStatus(environmentName, 70, "Error backing up database", true)
			return err
		}

		// Check if context is cancelled
		if ctx.Err() != nil {
			updateBackupStatus(environmentName, 70, "Backup cancelled", true)
			return ctx.Err()
		}

		updateBackupStatus(environmentName, 80, "Database backup completed", false)
	}

	// Backup eazyBI if selected
	if backupEazyBI {
		log.Printf("Backing up eazyBI for %s to %s...", environmentName, backupDir)
		err = BackupDatabaseForEnv(environmentName, eazybiDbHost, eazybiDbUser, eazybiDbPass, eazybiDbName, eazybiDbPort, dbDriver, backupDir)
		if err != nil {
			updateBackupStatus(environmentName, 80, "Error backing up eazyBI database", true)
			return err
		}

		// Check if context is cancelled
		if ctx.Err() != nil {
			updateBackupStatus(environmentName, 80, "Backup cancelled", true)
			return ctx.Err()
		}

		updateBackupStatus(environmentName, 90, "eazyBI backup completed", false)
	}

	// Final check before completing
	if ctx.Err() != nil {
		updateBackupStatus(environmentName, 90, "Backup cancelled", true)
		return ctx.Err()
	}

	updateBackupStatus(environmentName, 100, "Backup completed successfully", false)
	err = nil // If all tasks are successful, set error to nil so cleanup doesn't delete the folder
	return nil
}

func updateBackupStatus(environmentName string, progress int, message string, isError bool) {
	backupStatus[environmentName] = struct {
		Progress int
		Message  string
	}{Progress: progress, Message: message}
	if isError {
		log.Printf("Backup Error for %s: %s", environmentName, message)
	} else {
		log.Printf("Backup Progress for %s: %s", environmentName, message)
	}
}

func HandleCancelBackup(w http.ResponseWriter, r *http.Request) {
	environmentName := extractEnvironmentName(r.URL.Path)
	if cancelFunc, exists := backupCancelFuncs[environmentName]; exists {
		cancelFunc()                               // Trigger cancellation
		delete(backupCancelFuncs, environmentName) // Cleanup
		backupStatus[environmentName] = struct {
			Progress int
			Message  string
		}{Progress: 0, Message: "Backup cancelled by user."}
		http.Redirect(w, r, fmt.Sprintf("/environment/backup-progress/%s", environmentName), http.StatusSeeOther)
	} else {
		http.Error(w, "No ongoing backup found to cancel.", http.StatusBadRequest)
	}
}

// RemoveBackupDirectory removes the specified backup directory.
func RemoveBackupDirectory(dirPath string) error {
	log.Printf("Attempting to remove backup directory: %s", dirPath)
	err := os.RemoveAll(dirPath) // Use os.RemoveAll to delete the folder and its contents
	if err != nil {
		return fmt.Errorf("failed to remove directory %s: %v", dirPath, err)
	}
	log.Printf("Successfully removed backup directory: %s", dirPath)
	return nil
}
