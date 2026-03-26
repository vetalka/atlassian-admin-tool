package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HandleShowCleanup renders GET /cron/cleanup — DB-only, zero disk access.
func HandleShowCleanup(w http.ResponseWriter, r *http.Request) {
	username, _ := GetCurrentUsername(r)
	isAdmin, _ := IsAdminUser(username)

	type runRecord struct {
		ID        int64
		StartedAt string
		Status    string
		SizeBytes int64
	}
	type policyCleanup struct {
		ID       int64
		Name     string
		Records  []runRecord
		TotalMB  float64
	}

	pRows, err := db.Query("SELECT id, name FROM backup_policies ORDER BY name")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer pRows.Close()

	var policies []policyCleanup
	for pRows.Next() {
		var p policyCleanup
		pRows.Scan(&p.ID, &p.Name)

		rRows, err := db.Query(`
			SELECT id, COALESCE(started_at,''), COALESCE(status,''), COALESCE(backup_size_bytes,0)
			FROM backup_policy_runs WHERE policy_id=? ORDER BY started_at DESC`, p.ID)
		if err == nil {
			for rRows.Next() {
				var rec runRecord
				rRows.Scan(&rec.ID, &rec.StartedAt, &rec.Status, &rec.SizeBytes)
				p.TotalMB += float64(rec.SizeBytes) / 1024 / 1024
				p.Records = append(p.Records, rec)
			}
			rRows.Close()
		}
		policies = append(policies, p)
	}

	cards := ""
	for _, p := range policies {
		rowsHTML := ""
		for _, rec := range p.Records {
			started := rec.StartedAt
			if len(started) > 16 {
				started = started[:16]
			}
			sizeStr := "—"
			if rec.SizeBytes > 0 {
				sizeStr = fmtBytesCleanup(rec.SizeBytes)
			}
			statusColor := map[string]string{
				"success": "#00875A", "failed": "#DE350B",
				"partial": "#FF991F", "running": "#0052CC",
			}
			sc := statusColor[rec.Status]
			if sc == "" {
				sc = "#97A0AF"
			}
			statusBadge := fmt.Sprintf(
				`<span style="display:inline-block;padding:2px 8px;border-radius:12px;background:%s;color:#fff;font-size:11px;font-weight:600;">%s</span>`,
				sc, html.EscapeString(strings.ToUpper(rec.Status)),
			)
			rowsHTML += fmt.Sprintf(`
		<tr id="rec-%d-%d">
			<td style="font-family:monospace;font-size:12px;">#%d</td>
			<td style="font-size:12px;">%s</td>
			<td>%s</td>
			<td style="font-size:12px;">%s</td>
			<td>
				<button onclick="deleteRecord(%d,%d,this)"
					style="padding:3px 10px;font-size:12px;background:#6B778C;color:#fff;border:none;border-radius:4px;cursor:pointer;">
					&#x1F5D1; Delete Record
				</button>
			</td>
		</tr>`,
				p.ID, rec.ID,
				rec.ID,
				html.EscapeString(started),
				statusBadge,
				html.EscapeString(sizeStr),
				rec.ID, p.ID,
			)
		}
		if rowsHTML == "" {
			rowsHTML = `<tr><td colspan="5" style="text-align:center;padding:24px;color:var(--color-text-subtle);">No run records.</td></tr>`
		}

		totalStr := fmt.Sprintf("%.1f MB", p.TotalMB)
		if p.TotalMB == 0 {
			totalStr = "—"
		}
		cards += fmt.Sprintf(`
<div class="ads-card-flat" style="margin-bottom:24px;" id="policy-card-%d">
	<div class="ads-card-header" style="justify-content:space-between;">
		<span style="font-weight:600;font-size:15px;">%s</span>
		<div style="display:flex;align-items:center;gap:12px;">
			<span style="font-size:13px;color:var(--color-text-subtle);">Total recorded: <strong>%s</strong></span>
			<button onclick="deleteAllRecords(%d,this)"
				style="padding:3px 12px;font-size:12px;background:#DE350B;color:#fff;border:none;border-radius:4px;cursor:pointer;">
				&#x1F5D1; Delete All Records
			</button>
		</div>
	</div>
	<div style="padding:0 24px 16px;">
		<div id="msg-%d" style="margin:8px 0;font-size:13px;display:none;"></div>
		<div style="overflow-x:auto;">
		<table class="ads-table" id="tbl-%d">
			<thead><tr>
				<th>Run #</th><th>Started</th><th>Status</th><th>Size</th><th>Actions</th>
			</tr></thead>
			<tbody>%s</tbody>
		</table>
		</div>
	</div>
</div>`,
			p.ID,
			html.EscapeString(p.Name),
			html.EscapeString(totalStr),
			p.ID,
			p.ID,
			p.ID,
			rowsHTML,
		)
	}

	if cards == "" {
		cards = `<div class="ads-card-flat" style="padding:40px;text-align:center;color:var(--color-text-subtle);">No backup policies found.</div>`
	}

	content := fmt.Sprintf(`
<div style="position:fixed; top:56px; left:0; right:0; z-index:99;">
	<div class="ads-settings-bar">
		<a href="/settings/users">User management</a>
		<a href="/settings/updatelicense">License</a>
		<a href="/cron/policies">Backup</a>
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
			<a href="/cron/policies" class="ads-sidebar-item" data-full="1">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>
				Backup Policies</a>
			<a href="/cron/cleanup" class="ads-sidebar-item active" data-full="1">
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
		<div class="ads-page-centered"><div class="ads-page-content">
			<div class="ads-page-header">
				<h1>Cleanup Backups</h1>
				<p class="ads-page-header-description">View and delete backup run records. Use backup policies page to manage schedules.</p>
			</div>
			%s
		</div></div>
	</div>
</div>
<script>
function showMsg(policyID, msg, ok) {
	const el = document.getElementById('msg-' + policyID);
	if (!el) return;
	el.textContent = msg;
	el.style.display = 'block';
	el.style.color = ok ? '#00875A' : '#DE350B';
	setTimeout(() => { el.style.display = 'none'; }, 5000);
}

function deleteRecord(runID, policyID, btn) {
	if (!confirm('Delete this run record from DB? This cannot be undone.')) return;
	btn.disabled = true;
	fetch('/cron/cleanup/delete-record-only', {
		method: 'POST',
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify({run_id: runID})
	}).then(r => r.json()).then(d => {
		if (d.success) {
			const row = document.getElementById('rec-' + policyID + '-' + runID);
			if (row) row.remove();
			showMsg(policyID, '✓ Record deleted.', true);
		} else {
			showMsg(policyID, d.error || 'Failed.', false);
			btn.disabled = false;
		}
	}).catch(() => { showMsg(policyID, 'Failed.', false); btn.disabled = false; });
}

function deleteAllRecords(policyID, btn) {
	if (!confirm('Delete ALL run records for this policy from DB? This cannot be undone.')) return;
	btn.disabled = true;
	fetch('/cron/cleanup/clear-all-records', {
		method: 'POST',
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify({policy_id: policyID})
	}).then(r => r.json()).then(d => {
		if (d.success) {
			const tbody = document.querySelector('#tbl-' + policyID + ' tbody');
			if (tbody) tbody.innerHTML = '<tr><td colspan="5" style="text-align:center;padding:24px;color:var(--color-text-subtle);">No run records.</td></tr>';
			showMsg(policyID, '✓ Deleted ' + d.deleted + ' record(s).', true);
		} else {
			showMsg(policyID, d.error || 'Failed.', false);
			btn.disabled = false;
		}
	}).catch(() => { showMsg(policyID, 'Failed.', false); btn.disabled = false; });
}
</script>`, cards)

	RenderPage(w, PageData{
		Title:   "Cleanup Backups",
		IsAdmin: isAdmin,
		Content: template.HTML(content),
	})
}

// deleteRunDBRecord removes the backup_policy_runs record whose files_created
// contains the given folder name.
func deleteRunDBRecord(policyID int64, folder string) {
	db.Exec(`DELETE FROM backup_policy_runs WHERE policy_id=? AND files_created LIKE ?`,
		policyID, "%/"+folder+"/%")
}

// HandleCleanupDeleteRun handles POST /cron/cleanup/delete-run
// Body: {"policy_id": N, "folder": "2026-03-26_21-37-56"}
func HandleCleanupDeleteRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		PolicyID int64  `json:"policy_id"`
		Folder   string `json:"folder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !runFolderRe.MatchString(body.Folder) {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	var destFolder string
	if err := db.QueryRow("SELECT destination_folder FROM backup_policies WHERE id=?", body.PolicyID).Scan(&destFolder); err != nil {
		http.Error(w, "policy not found", http.StatusNotFound)
		return
	}
	fullPath, err := validateCleanupPath(destFolder, body.Folder)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	freed := calculateDirSize(fullPath)
	if err := removeAllSafe(fullPath); err != nil {
		jsonErr(w, err.Error())
		return
	}
	deleteRunDBRecord(body.PolicyID, body.Folder)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "freed_bytes": freed})
}

// HandleCleanupDeleteAll handles POST /cron/cleanup/delete-all
// Body: {"policy_id": N}
func HandleCleanupDeleteAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		PolicyID int64 `json:"policy_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PolicyID == 0 {
		log.Printf("HandleCleanupDeleteAll: bad request: %v", err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	var destFolder string
	if err := db.QueryRow("SELECT destination_folder FROM backup_policies WHERE id=?", body.PolicyID).Scan(&destFolder); err != nil {
		log.Printf("HandleCleanupDeleteAll: policy %d not found: %v", body.PolicyID, err)
		http.Error(w, "policy not found", http.StatusNotFound)
		return
	}
	cleanDest := filepath.Clean(destFolder)
	if !strings.HasPrefix(cleanDest, safeRoot) {
		log.Printf("HandleCleanupDeleteAll: path not allowed: %s", cleanDest)
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}
	// Delete run folders on disk
	entries, _ := os.ReadDir(cleanDest)
	var freed int64
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		full := filepath.Join(cleanDest, e.Name())
		freed += calculateDirSize(full)
		if err := os.RemoveAll(full); err != nil {
			log.Printf("HandleCleanupDeleteAll: RemoveAll %s: %v", full, err)
		} else {
			count++
		}
	}
	// Delete ALL run records for this policy from DB
	res, err := db.Exec("DELETE FROM backup_policy_runs WHERE policy_id=?", body.PolicyID)
	if err != nil {
		log.Printf("HandleCleanupDeleteAll: DB delete error: %v", err)
	}
	records, _ := res.RowsAffected()
	log.Printf("HandleCleanupDeleteAll: policy=%d folders=%d records=%d freed=%d", body.PolicyID, count, records, freed)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"freed_bytes": freed,
		"count":      count,
		"records":    records,
	})
}

// HandleCleanupDeleteOlderThan handles POST /cron/cleanup/delete-older-than
// Body: {"policy_id": N, "days": 30}
func HandleCleanupDeleteOlderThan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		PolicyID int64 `json:"policy_id"`
		Days     int   `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Days < 1 {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	var destFolder string
	if err := db.QueryRow("SELECT destination_folder FROM backup_policies WHERE id=?", body.PolicyID).Scan(&destFolder); err != nil {
		http.Error(w, "policy not found", http.StatusNotFound)
		return
	}
	cleanDest := filepath.Clean(destFolder)
	if !strings.HasPrefix(cleanDest, safeRoot) {
		http.Error(w, "path not allowed", http.StatusForbidden)
		return
	}
	cutoff := time.Now().AddDate(0, 0, -body.Days)
	entries, _ := listRunFolders(destFolder)
	var freed int64
	count := 0
	var deletedFolders []string
	for _, e := range entries {
		t, err := time.Parse("2006-01-02_15-04-05", e.Name())
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			full := filepath.Join(cleanDest, e.Name())
			freed += calculateDirSize(full)
			removeAllSafe(full)
			deleteRunDBRecord(body.PolicyID, e.Name())
			deletedFolders = append(deletedFolders, e.Name())
			count++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"freed_bytes":     freed,
		"count":           count,
		"deleted_folders": deletedFolders,
	})
}

// fmtBytesCleanup formats bytes into human-readable string (used in cleanup page).
func fmtBytesCleanup(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	if n > 1073741824 {
		return fmt.Sprintf("%.1f GB", float64(n)/1073741824)
	}
	if n > 1048576 {
		return fmt.Sprintf("%.1f MB", float64(n)/1048576)
	}
	if n > 1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%d B", n)
}

// removeAllSafe is os.RemoveAll with an extra safety check.
func removeAllSafe(path string) error {
	clean := filepath.Clean(path)
	if !strings.HasPrefix(clean, safeRoot) {
		return fmt.Errorf("path outside allowed root: %s", clean)
	}
	return os.RemoveAll(clean)
}

// HandleCleanupDeleteSelected handles POST /cron/cleanup/delete-selected
// Body: {"policy_id": N, "folders": ["2026-03-26_21-37-56", ...]}
func HandleCleanupDeleteSelected(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		PolicyID int64    `json:"policy_id"`
		Folders  []string `json:"folders"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Folders) == 0 {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	var destFolder string
	if err := db.QueryRow("SELECT destination_folder FROM backup_policies WHERE id=?", body.PolicyID).Scan(&destFolder); err != nil {
		http.Error(w, "policy not found", http.StatusNotFound)
		return
	}
	var freed int64
	var deleted []string
	for _, folder := range body.Folders {
		if !runFolderRe.MatchString(folder) {
			continue
		}
		fullPath, err := validateCleanupPath(destFolder, folder)
		if err != nil {
			continue
		}
		freed += calculateDirSize(fullPath)
		if err := os.RemoveAll(fullPath); err == nil {
			deleteRunDBRecord(body.PolicyID, folder)
			deleted = append(deleted, folder)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"freed_bytes": freed,
		"deleted":     deleted,
	})
}

// HandleCleanupDeleteRecordOnly handles POST /cron/cleanup/delete-record-only
// Body: {"run_id": N}  — removes only the DB record, not files on disk.
func HandleCleanupDeleteRecordOnly(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		RunID int64 `json:"run_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RunID == 0 {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	// Verify the run exists (security: confirm it's a real record)
	var policyID int64
	if err := db.QueryRow("SELECT policy_id FROM backup_policy_runs WHERE id=?", body.RunID).Scan(&policyID); err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	db.Exec("DELETE FROM backup_policy_runs WHERE id=?", body.RunID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// HandleCleanupClearAllRecords handles POST /cron/cleanup/clear-all-records
// Body: {"policy_id": N} — deletes ALL run records for a policy from DB.
func HandleCleanupClearAllRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		PolicyID int64 `json:"policy_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PolicyID == 0 {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	// Verify policy exists
	var dummy int64
	if err := db.QueryRow("SELECT id FROM backup_policies WHERE id=?", body.PolicyID).Scan(&dummy); err != nil {
		http.Error(w, "policy not found", http.StatusNotFound)
		return
	}
	res, _ := db.Exec("DELETE FROM backup_policy_runs WHERE policy_id=?", body.PolicyID)
	n, _ := res.RowsAffected()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "deleted": n})
}

// jsonErr writes a JSON error response.
func jsonErr(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": msg})
}
