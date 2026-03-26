package handlers

import (
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func HandleMyAccount(w http.ResponseWriter, r *http.Request) {
	username, err := GetCurrentUsername(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	isAdmin, _ := IsAdminUser(username)

	if r.Method == http.MethodPost {
		currentPassword := r.FormValue("current_password")
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_password")

		if newPassword != confirmPassword {
			renderMyAccountPage(w, r, username, isAdmin, "New passwords do not match.", true)
			return
		}
		if len(newPassword) < 4 {
			renderMyAccountPage(w, r, username, isAdmin, "Password must be at least 4 characters.", true)
			return
		}

		var passwordHash string
		err := db.QueryRow("SELECT password FROM users WHERE username = ?", username).Scan(&passwordHash)
		if err != nil {
			renderMyAccountPage(w, r, username, isAdmin, "Failed to verify current password.", true)
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(currentPassword)); err != nil {
			renderMyAccountPage(w, r, username, isAdmin, "Current password is incorrect.", true)
			return
		}

		newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			renderMyAccountPage(w, r, username, isAdmin, "Failed to update password.", true)
			return
		}
		_, err = db.Exec("UPDATE users SET password = ? WHERE username = ?", newHash, username)
		if err != nil {
			renderMyAccountPage(w, r, username, isAdmin, "Failed to update password.", true)
			return
		}

		renderMyAccountPage(w, r, username, isAdmin, "Password updated successfully!", false)
		return
	}

	renderMyAccountPage(w, r, username, isAdmin, "", false)
}

func renderMyAccountPage(w http.ResponseWriter, r *http.Request, username string, isAdmin bool, message string, isError bool) {
	safeUser := html.EscapeString(username)

	var directory, groups string
	err := db.QueryRow("SELECT directory, groups FROM users WHERE username = ?", username).Scan(&directory, &groups)
	if err != nil {
		log.Printf("Failed to load user info: %v", err)
		RenderErrorPage(w, r, "Error", "Failed to load account information.", "/", "Back to Home", http.StatusInternalServerError)
		return
	}

	// Apps use lowercase dbName to match the actions table
	type appDef struct {
		Display, DBName, Color, Icon string
	}
	apps := []appDef{
		{"Jira", "jira", "#0052CC", "&#x26A1;"},
		{"Confluence", "confluence", "#1868DB", "&#x1F4D6;"},
		{"Bitbucket", "bitbucket", "#0052CC", "&#x2692;"},
	}

	permissionsHTML := ""
	if isAdmin {
		permissionsHTML = `<div style="padding:16px; background:rgba(0,135,90,0.06); border:1px solid rgba(0,135,90,0.15); border-radius:8px; font-size:14px; color:#00875A;">
			<strong>Administrator</strong> — Full access to all actions across all applications.
		</div>`
	} else {
		for _, app := range apps {
			allActions, err := GetAvailableActionsForApp(app.DBName)
			if err != nil || len(allActions) == 0 {
				continue
			}

			// Get user's granted actions for this app
			userActions := make(map[string]bool)
			rows, err := db.Query(`
				SELECT a.action FROM group_actions ga
				JOIN actions a ON ga.action_id = a.id
				WHERE ga.group_name = ? AND a.app = ?`, groups, app.DBName)
			if err == nil {
				for rows.Next() {
					var action string
					if err := rows.Scan(&action); err == nil {
						userActions[action] = true
					}
				}
				rows.Close()
			}

			// Skip apps where user has zero permissions
			if len(userActions) == 0 {
				continue
			}

			actionsHTML := ""
			for _, a := range allActions {
				actionName := a["action"]
				hasAccess := userActions[actionName]
				if !hasAccess {
					continue // Only show granted permissions
				}
				actionsHTML += fmt.Sprintf(`
					<div style="display:flex; align-items:center; gap:8px; padding:8px 12px; background:rgba(0,135,90,0.08); border-radius:6px; margin-bottom:4px;">
						<span style="color:#00875A; font-size:14px;">&#x2714;</span>
						<span style="font-size:13px; color:var(--color-text);">%s</span>
					</div>`, html.EscapeString(actionName))
			}

			permissionsHTML += fmt.Sprintf(`
				<div style="background:var(--color-bg-card); border:1px solid var(--color-border); border-radius:8px; overflow:hidden;">
					<div style="padding:12px 16px; background:linear-gradient(135deg, %s, %scc); display:flex; align-items:center; gap:10px;">
						<span style="font-size:18px; color:white;">%s</span>
						<span style="font-weight:600; font-size:14px; color:white;">%s</span>
					</div>
					<div style="padding:12px;">%s</div>
				</div>`, app.Color, app.Color, app.Icon, app.Display, actionsHTML)
		}

		if permissionsHTML == "" {
			permissionsHTML = `<div style="text-align:center; padding:20px; color:var(--color-text-subtle);">No permissions assigned to your group.</div>`
		}
	}

	messageHTML := ""
	if message != "" {
		bannerClass := "ads-banner-success"
		if isError {
			bannerClass = "ads-banner-warning"
		}
		messageHTML = fmt.Sprintf(`<div class="ads-banner %s" style="margin-bottom:20px;">%s</div>`, bannerClass, html.EscapeString(message))
	}

	roleLabel := "Member"
	roleClass := "ads-lozenge-info"
	if isAdmin {
		roleLabel = "Administrator"
		roleClass = "ads-lozenge-success"
	}

	initial := "?"
	if len(username) > 0 {
		initial = strings.ToUpper(string(username[0]))
	}

	content := fmt.Sprintf(`
		<div class="ads-page-centered"><div class="ads-page-content">
		<div class="ads-breadcrumbs"><a href="/">Home</a> &rarr; My Account</div>
		%s

		<div class="ads-card-flat" style="margin-top:16px;">
			<div class="ads-card-header">
				<div style="width:48px; height:48px; background:linear-gradient(135deg, #0747A6, #0065FF); border-radius:50%%; display:flex; align-items:center; justify-content:center;">
					<span style="font-size:22px; color:white;">%s</span>
				</div>
				<div style="flex:1;">
					<span class="ads-card-title" style="font-size:18px;">%s</span>
					<div style="margin-top:4px;">
						<span class="ads-lozenge %s">%s</span>
						<span style="font-size:13px; color:var(--color-text-subtle); margin-left:8px;">%s &middot; %s</span>
					</div>
				</div>
			</div>
		</div>

		<div style="display:grid; grid-template-columns:1fr 1fr; gap:20px; margin-top:20px;">
			<div>
				<div class="ads-card-flat">
					<div style="padding:20px;">
						<div style="font-size:16px; font-weight:600; margin-bottom:16px;">Permissions</div>
						<div style="display:grid; gap:12px;">%s</div>
					</div>
				</div>
			</div>
			<div>
				<div class="ads-card-flat">
					<div style="padding:20px;">
						<div style="font-size:16px; font-weight:600; margin-bottom:4px;">Change Password</div>
						<div style="font-size:13px; color:var(--color-text-subtle); margin-bottom:16px;">Update your account password</div>
						<form action="/my-account" method="POST">
							<div class="ads-form-group" style="margin-bottom:12px;">
								<label class="ads-form-label">Current Password</label>
								<input type="password" name="current_password" class="ads-input" required autocomplete="current-password">
							</div>
							<div class="ads-form-group" style="margin-bottom:12px;">
								<label class="ads-form-label">New Password</label>
								<input type="password" name="new_password" class="ads-input" required minlength="4" autocomplete="new-password">
							</div>
							<div class="ads-form-group" style="margin-bottom:16px;">
								<label class="ads-form-label">Confirm New Password</label>
								<input type="password" name="confirm_password" class="ads-input" required minlength="4" autocomplete="new-password">
							</div>
							<button type="submit" class="ads-button ads-button-primary">Update Password</button>
						</form>
					</div>
				</div>
			</div>
		</div>
		<div style="margin-top:24px;">
			<a href="/" class="ads-button ads-button-default">&larr; Back to Home</a>
		</div>
		</div></div>
	`, messageHTML, initial,
		safeUser, roleClass, roleLabel,
		html.EscapeString(directory), html.EscapeString(groups),
		permissionsHTML)

	RenderPage(w, PageData{
		Title:   "My Account",
		IsAdmin: isAdmin,
		Content: template.HTML(content),
	})
}
