package handlers

import (
    "fmt"
    "html"
    "html/template"
    "io"
    "log"
    "net/http"
    "os"
    "path/filepath"
)

// HandleCleanupBackupsPage renders the cleanup page for backup files
func HandleCleanupBackupsPage(w http.ResponseWriter, r *http.Request) {
    username, err := GetCurrentUsername(r)
    if err != nil {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }
    isAdmin, _ := IsAdminUser(username)

    apps, err := getApps()
    if err != nil {
        log.Printf("Failed to retrieve apps: %v", err)
        http.Error(w, "Failed to load apps.", http.StatusInternalServerError)
        return
    }

    selectedApp := r.URL.Query().Get("app")

    var environments []string
    if selectedApp != "" {
        environments, err = getEnvironmentsByApp(selectedApp)
        if err != nil {
            log.Printf("Failed to retrieve environments for app %s: %v", selectedApp, err)
            http.Error(w, "Failed to load environments.", http.StatusInternalServerError)
            return
        }
    }

    // Build app options
    appOptions := ""
    for _, app := range apps {
        selected := ""
        if app == selectedApp {
            selected = " selected"
        }
        appOptions += fmt.Sprintf(`<option value="%s"%s>%s</option>`, html.EscapeString(app), selected, html.EscapeString(app))
    }

    // Build environment cards
    envCardsHTML := ""
    if selectedApp != "" && len(environments) > 0 {
        for _, env := range environments {
            envCardsHTML += fmt.Sprintf(`
                <a href="/get-backup-dates?environment=%s" class="ads-card-flat"
                   style="display:flex; align-items:center; gap:16px; padding:14px 20px; text-decoration:none; color:var(--color-text); margin-bottom:8px;">
                    <span style="font-size:22px;">📁</span>
                    <div>
                        <div style="font-weight:600;">%s</div>
                        <div style="font-size:12px; color:var(--color-text-subtle);">Click to view backups</div>
                    </div>
                    <span style="margin-left:auto; color:var(--color-text-subtle);">→</span>
                </a>`, html.EscapeString(env), html.EscapeString(env))
        }
    } else if selectedApp != "" {
        envCardsHTML = `<div style="text-align:center; padding:24px; color:var(--color-text-subtle);">No environments found for this app.</div>`
    }

    content := fmt.Sprintf(`
        <div class="ads-page-centered"><div class="ads-page-content"><div class="ads-breadcrumbs"><a href="/">Home</a> → Cleanup Backups</div>
        
            <div class="ads-card-flat" style="margin-top:16px;">
                <div class="ads-card-header">
                    <span style="font-size:24px;">🧹</span>
                    <div>
                        <span class="ads-card-title">Cleanup Backups</span>
                        <div style="font-size:13px; color:var(--color-text-subtle); margin-top:2px;">Select an application and environment to manage backup files</div>
                    </div>
                </div>
                <div style="padding:0 24px 24px;">
                    <form id="appForm" method="GET" action="/cleanup-backups">
                        <div class="ads-form-group">
                            <label class="ads-form-label" for="app">Application</label>
                            <select class="ads-input" id="app" name="app" onchange="document.getElementById('appForm').submit();">
                                <option value="">-- Select App --</option>
                                %s
                            </select>
                        </div>
                    </form>
                    %s
                    <div style="margin-top:16px;">
                        <a href="/" class="ads-button ads-button-default">← Back to Home</a>
                    </div>
                </div>
            </div>
        </div>
    </div></div>
    `, appOptions, envCardsHTML)

    RenderPage(w, PageData{
        Title:   "Cleanup Backups",
        IsAdmin: isAdmin,
        Content: template.HTML(content),
    })
}

// HandleGetBackupDatesAndTimes retrieves available dates and times for a selected environment
func HandleGetBackupDatesAndTimes(w http.ResponseWriter, r *http.Request) {
    environment := r.URL.Query().Get("environment")

    var appName string
    err := db.QueryRow("SELECT app FROM environments WHERE name = ?", environment).Scan(&appName)
    if err != nil {
        log.Printf("Failed to retrieve app for environment %s: %v", environment, err)
        http.Error(w, "Failed to load app information.", http.StatusInternalServerError)
        return
    }

    backupDir := filepath.Join("/adminToolBackupDirectory", appName, environment)

    dates, err := os.ReadDir(backupDir)
    if err != nil {
        log.Printf("Failed to read backup directory: %v", err)
        // Show empty state instead of error
        username, _ := GetCurrentUsername(r)
        isAdmin, _ := IsAdminUser(username)
        content := fmt.Sprintf(`
            <div class="ads-page-centered"><div class="ads-page-content"><div class="ads-breadcrumbs"><a href="/">Home</a> → <a href="/cleanup-backups">Cleanup</a> → %s</div>
            
                <div class="ads-card-flat" style="margin-top:16px;">
                    <div class="ads-card-header"><span class="ads-card-title">Cleanup Backups — %s</span></div>
                    <div style="padding:24px; text-align:center; color:var(--color-text-subtle);">
                        No backup directory found for this environment.
                    </div>
                    <div style="padding:0 24px 24px;"><a href="/cleanup-backups?app=%s" class="ads-button ads-button-default">← Back</a></div>
                </div>
            </div></div></div>`, html.EscapeString(environment), html.EscapeString(environment), html.EscapeString(appName))
        RenderPage(w, PageData{Title: "Cleanup - " + environment, IsAdmin: isAdmin, Content: template.HTML(content)})
        return
    }

    dateTimes := make(map[string][]string)
    totalCount := 0
    for _, date := range dates {
        if date.IsDir() {
            dateDir := filepath.Join(backupDir, date.Name())
            times, err := os.ReadDir(dateDir)
            if err != nil {
                continue
            }
            var timeFolders []string
            for _, timeFolder := range times {
                if timeFolder.IsDir() {
                    timeFolders = append(timeFolders, timeFolder.Name())
                    totalCount++
                }
            }
            if len(timeFolders) > 0 {
                dateTimes[date.Name()] = timeFolders
            }
        }
    }

    // Build table rows
    rowsHTML := ""
    for date, times := range dateTimes {
        for _, t := range times {
            rowsHTML += fmt.Sprintf(`
                <tr>
                    <td style="text-align:center;"><input type="checkbox" name="selectedTimes" value="%s/%s" class="backup-check"></td>
                    <td>📅 %s</td>
                    <td>🕐 %s</td>
                </tr>`, html.EscapeString(date), html.EscapeString(t),
                html.EscapeString(date), html.EscapeString(t))
        }
    }

    if rowsHTML == "" {
        rowsHTML = `<tr><td colspan="3" style="text-align:center; padding:24px; color:var(--color-text-subtle);">No backups found</td></tr>`
    }

    extraHead := template.HTML(`<script>
        function toggleAll(source) {
            var checks = document.querySelectorAll('.backup-check');
            checks.forEach(function(c) { c.checked = source.checked; });
            updateCount();
        }
        function updateCount() {
            var checked = document.querySelectorAll('.backup-check:checked').length;
            var btn = document.getElementById('delete-btn');
            if (checked > 0) {
                btn.textContent = '🗑 Delete ' + checked + ' Selected';
                btn.disabled = false;
                btn.style.opacity = '1';
            } else {
                btn.textContent = '🗑 Delete Selected';
                btn.disabled = true;
                btn.style.opacity = '0.5';
            }
        }
        document.addEventListener('DOMContentLoaded', function() {
            document.querySelectorAll('.backup-check').forEach(function(c) {
                c.addEventListener('change', updateCount);
            });
        });
    </script>`)

    content := fmt.Sprintf(`
        <div class="ads-page-centered"><div class="ads-page-content"><div class="ads-breadcrumbs"><a href="/">Home</a> → <a href="/cleanup-backups">Cleanup</a> → %s</div>
        
            <div class="ads-card-flat" style="margin-top:16px;">
                <div class="ads-card-header">
                    <span style="font-size:24px;">🧹</span>
                    <div>
                        <span class="ads-card-title">Cleanup Backups — %s</span>
                        <div style="font-size:13px; color:var(--color-text-subtle); margin-top:2px;">
                            <span class="ads-lozenge ads-lozenge-info">%s</span>
                            %d backup(s) found
                        </div>
                    </div>
                </div>
                <form id="deleteForm" method="POST" action="/delete-backups" style="padding:0 24px 24px;">
                    <input type="hidden" name="environment" value="%s">
                    <table class="ads-table" style="width:100%%;">
                        <thead>
                            <tr>
                                <th style="width:40px; text-align:center;"><input type="checkbox" onchange="toggleAll(this)"></th>
                                <th>Date</th>
                                <th>Time</th>
                            </tr>
                        </thead>
                        <tbody>%s</tbody>
                    </table>
                    <div style="margin-top:16px; display:flex; gap:8px;">
                        <button type="submit" id="delete-btn" class="ads-button ads-button-danger" disabled style="opacity:0.5;"
                                onclick="return confirm('Are you sure you want to delete the selected backups? This cannot be undone.')">
                            🗑 Delete Selected
                        </button>
                        <a href="/cleanup-backups?app=%s" class="ads-button ads-button-default">← Back</a>
                    </div>
                </form>
            </div>
        </div>
    </div></div>
    `, html.EscapeString(environment), html.EscapeString(environment),
       html.EscapeString(appName), totalCount,
       html.EscapeString(environment), rowsHTML, html.EscapeString(appName))

    username, _ := GetCurrentUsername(r)
    isAdmin, _ := IsAdminUser(username)

    RenderPage(w, PageData{
        Title:     "Cleanup - " + environment,
        IsAdmin:   isAdmin,
        ExtraHead: extraHead,
        Content:   template.HTML(content),
    })
}

func isDirEmpty(dir string) (bool, error) {
    f, err := os.Open(dir)
    if err != nil {
        return false, err
    }
    defer f.Close()
    _, err = f.Readdir(1)
    if err == io.EOF {
        return true, nil
    }
    return false, err
}

func getEnvironmentsByApp(app string) ([]string, error) {
    rows, err := db.Query("SELECT name FROM environments WHERE app = ?", app)
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve environments for app %s: %v", app, err)
    }
    defer rows.Close()

    var environments []string
    for rows.Next() {
        var environment string
        if err := rows.Scan(&environment); err != nil {
            return nil, fmt.Errorf("failed to scan environment: %v", err)
        }
        environments = append(environments, environment)
    }
    return environments, nil
}

func getApps() ([]string, error) {
    rows, err := db.Query("SELECT DISTINCT app FROM environments")
    if err != nil {
        return nil, fmt.Errorf("failed to retrieve apps: %v", err)
    }
    defer rows.Close()

    var apps []string
    for rows.Next() {
        var app string
        if err := rows.Scan(&app); err != nil {
            return nil, fmt.Errorf("failed to scan app: %v", err)
        }
        apps = append(apps, app)
    }
    return apps, nil
}

// HandleDeleteSelectedBackups handles the deletion of selected backups
func HandleDeleteSelectedBackups(w http.ResponseWriter, r *http.Request) {
    environment := r.FormValue("environment")

    var appName string
    err := db.QueryRow("SELECT app FROM environments WHERE name = ?", environment).Scan(&appName)
    if err != nil {
        log.Printf("Failed to retrieve app for environment %s: %v", environment, err)
        http.Error(w, "Failed to load app information.", http.StatusInternalServerError)
        return
    }

    selectedTimes := r.Form["selectedTimes"]
    baseBackupDir := filepath.Join("/adminToolBackupDirectory", appName, environment)

    for _, timePath := range selectedTimes {
        timeDir := filepath.Join(baseBackupDir, timePath)
        err := os.RemoveAll(timeDir)
        if err != nil {
            log.Printf("Failed to delete time folder %s: %v", timePath, err)
            continue
        }
        dateDir := filepath.Dir(timeDir)
        isEmpty, _ := isDirEmpty(dateDir)
        if isEmpty {
            os.RemoveAll(dateDir)
        }
    }

    http.Redirect(w, r, "/cleanup-backups", http.StatusSeeOther)
}
