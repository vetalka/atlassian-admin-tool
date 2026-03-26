package handlers

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
)

func HandleEnvironmentPage(w http.ResponseWriter, r *http.Request) {
	environmentName := r.URL.Path[len("/environment/"):]

	var appName string
	err := db.QueryRow("SELECT app FROM environments WHERE name = ?", environmentName).Scan(&appName)
	if err != nil {
		appName = "Jira"
	}

	username, err := GetCurrentUsername(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	allowedActions, err := GetCurrentActionsForUser(username)
	if err != nil {
		http.Error(w, "Failed to get user actions", http.StatusInternalServerError)
		return
	}

	isAdmin, err := IsAdminUser(username)
	if err != nil {
		http.Error(w, "Failed to check user permissions", http.StatusInternalServerError)
		return
	}

	extraHead := template.HTML(`<script>
        function ajaxAction(url, actionName) {
            var messageDiv = document.getElementById("message");
            messageDiv.className = "ads-alert ads-alert-info";
            messageDiv.innerHTML = "Processing " + actionName + "...";
            var xhr = new XMLHttpRequest();
            xhr.open("POST", url, true);
            xhr.setRequestHeader("Content-Type", "application/x-www-form-urlencoded");
            xhr.onload = function () {
                if (xhr.status === 200) {
                    messageDiv.className = "ads-alert ads-alert-success";
                    messageDiv.innerHTML = actionName + " completed successfully.";
                } else {
                    messageDiv.className = "ads-alert ads-alert-error";
                    messageDiv.innerHTML = "Failed to " + actionName.toLowerCase() + ". Check logs for details.";
                }
            };
            xhr.onerror = function() {
                messageDiv.className = "ads-alert ads-alert-error";
                messageDiv.innerHTML = "Network error during " + actionName.toLowerCase() + ".";
            };
            xhr.send();
        }
    </script>`)

	content := fmt.Sprintf(`
        <div class="ads-page-centered">
            <div class="ads-page-content">
                <div class="ads-page-header">
                    <div class="ads-breadcrumb">
                        <a href="/">Environments</a>
                        <span class="ads-breadcrumb-separator">/</span>
                        <span class="ads-breadcrumb-current">%s</span>
                    </div>
                    <h1>%s</h1>
                    <p class="ads-page-header-description">Manage your %s environment</p>
                </div>
                <div id="message"></div>
                <div class="ads-actions-grid">`, environmentName, environmentName, appName)

	type actionDef struct {
		key, label, desc, iconClass, iconSVG string
		isLink                               bool
		href, onclick                        string
	}
	actions := []actionDef{
		{"Environment Parameters", "Parameters", "View & edit configuration", "params",
			`<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>`,
			true, fmt.Sprintf("/environment/show/%s", environmentName), ""},
		{"Restart", fmt.Sprintf("Restart %s", appName), "Restart the application service", "restart",
			`<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"></polyline><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"></path></svg>`,
			false, "", fmt.Sprintf("ajaxAction('/environment/restart-app/%s/%s', 'Restart %s')", environmentName, appName, appName)},
		{"Stop", fmt.Sprintf("Stop %s", appName), "Stop the application service", "stop",
			`<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="6" y="6" width="12" height="12" rx="2"></rect></svg>`,
			false, "", fmt.Sprintf("ajaxAction('/environment/stop-app/%s/%s', 'Stop %s')", environmentName, appName, appName)},
		{"Start", fmt.Sprintf("Start %s", appName), "Start the application service", "start",
			`<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg>`,
			false, "", fmt.Sprintf("ajaxAction('/environment/start-app/%s/%s', 'Start %s')", environmentName, appName, appName)},
		{"Backup", "Backup", "Backup environment data", "backup",
			`<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="17 8 12 3 7 8"></polyline><line x1="12" y1="3" x2="12" y2="15"></line></svg>`,
			true, fmt.Sprintf("/environment/backup/%s", environmentName), ""},
		{"Restore", "Restore", "Restore from backup", "restore",
			`<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>`,
			true, fmt.Sprintf("/environment/restore/%s", environmentName), ""},
	}

	if isAdmin {
		actions = append(actions, actionDef{"Specific Node Actions", "Node Actions", "Manage individual cluster nodes", "nodes",
			`<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect><rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect><line x1="6" y1="6" x2="6.01" y2="6"></line><line x1="6" y1="18" x2="6.01" y2="18"></line></svg>`,
			true, fmt.Sprintf("/environment/get-config/%s/%s", environmentName, appName), ""})
	}

	for _, a := range actions {
		show := isAdmin || allowedActions[a.key+" "+appName] || allowedActions[a.key]
		if !show {
			continue
		}
		if a.isLink {
			content += fmt.Sprintf(`
                    <a href="%s" class="ads-action-card">
                        <div class="ads-action-card-icon %s">%s</div>
                        <div class="ads-action-card-label">%s</div>
                        <div class="ads-action-card-desc">%s</div>
                    </a>`, a.href, a.iconClass, a.iconSVG, a.label, a.desc)
		} else {
			content += fmt.Sprintf(`
                    <div class="ads-action-card" onclick="%s">
                        <div class="ads-action-card-icon %s">%s</div>
                        <div class="ads-action-card-label">%s</div>
                        <div class="ads-action-card-desc">%s</div>
                    </div>`, a.onclick, a.iconClass, a.iconSVG, a.label, a.desc)
		}
	}

	content += fmt.Sprintf(`
                </div>
                <div class="ads-action-bar" style="margin-top: 32px;">
                    <a href="/" class="ads-btn ads-btn-default">
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
                        Back to environments
                    </a>
                </div>
            </div>
        </div>`)

	RenderPage(w, PageData{
		Title:     environmentName,
		IsAdmin:   isAdmin,
		ExtraHead: extraHead,
		Content:   template.HTML(content),
	})
}

func GetCurrentActionsForUser(username string) (map[string]bool, error) {
	isAdmin, err := IsAdminUser(username)
	if err != nil {
		return nil, err
	}

	if isAdmin {
		return map[string]bool{
			"Environment Parameters": true, "Restart": true, "Stop": true,
			"Start": true, "Backup": true, "Restore": true, "Specific Node Actions": true,
		}, nil
	}

	rows, err := db.Query(`SELECT a.action FROM group_actions ga JOIN actions a ON ga.action_id = a.id JOIN users u ON u.groups = ga.group_name WHERE u.username = ?`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	currentActions := make(map[string]bool)
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			return nil, err
		}
		currentActions[action] = true
	}
	if len(currentActions) == 0 {
		log.Printf("No actions found for user: %s", username)
	}
	return currentActions, nil
}
