package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ─── Schedule helpers ─────────────────────────────────────────────────────────

// describeSchedule returns a human-readable label for a 5-field cron expression.
func describeSchedule(expr string) string {
	expr = strings.TrimSpace(expr)
	switch expr {
	case "0 * * * *":
		return "Every hour"
	case "0 */6 * * *":
		return "Every 6 hours"
	case "0 */12 * * *":
		return "Every 12 hours"
	}
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return expr
	}
	min, hour, dom, _, dow := parts[0], parts[1], parts[2], parts[3], parts[4]
	dayNames := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

	// Daily: "0 H * * *"
	if min == "0" && dom == "*" && dow == "*" {
		h, err := strconv.Atoi(hour)
		if err == nil {
			return fmt.Sprintf("Daily at %02d:00", h)
		}
	}
	// Weekly: "0 H * * D"
	if min == "0" && dom == "*" && dow != "*" {
		h, herr := strconv.Atoi(hour)
		d, derr := strconv.Atoi(dow)
		if herr == nil && derr == nil && d >= 0 && d <= 6 {
			return fmt.Sprintf("Weekly on %s at %02d:00", dayNames[d], h)
		}
	}
	// Monthly: "0 H D * *"
	if min == "0" && dom != "*" && dow == "*" {
		h, herr := strconv.Atoi(hour)
		if herr == nil {
			return fmt.Sprintf("Monthly on day %s at %02d:00", dom, h)
		}
	}
	return expr
}

// slugify converts a name to a safe filesystem slug.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == ' ', r == '_':
			b.WriteRune('-')
		}
	}
	return b.String()
}

// extractIDFromPath pulls the last path segment as an int64.
func extractIDFromPath(path string) (int64, error) {
	parts := strings.Split(strings.TrimRight(path, "/"), "/")
	return strconv.ParseInt(parts[len(parts)-1], 10, 64)
}

// backupTypeBadge renders a small coloured badge for a backup type name.
func backupTypeBadge(bt string) string {
	colours := map[string]string{
		"database":    "#00875A",
		"eazybi":      "#00B8D9",
		"attachments": "#0052CC",
		"nfs":         "#FF991F",
		"appdata":     "#6554C0",
		"full":        "#DE350B",
	}
	c := colours[bt]
	if c == "" {
		c = "#97A0AF"
	}
	return fmt.Sprintf(
		`<span style="display:inline-block;padding:2px 8px;border-radius:12px;background:%s;color:#fff;font-size:11px;font-weight:600;margin:1px;">%s</span>`,
		c, html.EscapeString(strings.ToUpper(bt)))
}

// statusBadge renders a run-status badge.
func statusBadge(status string) string {
	colours := map[string]string{
		"success": "#00875A",
		"failed":  "#DE350B",
		"partial": "#FF991F",
		"running": "#0052CC",
	}
	c := colours[status]
	if c == "" {
		c = "#97A0AF"
	}
	label := strings.ToUpper(status)
	if status == "" {
		label = "NEVER RUN"
		c = "#97A0AF"
	}
	return fmt.Sprintf(
		`<span style="display:inline-block;padding:2px 8px;border-radius:12px;background:%s;color:#fff;font-size:11px;font-weight:600;">%s</span>`,
		c, label)
}

// ─── List policies ────────────────────────────────────────────────────────────

func HandleListPolicies(w http.ResponseWriter, r *http.Request) {
	username, _ := GetCurrentUsername(r)
	isAdmin, _ := IsAdminUser(username)

	type policyRow struct {
		BackupPolicy
		LastStatus    string
		LastStartedAt string
		NextRun       string
		DiskUsage     string
	}

	rows, err := db.Query(`
		SELECT bp.id, bp.name, bp.environment_id, COALESCE(e.name,'—'),
		       bp.schedule, bp.backup_types, bp.destination_folder,
		       bp.retention_days, bp.enabled, bp.created_at, bp.updated_at,
		       COALESCE(r.status,''), COALESCE(r.started_at,'')
		FROM backup_policies bp
		LEFT JOIN environments e ON e.id = bp.environment_id
		LEFT JOIN backup_policy_runs r ON r.id = (
		    SELECT id FROM backup_policy_runs WHERE policy_id = bp.id ORDER BY id DESC LIMIT 1
		)
		ORDER BY bp.id`)
	if err != nil {
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var policies []policyRow
	for rows.Next() {
		var p policyRow
		var typesJSON string
		var enabled int
		if err := rows.Scan(
			&p.ID, &p.Name, &p.EnvironmentID, &p.EnvironmentName,
			&p.Schedule, &typesJSON, &p.DestinationFolder,
			&p.RetentionDays, &enabled, &p.CreatedAt, &p.UpdatedAt,
			&p.LastStatus, &p.LastStartedAt,
		); err != nil {
			log.Printf("HandleListPolicies scan: %v", err)
			continue
		}
		p.Enabled = enabled == 1
		_ = json.Unmarshal([]byte(typesJSON), &p.BackupTypes)
		p.NextRun = NextRunTime(p.ID)
		p.DiskUsage = policyDiskUsage(p.DestinationFolder)
		policies = append(policies, p)
	}

	// Build environments dropdown for create/edit modal
	envRows, _ := db.Query("SELECT id, name, app, COALESCE(eazybi_dbname,'') FROM environments ORDER BY app, name")
	type envOpt struct {
		ID           int64
		Name         string
		App          string
		EazyBIDBName string
	}
	var envOpts []envOpt
	if envRows != nil {
		defer envRows.Close()
		for envRows.Next() {
			var e envOpt
			envRows.Scan(&e.ID, &e.Name, &e.App, &e.EazyBIDBName)
			envOpts = append(envOpts, e)
		}
	}

	// ── Table rows ────────────────────────────────────────────────────────────
	tableRows := ""
	if len(policies) == 0 {
		tableRows = `<tr><td colspan="9" style="text-align:center;padding:32px;color:var(--color-text-subtle);">No backup policies configured yet.</td></tr>`
	}
	for _, p := range policies {
		typeBadges := ""
		for _, bt := range p.BackupTypes {
			typeBadges += backupTypeBadge(bt)
		}

		enabledToggle := fmt.Sprintf(`
			<form method="POST" action="/cron/policies/toggle/%d" style="display:inline;">
				<button type="submit" class="ads-btn ads-btn-sm %s" title="%s" style="padding:3px 10px;font-size:12px;">
					%s
				</button>
			</form>`,
			p.ID,
			map[bool]string{true: "ads-btn-success", false: "ads-btn-default"}[p.Enabled],
			map[bool]string{true: "Click to disable", false: "Click to enable"}[p.Enabled],
			map[bool]string{true: "Enabled", false: "Disabled"}[p.Enabled],
		)

		lastRun := p.LastStartedAt
		if lastRun == "" {
			lastRun = "—"
		} else if len(lastRun) >= 16 {
			lastRun = lastRun[:16]
		}

		tableRows += fmt.Sprintf(`
		<tr>
			<td><a href="/cron/policies/logs/%d" style="font-weight:600;">%s</a></td>
			<td>%s</td>
			<td style="font-size:12px;">%s</td>
			<td>%s</td>
			<td>%s</td>
			<td style="font-size:12px;color:var(--color-text-subtle);">%s</td>
			<td style="font-size:12px;color:var(--color-text-subtle);">%s</td>
			<td>%s</td>
			<td style="white-space:nowrap;">
				<button onclick="openEditModal(%d)" class="ads-btn ads-btn-sm ads-btn-default" style="padding:3px 8px;font-size:12px;">Edit</button>
				<form method="POST" action="/cron/policies/run/%d" style="display:inline;">
					<button type="submit" class="ads-btn ads-btn-sm ads-btn-primary" style="padding:3px 8px;font-size:12px;">▶ Run</button>
				</form>
				<a href="/cron/policies/logs/%d" class="ads-btn ads-btn-sm ads-btn-default" style="padding:3px 8px;font-size:12px;">Logs</a>
				<form method="POST" action="/cron/policies/delete/%d" style="display:inline;"
				      onsubmit="return confirm('Delete policy %s? This cannot be undone.')">
					<button type="submit" class="ads-btn ads-btn-sm ads-btn-danger" style="padding:3px 8px;font-size:12px;">Delete</button>
				</form>
			</td>
		</tr>`,
			p.ID, html.EscapeString(p.Name),
			html.EscapeString(p.EnvironmentName),
			html.EscapeString(describeSchedule(p.Schedule)),
			typeBadges,
			statusBadge(p.LastStatus),
			html.EscapeString(lastRun),
			html.EscapeString(p.NextRun),
			enabledToggle,
			p.ID,
			p.ID,
			p.ID,
			p.ID,
			html.EscapeString(p.Name),
		)
	}

	// ── Environment options for modal ─────────────────────────────────────────
	envOptsHTML := `<option value="">— select environment —</option>`
	for _, e := range envOpts {
		hasEazyBI := "0"
		if e.EazyBIDBName != "" {
			hasEazyBI = "1"
		}
		envOptsHTML += fmt.Sprintf(`<option value="%d" data-app="%s" data-eazybi="%s">%s (%s)</option>`,
			e.ID, html.EscapeString(e.App), hasEazyBI,
			html.EscapeString(e.Name), html.EscapeString(e.App))
	}

	// ── Policy JSON for edit modal ─────────────────────────────────────────────
	policiesJSON, _ := json.Marshal(func() []map[string]interface{} {
		var out []map[string]interface{}
		for _, p := range policies {
			out = append(out, map[string]interface{}{
				"id":                 p.ID,
				"name":               p.Name,
				"environment_id":     p.EnvironmentID,
				"schedule":           p.Schedule,
				"backup_types":       p.BackupTypes,
				"destination_folder": p.DestinationFolder,
				"retention_days":     p.RetentionDays,
				"enabled":            p.Enabled,
			})
		}
		return out
	}())

	content := fmt.Sprintf(`
<div style="position:fixed; top:56px; left:0; right:0; z-index:99;">
	<div class="ads-settings-bar">
		<a href="/settings/users">User management</a>
		<a href="/settings/updatelicense">License</a>
		<a href="/cron/policies" class="active">Backup</a>
	</div>
</div>
<div class="ads-page-with-sidebar" style="margin-top: 100px;">
	<div class="ads-sidebar" style="top: 100px; height: calc(100vh - 100px);">
		<div class="ads-sidebar-section">
			<div class="ads-sidebar-section-title">Directory</div>
			<a href="/settings/all-users" class="ads-sidebar-item">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"></path><circle cx="9" cy="7" r="4"></circle></svg>
				Users</a>
			<a href="/settings/groups" class="ads-sidebar-item">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"></path><circle cx="9" cy="7" r="4"></circle></svg>
				Groups</a>
			<a href="/settings/local-ad" class="ads-sidebar-item">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7"></rect><rect x="14" y="3" width="7" height="7"></rect><rect x="14" y="14" width="7" height="7"></rect><rect x="3" y="14" width="7" height="7"></rect></svg>
				Local Directory</a>
		</div>
		<div class="ads-sidebar-section">
			<div class="ads-sidebar-section-title">Security</div>
			<a href="/settings/auth-methods/toggle" class="ads-sidebar-item">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="11" width="18" height="11" rx="2"></rect><path d="M7 11V7a5 5 0 0 1 10 0v4"></path></svg>
				Authentication</a>
			<a href="/settings/sso" class="ads-sidebar-item">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4"></path><polyline points="10 17 15 12 10 7"></polyline><line x1="15" y1="12" x2="3" y2="12"></line></svg>
				SAML (SSO)</a>
			<a href="/settings/user-directories" class="ads-sidebar-item">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path></svg>
				User Directories</a>
		</div>
		<div class="ads-sidebar-section">
			<div class="ads-sidebar-section-title">Backup</div>
			<a href="/cron/policies" class="ads-sidebar-item active" data-full="1">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>
				Backup Policies</a>
			<a href="/cron/cleanup" class="ads-sidebar-item" data-full="1">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"></polyline><path d="M19 6l-1 14H6L5 6"></path><path d="M10 11v6"></path><path d="M14 11v6"></path><path d="M9 6V4h6v2"></path></svg>
				Cleanup Backups</a>
		</div>
		<div class="ads-sidebar-section">
			<div class="ads-sidebar-section-title">System</div>
			<a href="/settings/updatelicense" class="ads-sidebar-item">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.78 7.78 5.5 5.5 0 0 1 7.78-7.78zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"></path></svg>
				License</a>
		</div>
	</div>
	<div class="ads-main-content">
		<div id="content-section">
			<div class="ads-page-header" style="display:flex;justify-content:space-between;align-items:center;">
				<div>
					<h1>Backup Policies</h1>
					<p class="ads-page-header-description">Scheduled backups run automatically via cron. All times are server-local.</p>
				</div>
				<button onclick="openCreateModal()" class="ads-btn ads-btn-primary">
					<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
					New Policy
				</button>
			</div>
			<div class="ads-card-flat">
				<div style="overflow-x:auto;">
				<table class="ads-table">
					<thead><tr>
						<th>Policy Name</th>
						<th>Environment</th>
						<th>Schedule</th>
						<th>Backup Types</th>
						<th>Last Status</th>
						<th>Last Run</th>
						<th>Next Run</th>
						<th>State</th>
						<th>Actions</th>
					</tr></thead>
					<tbody>%s</tbody>
				</table>
				</div>
			</div>
		</div>
	</div>
</div>

<!-- ── Create/Edit Modal ───────────────────────────────────────────────────── -->
<div id="policy-modal" style="display:none;position:fixed;inset:0;background:rgba(0,0,0,.5);z-index:1000;overflow-y:auto;">
	<div style="background:var(--color-bg);border-radius:8px;max-width:580px;margin:60px auto;padding:32px;position:relative;">
		<button onclick="closeModal()" style="position:absolute;top:16px;right:16px;background:none;border:none;font-size:20px;cursor:pointer;color:var(--color-text-subtle);">&times;</button>
		<h2 id="modal-title" style="margin-bottom:24px;">New Backup Policy</h2>
		<form id="policy-form" method="POST">
			<input type="hidden" id="form-policy-id" name="id" value="">

			<div class="ads-form-group" style="margin-bottom:16px;">
				<label class="ads-form-label">Policy Name</label>
				<input class="ads-input" type="text" name="name" id="f-name" required placeholder="e.g. Nightly DB Backup">
			</div>

			<div class="ads-form-group" style="margin-bottom:16px;">
				<label class="ads-form-label">Environment</label>
				<select class="ads-input" name="environment_id" id="f-env" required>%s</select>
			</div>

			<div class="ads-form-group" style="margin-bottom:16px;">
				<label class="ads-form-label">Schedule</label>
				<select class="ads-input" id="sched-preset" onchange="applyPreset(this.value)" style="margin-bottom:8px;">
					<option value="">— preset —</option>
					<option value="0 * * * *">Every hour</option>
					<option value="0 */6 * * *">Every 6 hours</option>
					<option value="0 */12 * * *">Every 12 hours</option>
					<option value="0 2 * * *">Daily at 02:00</option>
					<option value="0 2 * * 0">Weekly on Sunday at 02:00</option>
					<option value="0 2 1 * *">Monthly on day 1 at 02:00</option>
				</select>
				<input class="ads-input" type="text" name="schedule" id="f-schedule" required
				       placeholder="cron: minute hour day month weekday"
				       oninput="updateSchedDesc(this.value)" style="margin-bottom:4px;">
				<div id="sched-desc" style="font-size:12px;color:var(--color-text-subtle);min-height:18px;"></div>
			</div>

			<div class="ads-form-group" style="margin-bottom:16px;">
				<label class="ads-form-label">Backup Types</label>
				<div style="display:flex;flex-wrap:wrap;gap:10px;">
					<label style="display:flex;align-items:center;gap:6px;cursor:pointer;">
						<input type="checkbox" name="backup_types" value="database" id="bt-database"> <span id="bt-database-label">Database</span>
					</label>
					<label id="bt-eazybi-row" style="display:none;align-items:center;gap:6px;cursor:pointer;">
						<input type="checkbox" name="backup_types" value="eazybi" id="bt-eazybi"> EazyBI Database
					</label>
					<label style="display:flex;align-items:center;gap:6px;cursor:pointer;">
						<input type="checkbox" name="backup_types" value="attachments" id="bt-attachments"> Attachments
					</label>
					<label style="display:flex;align-items:center;gap:6px;cursor:pointer;">
						<input type="checkbox" name="backup_types" value="nfs" id="bt-nfs"> NFS / Shared Home
					</label>
					<label style="display:flex;align-items:center;gap:6px;cursor:pointer;">
						<input type="checkbox" name="backup_types" value="appdata" id="bt-appdata"> App Data
					</label>
					<label style="display:flex;align-items:center;gap:6px;cursor:pointer;">
						<input type="checkbox" name="backup_types" value="full" id="bt-full"
						       onchange="if(this.checked){['database','eazybi','attachments','nfs','appdata'].forEach(id=>{document.getElementById('bt-'+id).checked=false;})}"> Full (all)
					</label>
				</div>
			</div>

			<div class="ads-form-group" style="margin-bottom:16px;">
				<label class="ads-form-label">Destination Folder</label>
				<input class="ads-input" type="text" name="destination_folder" id="f-dest"
				       placeholder="/adminToolBackupDirectory/scheduled/my-policy">
				<div style="font-size:12px;color:var(--color-text-subtle);margin-top:4px;">
					Each run creates a timestamped sub-folder here.
				</div>
			</div>

			<div class="ads-form-group" style="margin-bottom:24px;">
				<label class="ads-form-label">Retention (days)</label>
				<input class="ads-input" type="number" name="retention_days" id="f-retention" value="30" min="1" max="3650">
				<div style="font-size:12px;color:var(--color-text-subtle);margin-top:4px;">
					Backup run directories older than this are automatically removed.
				</div>
			</div>

			<div style="display:flex;gap:8px;justify-content:flex-end;">
				<button type="button" onclick="closeModal()" class="ads-btn ads-btn-default">Cancel</button>
				<button type="submit" class="ads-btn ads-btn-primary" id="modal-submit">Create Policy</button>
			</div>
		</form>
	</div>
</div>
<script>
function loadContent(url) {
    var xhr = new XMLHttpRequest();
    xhr.open("GET", url, true);
    xhr.setRequestHeader("X-Requested-With", "XMLHttpRequest");
    xhr.onreadystatechange = function() {
        if (xhr.readyState === 4 && xhr.status === 200) {
            document.getElementById("content-section").innerHTML = xhr.responseText;
            var scripts = document.getElementById("content-section").querySelectorAll("script");
            scripts.forEach(function(oldScript) {
                var newScript = document.createElement("script");
                newScript.textContent = oldScript.textContent;
                oldScript.parentNode.replaceChild(newScript, oldScript);
            });
        }
    };
    xhr.send();
}
document.addEventListener("DOMContentLoaded", function() {
    document.querySelectorAll(".ads-sidebar-item").forEach(function(link) {
        link.addEventListener("click", function(event) {
            if (this.dataset.full) return;
            event.preventDefault();
            document.querySelectorAll(".ads-sidebar-item").forEach(function(l) { l.classList.remove("active"); });
            this.classList.add("active");
            loadContent(this.getAttribute("href"));
        });
    });
});
</script>`, tableRows, envOptsHTML)

	extraHead := template.HTML(fmt.Sprintf(`<script>
const POLICIES = %s;

function openCreateModal() {
    document.getElementById('modal-title').textContent = 'New Backup Policy';
    document.getElementById('modal-submit').textContent = 'Create Policy';
    document.getElementById('policy-form').action = '/cron/policies/create';
    document.getElementById('form-policy-id').value = '';
    document.getElementById('f-name').value = '';
    document.getElementById('f-env').value = '';
    document.getElementById('f-schedule').value = '';
    document.getElementById('sched-desc').textContent = '';
    document.getElementById('sched-preset').value = '';
    document.getElementById('f-dest').value = '/adminToolBackupDirectory/scheduled/';
    document.getElementById('f-retention').value = '30';
    ['database','eazybi','attachments','nfs','appdata','full'].forEach(bt => {
        const el = document.getElementById('bt-' + bt);
        if (el) el.checked = false;
    });
    updateEnvBackupTypes('');
    document.getElementById('policy-modal').style.display = 'block';
}

function openEditModal(id) {
    const p = POLICIES.find(x => x.id === id);
    if (!p) return;
    document.getElementById('modal-title').textContent = 'Edit Policy';
    document.getElementById('modal-submit').textContent = 'Save Changes';
    document.getElementById('policy-form').action = '/cron/policies/update/' + id;
    document.getElementById('form-policy-id').value = id;
    document.getElementById('f-name').value = p.name;
    document.getElementById('f-env').value = p.environment_id;
    document.getElementById('f-schedule').value = p.schedule;
    updateSchedDesc(p.schedule);
    document.getElementById('f-dest').value = p.destination_folder;
    document.getElementById('f-retention').value = p.retention_days;
    ['database','eazybi','attachments','nfs','appdata','full'].forEach(bt => {
        const el = document.getElementById('bt-' + bt);
        if (el) el.checked = (p.backup_types || []).indexOf(bt) >= 0;
    });
    updateEnvBackupTypes(p.environment_id);
    document.getElementById('policy-modal').style.display = 'block';
}

function closeModal() {
    document.getElementById('policy-modal').style.display = 'none';
}

function applyPreset(v) {
    if (v) {
        document.getElementById('f-schedule').value = v;
        updateSchedDesc(v);
    }
}

function updateSchedDesc(v) {
    // Very light client-side schedule description
    const map = {
        '0 * * * *':     'Every hour',
        '0 */6 * * *':   'Every 6 hours',
        '0 */12 * * *':  'Every 12 hours',
        '0 2 * * *':     'Daily at 02:00',
        '0 2 * * 0':     'Weekly on Sunday at 02:00',
        '0 2 1 * *':     'Monthly on day 1 at 02:00',
    };
    const desc = map[v.trim()] || (v ? '(custom expression)' : '');
    document.getElementById('sched-desc').textContent = desc;
}

function updateEnvBackupTypes(envID) {
    const sel = document.getElementById('f-env');
    const opt = sel ? sel.querySelector('option[value="' + envID + '"]') : null;
    const app = opt ? (opt.dataset.app || '') : '';
    const hasEazyBI = opt ? (opt.dataset.eazybi === '1') : false;

    // Update database label to show app name
    const dbLabel = document.getElementById('bt-database-label');
    if (dbLabel) {
        const appName = app ? app.charAt(0).toUpperCase() + app.slice(1).toLowerCase() : 'App';
        dbLabel.textContent = appName + ' Database';
    }

    // Show/hide EazyBI row
    const eazybiRow = document.getElementById('bt-eazybi-row');
    if (eazybiRow) {
        eazybiRow.style.display = hasEazyBI ? 'flex' : 'none';
        if (!hasEazyBI) {
            const el = document.getElementById('bt-eazybi');
            if (el) el.checked = false;
        }
    }
}

// Auto-suggest destination folder from policy name
document.addEventListener('DOMContentLoaded', function() {
    document.getElementById('f-env').addEventListener('change', function() {
        updateEnvBackupTypes(this.value);
    });
    document.getElementById('f-name').addEventListener('input', function() {
        const dest = document.getElementById('f-dest');
        if (!dest.dataset.userEdited) {
            dest.value = '/adminToolBackupDirectory/scheduled/' + slugify(this.value);
        }
    });
    document.getElementById('f-dest').addEventListener('input', function() {
        this.dataset.userEdited = '1';
    });

    // Close modal on background click
    document.getElementById('policy-modal').addEventListener('click', function(e) {
        if (e.target === this) closeModal();
    });
});

function slugify(s) {
    return s.toLowerCase().replace(/[^a-z0-9-]/g, c => c === ' ' || c === '_' ? '-' : '').replace(/-+/g, '-');
}
</script>`, string(policiesJSON)))

	RenderPage(w, PageData{
		Title:     "Backup Policies",
		IsAdmin:   isAdmin,
		ExtraHead: extraHead,
		Content:   template.HTML(content),
	})
}

// ─── Create policy ────────────────────────────────────────────────────────────

func HandleCreatePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/cron/policies", http.StatusSeeOther)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	envIDStr := r.FormValue("environment_id")
	schedule := strings.TrimSpace(r.FormValue("schedule"))
	dest := strings.TrimSpace(r.FormValue("destination_folder"))
	retention, _ := strconv.Atoi(r.FormValue("retention_days"))
	if retention <= 0 {
		retention = 30
	}
	if dest == "" {
		dest = "/adminToolBackupDirectory/scheduled/" + slugify(name)
	}

	bts := r.Form["backup_types"]
	typesJSON, _ := json.Marshal(bts)

	envID, _ := strconv.ParseInt(envIDStr, 10, 64)

	_, err := db.Exec(`
		INSERT INTO backup_policies (name, environment_id, schedule, backup_types, destination_folder, retention_days, enabled)
		VALUES (?, ?, ?, ?, ?, ?, 1)`,
		name, envID, schedule, string(typesJSON), dest, retention)
	if err != nil {
		log.Printf("HandleCreatePolicy: %v", err)
		http.Error(w, "Failed to create policy: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Schedule the new policy
	var newID int64
	db.QueryRow("SELECT id FROM backup_policies WHERE name = ? ORDER BY id DESC LIMIT 1", name).Scan(&newID)
	if newID > 0 {
		schedulePolicy(newID, schedule)
	}

	http.Redirect(w, r, "/cron/policies", http.StatusSeeOther)
}

// ─── Update policy ────────────────────────────────────────────────────────────

func HandleUpdatePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/cron/policies", http.StatusSeeOther)
		return
	}

	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid policy ID", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	envIDStr := r.FormValue("environment_id")
	schedule := strings.TrimSpace(r.FormValue("schedule"))
	dest := strings.TrimSpace(r.FormValue("destination_folder"))
	retention, _ := strconv.Atoi(r.FormValue("retention_days"))
	if retention <= 0 {
		retention = 30
	}

	bts := r.Form["backup_types"]
	typesJSON, _ := json.Marshal(bts)
	envID, _ := strconv.ParseInt(envIDStr, 10, 64)

	_, err = db.Exec(`
		UPDATE backup_policies
		SET name=?, environment_id=?, schedule=?, backup_types=?,
		    destination_folder=?, retention_days=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?`,
		name, envID, schedule, string(typesJSON), dest, retention, id)
	if err != nil {
		log.Printf("HandleUpdatePolicy: %v", err)
		http.Error(w, "Failed to update policy: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Re-schedule with new expression
	schedulePolicy(id, schedule)

	http.Redirect(w, r, "/cron/policies", http.StatusSeeOther)
}

// ─── Delete policy ────────────────────────────────────────────────────────────

func HandleDeletePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/cron/policies", http.StatusSeeOther)
		return
	}

	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid policy ID", http.StatusBadRequest)
		return
	}

	unschedulePolicy(id)

	if _, err := db.Exec("DELETE FROM backup_policies WHERE id = ?", id); err != nil {
		log.Printf("HandleDeletePolicy: %v", err)
		http.Error(w, "Failed to delete policy", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/cron/policies", http.StatusSeeOther)
}

// ─── Toggle policy enabled/disabled ──────────────────────────────────────────

func HandleTogglePolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/cron/policies", http.StatusSeeOther)
		return
	}

	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid policy ID", http.StatusBadRequest)
		return
	}

	var enabled int
	var schedule string
	err = db.QueryRow("SELECT enabled, schedule FROM backup_policies WHERE id = ?", id).
		Scan(&enabled, &schedule)
	if err != nil {
		http.Error(w, "Policy not found", http.StatusNotFound)
		return
	}

	newEnabled := 1
	if enabled == 1 {
		newEnabled = 0
	}

	db.Exec("UPDATE backup_policies SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		newEnabled, id)

	if newEnabled == 1 {
		schedulePolicy(id, schedule)
	} else {
		unschedulePolicy(id)
	}

	http.Redirect(w, r, "/cron/policies", http.StatusSeeOther)
}

// ─── Manual trigger ───────────────────────────────────────────────────────────

func HandleRunNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/cron/policies", http.StatusSeeOther)
		return
	}

	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid policy ID", http.StatusBadRequest)
		return
	}

	// Verify policy exists
	var name string
	if err := db.QueryRow("SELECT name FROM backup_policies WHERE id = ?", id).Scan(&name); err != nil {
		http.Error(w, "Policy not found", http.StatusNotFound)
		return
	}

	runID, err := createPolicyRun(id)
	if err != nil {
		http.Error(w, "Failed to start run: "+err.Error(), http.StatusInternalServerError)
		return
	}
	go runPolicyCore(id, runID)

	http.Redirect(w, r, fmt.Sprintf("/cron/policies/logs/%d?live=%d", id, runID), http.StatusSeeOther)
}

// ─── Live log endpoint ────────────────────────────────────────────────────────

func HandleLogLive(w http.ResponseWriter, r *http.Request) {
	runID, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}
	var status, logText, startedAt, finishedAt string
	var sizeBytes int64
	if err := db.QueryRow(`SELECT status, log, COALESCE(started_at,''), COALESCE(finished_at,''), backup_size_bytes FROM backup_policy_runs WHERE id = ?`, runID).
		Scan(&status, &logText, &startedAt, &finishedAt, &sizeBytes); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"","log":"","started_at":"","finished_at":"","size_bytes":0}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      status,
		"log":         logText,
		"started_at":  startedAt,
		"finished_at": finishedAt,
		"size_bytes":  sizeBytes,
	})
}

// ─── Logs (run history for a policy) ─────────────────────────────────────────

func HandleGetLogs(w http.ResponseWriter, r *http.Request) {
	username, _ := GetCurrentUsername(r)
	isAdmin, _ := IsAdminUser(username)

	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid policy ID", http.StatusBadRequest)
		return
	}

	var policyName string
	err = db.QueryRow("SELECT name FROM backup_policies WHERE id = ?", id).Scan(&policyName)
	if err == sql.ErrNoRows {
		http.Error(w, "Policy not found", http.StatusNotFound)
		return
	}

	rows, err := db.Query(`
		SELECT id, started_at, COALESCE(finished_at,''), status, backup_size_bytes, log
		FROM backup_policy_runs
		WHERE policy_id = ?
		ORDER BY id DESC
		LIMIT 50`, id)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	tableRows := ""
	firstLog := ""
	firstRunID := int64(0)
	hasRunning := false
	for rows.Next() {
		var run BackupPolicyRun
		var sizeBytes int64
		if err := rows.Scan(&run.ID, &run.StartedAt, &run.FinishedAt,
			&run.Status, &sizeBytes, &run.Log); err != nil {
			continue
		}

		duration := "—"
		if run.StartedAt != "" && run.FinishedAt != "" {
			t1, e1 := time.Parse("2006-01-02T15:04:05Z", run.StartedAt)
			t2, e2 := time.Parse("2006-01-02T15:04:05Z", run.FinishedAt)
			if e1 != nil {
				t1, e1 = time.Parse("2006-01-02 15:04:05", run.StartedAt)
			}
			if e2 != nil {
				t2, e2 = time.Parse("2006-01-02 15:04:05", run.FinishedAt)
			}
			if e1 == nil && e2 == nil {
				dur := t2.Sub(t1)
				if dur < time.Minute {
					duration = fmt.Sprintf("%ds", int(dur.Seconds()))
				} else {
					duration = fmt.Sprintf("%dm%ds", int(dur.Minutes()), int(dur.Seconds())%60)
				}
			}
		}

		started := run.StartedAt
		if len(started) >= 16 {
			started = started[:16]
		}

		sizeStr := "—"
		if sizeBytes > 0 {
			sizeStr = policyDiskUsage("") // can't use helper, compute inline
			sizeStr = fmt.Sprintf("%d B", sizeBytes)
			if sizeBytes > 1024*1024*1024 {
				sizeStr = fmt.Sprintf("%.1f GB", float64(sizeBytes)/1024/1024/1024)
			} else if sizeBytes > 1024*1024 {
				sizeStr = fmt.Sprintf("%.1f MB", float64(sizeBytes)/1024/1024)
			} else if sizeBytes > 1024 {
				sizeStr = fmt.Sprintf("%.1f KB", float64(sizeBytes)/1024)
			}
		}

		if firstLog == "" {
			firstLog = run.Log
			firstRunID = run.ID
		}
		if run.Status == "running" {
			hasRunning = true
		}

		tableRows += fmt.Sprintf(`
		<tr id="run-row-%d">
			<td style="font-family:monospace;font-size:12px;">#%d</td>
			<td style="font-size:12px;">%s</td>
			<td id="run-status-%d">%s</td>
			<td id="run-duration-%d" style="font-size:12px;">%s</td>
			<td id="run-size-%d" style="font-size:12px;">%s</td>
			<td>
				<a href="/cron/policies/runs/%d" class="ads-btn ads-btn-sm ads-btn-default" style="padding:2px 8px;font-size:11px;">Detail</a>
				<button onclick="showLog(%d)" class="ads-btn ads-btn-sm ads-btn-default" style="padding:2px 8px;font-size:11px;">Log</button>
			</td>
		</tr>`,
			run.ID, run.ID, html.EscapeString(started),
			run.ID, statusBadge(run.Status),
			run.ID, html.EscapeString(duration),
			run.ID, html.EscapeString(sizeStr),
			run.ID, run.ID,
		)
	}

	if tableRows == "" {
		tableRows = `<tr><td colspan="6" style="text-align:center;padding:32px;color:var(--color-text-subtle);">No runs yet.</td></tr>`
	}

	// Encode all logs as JSON for client-side display
	allLogs := map[int64]string{}
	rows2, _ := db.Query("SELECT id, log FROM backup_policy_runs WHERE policy_id = ? ORDER BY id DESC LIMIT 50", id)
	if rows2 != nil {
		defer rows2.Close()
		for rows2.Next() {
			var rid int64
			var lg string
			rows2.Scan(&rid, &lg)
			allLogs[rid] = lg
		}
	}
	logsJSON, _ := json.Marshal(allLogs)
	_ = firstRunID

	autoReload := ""
	if hasRunning {
		autoReload = `<script>setTimeout(()=>location.reload(),3000)</script>`
	}

	content := fmt.Sprintf(`
<div class="ads-page-centered"><div class="ads-page-content">
	<div class="ads-breadcrumbs">
		<a href="/cron/policies">Backup Policies</a> &rarr; Logs: %s
	</div>
	<div class="ads-page-header" style="display:flex;justify-content:space-between;align-items:center;">
		<h1>Run History</h1>
		<div style="display:flex;gap:8px;">
			<form method="POST" action="/cron/policies/run/%d">
				<button type="submit" class="ads-btn ads-btn-primary">▶ Run Now</button>
			</form>
			<a href="/cron/policies" class="ads-btn ads-btn-default">&larr; All Policies</a>
		</div>
	</div>

	<div class="ads-card-flat" style="margin-bottom:24px;">
		<div style="overflow-x:auto;">
		<table class="ads-table">
			<thead><tr>
				<th>Run #</th><th>Started</th><th>Status</th><th>Duration</th><th>Size</th><th>Actions</th>
			</tr></thead>
			<tbody>%s</tbody>
		</table>
		</div>
	</div>

	<div class="ads-card-flat" id="log-panel" style="display:%s;">
		<div class="ads-card-header">
			<span style="font-weight:600;">Log Output</span>
			<span id="log-run-label" style="font-size:12px;color:var(--color-text-subtle);margin-left:12px;"></span>
		</div>
		<pre id="log-output" style="margin:0;padding:16px 24px;font-size:12px;font-family:monospace;background:var(--color-bg-card);
		     max-height:400px;overflow-y:auto;white-space:pre-wrap;word-break:break-all;">%s</pre>
	</div>

</div></div>%s`,
		html.EscapeString(policyName),
		id,
		tableRows,
		map[bool]string{true: "block", false: "none"}[firstLog != ""],
		html.EscapeString(firstLog),
		template.HTML(autoReload),
	)

	extraHead := template.HTML(fmt.Sprintf(`<script>
const ALL_LOGS = %s;
let activeRunID = 0;

function showLog(runID) {
    activeRunID = runID;
    const log = ALL_LOGS[runID] || '';
    document.getElementById('log-output').textContent = log || '(no log yet)';
    document.getElementById('log-run-label').textContent = 'Run #' + runID;
    document.getElementById('log-panel').style.display = 'block';
    document.getElementById('log-panel').scrollIntoView({behavior:'smooth'});
    // Start live polling for the selected run regardless of how page was opened
    pollLog(runID);
}

// On page load: open the most recent run's log and start polling it
document.addEventListener('DOMContentLoaded', function() {
    const params = new URLSearchParams(window.location.search);
    const liveID = params.get('live');
    if (liveID) {
        showLog(parseInt(liveID));
    } else {
        const firstBtn = document.querySelector('button[onclick^="showLog"]');
        if (firstBtn) {
            const m = firstBtn.getAttribute('onclick').match(/showLog\((\d+)\)/);
            if (m) showLog(parseInt(m[1]));
        }
    }
});

function statusBadgeHTML(status) {
    const colours = {success:'#00875A', failed:'#DE350B', partial:'#FF991F', running:'#0052CC'};
    const c = colours[status] || '#97A0AF';
    const label = status ? status.toUpperCase() : 'NEVER RUN';
    return '<span style="display:inline-block;padding:2px 8px;border-radius:12px;background:' + c + ';color:#fff;font-size:11px;font-weight:600;">' + label + '</span>';
}

function fmtDuration(startedAt, finishedAt) {
    if (!startedAt || !finishedAt) return finishedAt ? '' : 'running…';
    const d = Math.round((new Date(finishedAt) - new Date(startedAt)) / 1000);
    return d < 60 ? d + 's' : Math.floor(d/60) + 'm' + (d%%60) + 's';
}

function fmtSize(bytes) {
    if (!bytes) return '—';
    if (bytes > 1073741824) return (bytes/1073741824).toFixed(1) + ' GB';
    if (bytes > 1048576)    return (bytes/1048576).toFixed(1) + ' MB';
    if (bytes > 1024)       return (bytes/1024).toFixed(1) + ' KB';
    return bytes + ' B';
}

function pollLog(runID) {
    if (activeRunID !== runID) return;
    fetch('/cron/policies/log-live/' + runID)
        .then(r => r.json())
        .then(data => {
            if (activeRunID !== runID) return;
            // Update log panel
            const pre = document.getElementById('log-output');
            if (pre) {
                pre.textContent = data.log || '(no log yet)';
                pre.scrollTop = pre.scrollHeight;
            }
            document.getElementById('log-panel').style.display = 'block';
            // Update table row cells in-place
            const statusEl = document.getElementById('run-status-' + runID);
            if (statusEl) statusEl.innerHTML = statusBadgeHTML(data.status);
            const durEl = document.getElementById('run-duration-' + runID);
            if (durEl) durEl.textContent = fmtDuration(data.started_at, data.finished_at);
            const sizeEl = document.getElementById('run-size-' + runID);
            if (sizeEl) sizeEl.textContent = fmtSize(data.size_bytes);

            if (data.status === 'running') {
                setTimeout(() => pollLog(runID), 2000);
            }
            // No page reload needed — table is already updated in-place
        })
        .catch(() => {
            if (activeRunID === runID) setTimeout(() => pollLog(runID), 3000);
        });
}


</script>`, string(logsJSON)))

	RenderPage(w, PageData{
		Title:     "Logs — " + policyName,
		IsAdmin:   isAdmin,
		ExtraHead: extraHead,
		Content:   template.HTML(content),
	})
}

// ─── Run detail ───────────────────────────────────────────────────────────────

func HandleGetRunDetail(w http.ResponseWriter, r *http.Request) {
	username, _ := GetCurrentUsername(r)
	isAdmin, _ := IsAdminUser(username)

	runID, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid run ID", http.StatusBadRequest)
		return
	}

	var run BackupPolicyRun
	var filesJSON string
	var sizeBytes int64
	err = db.QueryRow(`
		SELECT r.id, r.policy_id, COALESCE(p.name,''), r.started_at,
		       COALESCE(r.finished_at,''), r.status, r.log,
		       r.backup_size_bytes, r.files_created
		FROM backup_policy_runs r
		LEFT JOIN backup_policies p ON p.id = r.policy_id
		WHERE r.id = ?`, runID).
		Scan(&run.ID, &run.PolicyID, &run.PolicyName,
			&run.StartedAt, &run.FinishedAt, &run.Status,
			&run.Log, &sizeBytes, &filesJSON)
	if err == sql.ErrNoRows {
		http.Error(w, "Run not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "DB error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_ = json.Unmarshal([]byte(filesJSON), &run.FilesCreated)

	sizeStr := "0 B"
	if sizeBytes > 1024*1024*1024 {
		sizeStr = fmt.Sprintf("%.2f GB", float64(sizeBytes)/1024/1024/1024)
	} else if sizeBytes > 1024*1024 {
		sizeStr = fmt.Sprintf("%.2f MB", float64(sizeBytes)/1024/1024)
	} else if sizeBytes > 1024 {
		sizeStr = fmt.Sprintf("%.2f KB", float64(sizeBytes)/1024)
	} else if sizeBytes > 0 {
		sizeStr = fmt.Sprintf("%d B", sizeBytes)
	}

	filesList := ""
	for _, f := range run.FilesCreated {
		filesList += fmt.Sprintf(`<li style="font-family:monospace;font-size:12px;">%s</li>`, html.EscapeString(f))
	}
	if filesList == "" {
		filesList = `<li style="color:var(--color-text-subtle);">No files recorded.</li>`
	}

	started := run.StartedAt
	if len(started) >= 16 {
		started = started[:16]
	}
	finished := run.FinishedAt
	if len(finished) >= 16 {
		finished = finished[:16]
	}
	if finished == "" {
		finished = "—"
	}

	content := fmt.Sprintf(`
<div class="ads-page-centered"><div class="ads-page-content">
	<div class="ads-breadcrumbs">
		<a href="/cron/policies">Backup Policies</a> &rarr;
		<a href="/cron/policies/logs/%d">%s</a> &rarr;
		Run #%d
	</div>
	<div class="ads-page-header">
		<h1>Run Detail #%d</h1>
	</div>

	<div class="ads-card-flat" style="margin-bottom:16px;">
		<div style="display:grid;grid-template-columns:repeat(4,1fr);gap:16px;padding:16px 24px;">
			<div><div style="font-size:11px;color:var(--color-text-subtle);text-transform:uppercase;margin-bottom:4px;">Status</div>%s</div>
			<div><div style="font-size:11px;color:var(--color-text-subtle);text-transform:uppercase;margin-bottom:4px;">Started</div><span style="font-size:13px;">%s</span></div>
			<div><div style="font-size:11px;color:var(--color-text-subtle);text-transform:uppercase;margin-bottom:4px;">Finished</div><span style="font-size:13px;">%s</span></div>
			<div><div style="font-size:11px;color:var(--color-text-subtle);text-transform:uppercase;margin-bottom:4px;">Total Size</div><span style="font-size:13px;font-weight:600;">%s</span></div>
		</div>
	</div>

	<div style="display:grid;grid-template-columns:1fr 1fr;gap:16px;margin-bottom:16px;">
		<div class="ads-card-flat">
			<div class="ads-card-header"><span class="ads-card-title">Files Created</span></div>
			<ul style="padding:8px 24px 16px;margin:0;list-style:none;">%s</ul>
		</div>
	</div>

	<div class="ads-card-flat">
		<div class="ads-card-header">
			<span class="ads-card-title">Full Log</span>
			<a href="/cron/policies/logs/%d" class="ads-btn ads-btn-sm ads-btn-default" style="padding:3px 10px;font-size:12px;">← Back</a>
		</div>
		<pre style="margin:0;padding:16px 24px;font-size:12px;font-family:monospace;background:var(--color-bg-card);
		     max-height:600px;overflow-y:auto;white-space:pre-wrap;word-break:break-all;">%s</pre>
	</div>
</div></div>`,
		run.PolicyID, html.EscapeString(run.PolicyName),
		run.ID, run.ID,
		statusBadge(run.Status),
		html.EscapeString(started),
		html.EscapeString(finished),
		html.EscapeString(sizeStr),
		filesList,
		run.PolicyID,
		html.EscapeString(run.Log),
	)

	RenderPage(w, PageData{
		Title:   fmt.Sprintf("Run #%d — %s", run.ID, run.PolicyName),
		IsAdmin: isAdmin,
		Content: template.HTML(content),
	})
}

// ─── Cleanup helpers ──────────────────────────────────────────────────────────

var runFolderRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}_\d{2}-\d{2}-\d{2}$`)

const safeRoot = "/adminToolBackupDirectory/scheduled"

// validateCleanupPath checks that target is inside the policy's destination
// folder and that the destination folder itself lives under safeRoot.
func validateCleanupPath(destFolder, target string) (string, error) {
	cleanDest := filepath.Clean(destFolder)
	if !strings.HasPrefix(cleanDest, safeRoot) {
		return "", fmt.Errorf("destination folder outside allowed root")
	}
	full := filepath.Clean(filepath.Join(cleanDest, target))
	if !strings.HasPrefix(full, cleanDest+"/") {
		return "", fmt.Errorf("path traversal detected")
	}
	return full, nil
}

// listRunFolders returns all timestamp-named subdirs inside destFolder.
func listRunFolders(destFolder string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(destFolder)
	if err != nil {
		return nil, err
	}
	var out []os.DirEntry
	for _, e := range entries {
		if e.IsDir() && runFolderRe.MatchString(e.Name()) {
			out = append(out, e)
		}
	}
	return out, nil
}

// HandleCleanupInfo returns JSON: [{folder, size_bytes}, …] for a policy.
// GET /cron/policies/cleanup-info/{id}
func HandleCleanupInfo(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid policy id", http.StatusBadRequest)
		return
	}
	var destFolder string
	if err = db.QueryRow("SELECT destination_folder FROM backup_policies WHERE id=?", id).Scan(&destFolder); err != nil {
		http.Error(w, "policy not found", http.StatusNotFound)
		return
	}
	entries, _ := listRunFolders(destFolder)
	type item struct {
		Folder    string `json:"folder"`
		SizeBytes int64  `json:"size_bytes"`
	}
	result := make([]item, 0, len(entries))
	for _, e := range entries {
		full := filepath.Join(destFolder, e.Name())
		result = append(result, item{Folder: e.Name(), SizeBytes: calculateDirSize(full)})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleDeleteRun deletes one run folder.
// POST /cron/policies/delete-run/{id}   body: {"folder":"2026-03-26_21-37-56"}
func HandleDeleteRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid policy id", http.StatusBadRequest)
		return
	}
	var destFolder string
	if err = db.QueryRow("SELECT destination_folder FROM backup_policies WHERE id=?", id).Scan(&destFolder); err != nil {
		http.Error(w, "policy not found", http.StatusNotFound)
		return
	}
	var body struct {
		Folder string `json:"folder"`
	}
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil || !runFolderRe.MatchString(body.Folder) {
		http.Error(w, "invalid folder name", http.StatusBadRequest)
		return
	}
	fullPath, err := validateCleanupPath(destFolder, body.Folder)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	freed := calculateDirSize(fullPath)
	if err = os.RemoveAll(fullPath); err != nil {
		http.Error(w, "delete failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"freed_bytes": freed, "success": true})
}

// HandleDeleteAllRuns deletes every run folder inside a policy's destination.
// POST /cron/policies/delete-all-runs/{id}
func HandleDeleteAllRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid policy id", http.StatusBadRequest)
		return
	}
	var destFolder string
	if err = db.QueryRow("SELECT destination_folder FROM backup_policies WHERE id=?", id).Scan(&destFolder); err != nil {
		http.Error(w, "policy not found", http.StatusNotFound)
		return
	}
	cleanDest := filepath.Clean(destFolder)
	if !strings.HasPrefix(cleanDest, safeRoot) {
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}
	entries, _ := listRunFolders(destFolder)
	var freed int64
	deleted := 0
	for _, e := range entries {
		full := filepath.Join(cleanDest, e.Name())
		freed += calculateDirSize(full)
		os.RemoveAll(full)
		deleted++
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"freed_bytes": freed, "deleted": deleted, "success": true})
}

// HandleDeleteOlderThan deletes run folders older than N days.
// POST /cron/policies/delete-older-than/{id}   body: {"days":30}
func HandleDeleteOlderThan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := extractIDFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "invalid policy id", http.StatusBadRequest)
		return
	}
	var destFolder string
	if err = db.QueryRow("SELECT destination_folder FROM backup_policies WHERE id=?", id).Scan(&destFolder); err != nil {
		http.Error(w, "policy not found", http.StatusNotFound)
		return
	}
	cleanDest := filepath.Clean(destFolder)
	if !strings.HasPrefix(cleanDest, safeRoot) {
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}
	var body struct {
		Days int `json:"days"`
	}
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil || body.Days < 1 {
		http.Error(w, "invalid days value", http.StatusBadRequest)
		return
	}
	cutoff := time.Now().AddDate(0, 0, -body.Days)
	entries, _ := listRunFolders(destFolder)
	var freed int64
	deleted := 0
	for _, e := range entries {
		t, parseErr := time.Parse("2006-01-02_15-04-05", e.Name())
		if parseErr != nil {
			continue
		}
		if t.Before(cutoff) {
			full := filepath.Join(cleanDest, e.Name())
			freed += calculateDirSize(full)
			os.RemoveAll(full)
			deleted++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"freed_bytes": freed, "deleted": deleted, "success": true})
}
