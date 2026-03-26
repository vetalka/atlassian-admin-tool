package handlers

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PolicyCleanupInfo holds a policy + its merged disk+DB run list.
type PolicyCleanupInfo struct {
	ID                int64
	Name              string
	DestinationFolder string
	TotalSizeBytes    int64
	Runs              []RunInfo
}

// RunInfo represents one backup run — from disk, DB, or both.
type RunInfo struct {
	RunID        int64  // 0 if no DB record
	Folder       string // timestamp folder name; empty if orphan DB record with no parseable folder
	SizeBytes    int64
	Status       string
	StartedAt    string
	ExistsOnDisk bool // folder present on disk
	IsOrphanDB   bool // DB record exists but no folder on disk
}

func loadPolicyCleanupInfos() []PolicyCleanupInfo {
	rows, err := db.Query("SELECT id, name, destination_folder FROM backup_policies ORDER BY name")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var infos []PolicyCleanupInfo
	for rows.Next() {
		var p PolicyCleanupInfo
		if err := rows.Scan(&p.ID, &p.Name, &p.DestinationFolder); err != nil {
			continue
		}

		// --- disk folders ---
		diskFolders := map[string]int64{} // folder→size
		if entries, err := listRunFolders(p.DestinationFolder); err == nil {
			for _, e := range entries {
				full := filepath.Join(p.DestinationFolder, e.Name())
				diskFolders[e.Name()] = calculateDirSize(full)
			}
		}

		// --- DB records ---
		type dbRun struct {
			id        int64
			folder    string // extracted from files_created
			status    string
			startedAt string
		}
		dbRuns := map[string]dbRun{} // folder→record
		dbRows, err := db.Query(
			`SELECT id, status, COALESCE(started_at,''), COALESCE(files_created,'[]')
			 FROM backup_policy_runs WHERE policy_id=? ORDER BY id DESC`, p.ID)
		if err == nil {
			defer dbRows.Close()
			for dbRows.Next() {
				var rid int64
				var status, startedAt, filesJSON string
				dbRows.Scan(&rid, &status, &startedAt, &filesJSON)
				// extract folder from any path in files_created
				folder := extractFolderFromFilesJSON(filesJSON, p.DestinationFolder)
				dbRuns[folder] = dbRun{id: rid, folder: folder, status: status, startedAt: startedAt}
			}
		}

		// --- merge ---
		seen := map[string]bool{}

		// disk-first: folders on disk (matched or unmatched to DB)
		for folder, sz := range diskFolders {
			run := RunInfo{Folder: folder, SizeBytes: sz, ExistsOnDisk: true}
			if rec, ok := dbRuns[folder]; ok {
				run.RunID = rec.id
				run.Status = rec.status
				run.StartedAt = rec.startedAt
			}
			p.Runs = append(p.Runs, run)
			p.TotalSizeBytes += sz
			seen[folder] = true
		}

		// orphan DB records: in DB but no folder on disk
		for folder, rec := range dbRuns {
			if seen[folder] {
				continue
			}
			p.Runs = append(p.Runs, RunInfo{
				RunID:      rec.id,
				Folder:     folder,
				Status:     rec.status,
				StartedAt:  rec.startedAt,
				IsOrphanDB: true,
			})
		}

		// sort newest first by folder name (lexicographic desc = time desc)
		sortRunInfos(p.Runs)
		infos = append(infos, p)
	}
	return infos
}

// extractFolderFromFilesJSON finds the timestamp folder name embedded in the
// files_created JSON array (e.g. ".../2026-03-26_21-37-56/...").
func extractFolderFromFilesJSON(filesJSON, destFolder string) string {
	// Look for runFolderRe pattern preceded by a '/'
	for _, match := range runFolderRe.FindAllString(filesJSON, -1) {
		return match
	}
	return ""
}

// sortRunInfos sorts runs newest-first by folder name.
func sortRunInfos(runs []RunInfo) {
	for i := 1; i < len(runs); i++ {
		for j := i; j > 0 && runs[j].Folder > runs[j-1].Folder; j-- {
			runs[j], runs[j-1] = runs[j-1], runs[j]
		}
	}
}

// HandleShowCleanup renders GET /cron/cleanup
func HandleShowCleanup(w http.ResponseWriter, r *http.Request) {
	username, _ := GetCurrentUsername(r)
	isAdmin, _ := IsAdminUser(username)

	infos := loadPolicyCleanupInfos()

	cards := ""
	for _, p := range infos {
		runsHTML := ""
		hasRuns := len(p.Runs) > 0
		if hasRuns {
			runsHTML += fmt.Sprintf(`
			<div style="display:flex;align-items:center;gap:12px;padding:5px 0;border-bottom:2px solid var(--color-border);font-size:12px;color:var(--color-text-subtle);">
				<input type="checkbox" id="sel-all-%d" title="Select all" style="width:16px;height:16px;cursor:pointer;" onchange="toggleSelectAll(%d,this)">
				<span style="flex:1;font-weight:600;">Folder</span>
				<span style="min-width:70px;text-align:right;font-weight:600;">Size</span>
				<span style="min-width:60px;"></span>
			</div>`, p.ID, p.ID)
		}
		for _, run := range p.Runs {
			if run.IsOrphanDB {
				// Orphan: DB record exists but no folder on disk
				label := run.Folder
				if label == "" {
					label = fmt.Sprintf("run #%d", run.RunID)
				}
				runsHTML += fmt.Sprintf(`
			<div class="cleanup-run-row" id="run-%d-%d" style="display:flex;align-items:center;gap:12px;padding:6px 0;border-bottom:1px solid var(--color-border);opacity:0.6;">
				<input type="checkbox" style="width:16px;height:16px;opacity:0.3;" disabled>
				<span style="font-size:18px;">&#x26A0;&#xFE0F;</span>
				<span style="font-family:monospace;font-size:13px;flex:1;font-style:italic;color:var(--color-text-subtle);">%s <span style="font-size:11px;">(files missing from disk)</span></span>
				<button onclick="deleteRecordOnly(%d,%d,this)" style="padding:3px 10px;font-size:12px;background:#6B778C;color:#fff;border:none;border-radius:4px;cursor:pointer;">&#x1F5D1; Remove Record</button>
			</div>`,
					p.ID, run.RunID,
					html.EscapeString(label),
					run.RunID, p.ID,
				)
			} else {
				// Normal: folder exists on disk
				runsHTML += fmt.Sprintf(`
			<div class="cleanup-run-row" id="run-%d-%s" data-policy="%d" data-folder="%s" data-size="%d" style="display:flex;align-items:center;gap:12px;padding:6px 0;border-bottom:1px solid var(--color-border);">
				<input type="checkbox" class="run-cb run-cb-%d" data-size="%d" style="width:16px;height:16px;cursor:pointer;" onchange="updateSelBar(%d)">
				<span style="font-size:18px;">&#x1F4C1;</span>
				<span style="font-family:monospace;font-size:13px;flex:1;">%s</span>
				<span style="font-size:12px;color:var(--color-text-subtle);min-width:70px;text-align:right;">%s</span>
				<button onclick="deleteRun(%d,'%s',this)" style="padding:3px 10px;font-size:12px;background:#DE350B;color:#fff;border:none;border-radius:4px;cursor:pointer;">&#x1F5D1; Delete</button>
			</div>`,
					p.ID, html.EscapeString(run.Folder),
					p.ID, html.EscapeString(run.Folder), run.SizeBytes,
					p.ID, run.SizeBytes, p.ID,
					html.EscapeString(run.Folder),
					html.EscapeString(fmtBytesCleanup(run.SizeBytes)),
					p.ID, html.EscapeString(run.Folder),
				)
			}
		}
		if !hasRuns {
			runsHTML = `<div style="padding:12px 0;color:var(--color-text-subtle);font-size:13px;">No backup runs on disk.</div>`
		}

		cards += fmt.Sprintf(`
	<div class="ads-card-flat" style="margin-bottom:24px;" id="policy-card-%d">
		<div class="ads-card-header" style="justify-content:space-between;">
			<div>
				<span style="font-size:18px;margin-right:8px;">&#x1F4C1;</span>
				<span style="font-weight:600;font-size:15px;">Policy: %s</span>
			</div>
			<div style="display:flex;align-items:center;gap:12px;">
				<span style="font-size:13px;color:var(--color-text-subtle);">Total: <strong id="total-size-%d">%s</strong></span>
				<button onclick="clearAllRecords(%d)" style="padding:3px 10px;font-size:12px;background:var(--color-border);color:var(--color-text);border:none;border-radius:4px;cursor:pointer;" title="Delete all DB records for this policy (files stay on disk)">&#x1F5D1; Clear All Records</button>
			</div>
		</div>
		<div style="padding:0 24px 8px;">
			<div style="font-size:12px;color:var(--color-text-subtle);margin-bottom:12px;font-family:monospace;">%s</div>
			<div id="runs-%d">%s</div>
			<div id="sel-bar-%d" style="display:none;margin:10px 0;padding:10px 14px;background:var(--color-bg);border:1px solid var(--color-border);border-radius:6px;display:none;align-items:center;gap:12px;flex-wrap:wrap;">
				<span id="sel-label-%d" style="font-size:13px;font-weight:600;flex:1;"></span>
				<button onclick="deleteSelected(%d)" style="padding:5px 14px;font-size:13px;background:#DE350B;color:#fff;border:none;border-radius:4px;cursor:pointer;">&#x1F5D1; Delete Selected</button>
				<button onclick="clearSelection(%d)" style="padding:5px 14px;font-size:13px;background:var(--color-border);color:var(--color-text);border:none;border-radius:4px;cursor:pointer;">&#x2715; Clear</button>
			</div>
			<div id="msg-%d" style="margin:8px 0;font-size:13px;display:none;"></div>
			<div style="display:flex;gap:10px;align-items:center;margin-top:14px;flex-wrap:wrap;">
				<button onclick="deleteAll(%d)" style="padding:5px 14px;font-size:13px;background:#DE350B;color:#fff;border:none;border-radius:4px;cursor:pointer;">&#x1F5D1; Delete All</button>
				<label style="font-size:13px;color:var(--color-text-subtle);">Delete older than</label>
				<input id="days-%d" type="number" value="30" min="1" style="width:60px;padding:4px 8px;border:1px solid var(--color-border);border-radius:4px;background:var(--color-bg);color:var(--color-text);font-size:13px;">
				<span style="font-size:13px;color:var(--color-text-subtle);">days</span>
				<button onclick="deleteOlderThan(%d)" style="padding:5px 14px;font-size:13px;background:#FF991F;color:#fff;border:none;border-radius:4px;cursor:pointer;">&#x1F9F9; Clean Now</button>
			</div>
		</div>
	</div>`,
			p.ID,
			html.EscapeString(p.Name),
			p.ID, html.EscapeString(fmtBytesCleanup(p.TotalSizeBytes)),
			p.ID, // clearAllRecords
			html.EscapeString(p.DestinationFolder),
			p.ID, runsHTML,
			p.ID, p.ID, p.ID, p.ID,
			p.ID,
			p.ID, p.ID, p.ID,
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
		<a href="/cron/policies">Backup Policies</a>
		<a href="/cron/cleanup" class="active">Cleanup Backups</a>
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
				<p class="ads-page-header-description">Manage and delete backup files from scheduled backup policies.</p>
			</div>
			%s
		</div></div>
	</div>
</div>
<script>
function fmtMB(bytes) {
	if (!bytes) return '0 B';
	if (bytes > 1073741824) return (bytes/1073741824).toFixed(1) + ' GB';
	if (bytes > 1048576)    return (bytes/1048576).toFixed(1) + ' MB';
	if (bytes > 1024)       return (bytes/1024).toFixed(1) + ' KB';
	return bytes + ' B';
}

function showMsg(policyID, msg, ok) {
	const el = document.getElementById('msg-' + policyID);
	if (!el) return;
	el.textContent = msg;
	el.style.display = 'block';
	el.style.color = ok ? '#00875A' : '#DE350B';
	setTimeout(() => { el.style.display = 'none'; }, 5000);
}

// ── Checkbox selection ────────────────────────────────────────────────────────

function toggleSelectAll(policyID, cb) {
	document.querySelectorAll('.run-cb-' + policyID).forEach(c => c.checked = cb.checked);
	updateSelBar(policyID);
}

function updateSelBar(policyID) {
	const checked = document.querySelectorAll('.run-cb-' + policyID + ':checked');
	const bar = document.getElementById('sel-bar-' + policyID);
	const label = document.getElementById('sel-label-' + policyID);
	if (!bar) return;
	if (checked.length === 0) {
		bar.style.display = 'none';
		return;
	}
	let totalBytes = 0;
	checked.forEach(c => { totalBytes += parseInt(c.dataset.size || '0', 10); });
	label.textContent = checked.length + ' run' + (checked.length > 1 ? 's' : '') + ' selected (' + fmtMB(totalBytes) + ')';
	bar.style.display = 'flex';
}

function clearSelection(policyID) {
	document.querySelectorAll('.run-cb-' + policyID).forEach(c => c.checked = false);
	const selAll = document.getElementById('sel-all-' + policyID);
	if (selAll) selAll.checked = false;
	updateSelBar(policyID);
}

function deleteSelected(policyID) {
	const checked = document.querySelectorAll('.run-cb-' + policyID + ':checked');
	if (checked.length === 0) return;
	const folders = Array.from(checked).map(c => c.closest('.cleanup-run-row').dataset.folder);
	let totalBytes = 0;
	checked.forEach(c => { totalBytes += parseInt(c.dataset.size || '0', 10); });
	if (!confirm('Delete ' + folders.length + ' selected run(s) (' + fmtMB(totalBytes) + ')? This cannot be undone.')) return;
	fetch('/cron/cleanup/delete-selected', {
		method: 'POST',
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify({policy_id: policyID, folders: folders})
	}).then(r => r.json()).then(d => {
		if (d.success) {
			(d.deleted || folders).forEach(f => {
				const row = document.getElementById('run-' + policyID + '-' + f);
				if (row) row.remove();
			});
			clearSelection(policyID);
			showMsg(policyID, '✓ Deleted ' + (d.deleted ? d.deleted.length : folders.length) + ' run(s). Freed ' + fmtMB(d.freed_bytes) + '.', true);
			checkEmpty(policyID);
		} else {
			showMsg(policyID, d.error || 'Delete failed.', false);
		}
	}).catch(() => showMsg(policyID, 'Delete failed.', false));
}

// ── Individual / bulk delete ──────────────────────────────────────────────────

function deleteRun(policyID, folder, btn) {
	const row = document.getElementById('run-' + policyID + '-' + folder);
	const sizeEl = row ? row.querySelector('span[style*="color"]') : null;
	const sizeText = sizeEl ? sizeEl.textContent : '';
	if (!confirm('Delete backup folder "' + folder + '"? (' + sizeText + ') This cannot be undone.')) return;
	btn.disabled = true;
	fetch('/cron/cleanup/delete-run', {
		method: 'POST',
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify({policy_id: policyID, folder: folder})
	}).then(r => r.json()).then(d => {
		if (d.success) {
			if (row) row.remove();
			showMsg(policyID, '✓ Deleted. Freed ' + fmtMB(d.freed_bytes) + '.', true);
			checkEmpty(policyID);
		} else {
			showMsg(policyID, d.error || 'Delete failed.', false);
			btn.disabled = false;
		}
	}).catch(() => { showMsg(policyID, 'Delete failed.', false); btn.disabled = false; });
}

function deleteAll(policyID) {
	const name = document.querySelector('#policy-card-' + policyID + ' .ads-card-header span:nth-child(2)');
	const label = name ? name.textContent.replace('Policy: ','') : 'this policy';
	if (!confirm('Delete ALL backup runs for policy "' + label + '"? This cannot be undone.')) return;
	fetch('/cron/cleanup/delete-all', {
		method: 'POST',
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify({policy_id: policyID})
	}).then(r => r.json()).then(d => {
		if (d.success) {
			const runsDiv = document.getElementById('runs-' + policyID);
			if (runsDiv) runsDiv.innerHTML = '<div style="padding:12px 0;color:var(--color-text-subtle);font-size:13px;">No backup runs on disk.</div>';
			const bar = document.getElementById('sel-bar-' + policyID);
			if (bar) bar.style.display = 'none';
			showMsg(policyID, '✓ Deleted ' + d.count + ' folder(s). Freed ' + fmtMB(d.freed_bytes) + '.', true);
		} else {
			showMsg(policyID, d.error || 'Delete failed.', false);
		}
	}).catch(() => showMsg(policyID, 'Delete failed.', false));
}

function deleteOlderThan(policyID) {
	const days = parseInt(document.getElementById('days-' + policyID).value, 10);
	if (!days || days < 1) { showMsg(policyID, 'Enter a valid number of days.', false); return; }
	if (!confirm('Delete all backups older than ' + days + ' days for this policy? This cannot be undone.')) return;
	fetch('/cron/cleanup/delete-older-than', {
		method: 'POST',
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify({policy_id: policyID, days: days})
	}).then(r => r.json()).then(d => {
		if (d.success) {
			if (d.deleted_folders) {
				d.deleted_folders.forEach(f => {
					const row = document.getElementById('run-' + policyID + '-' + f);
					if (row) row.remove();
				});
			}
			clearSelection(policyID);
			showMsg(policyID, '✓ Deleted ' + d.count + ' folder(s). Freed ' + fmtMB(d.freed_bytes) + '.', true);
			checkEmpty(policyID);
		} else {
			showMsg(policyID, d.error || 'Delete failed.', false);
		}
	}).catch(() => showMsg(policyID, 'Delete failed.', false));
}

function checkEmpty(policyID) {
	const runsDiv = document.getElementById('runs-' + policyID);
	if (runsDiv && runsDiv.querySelectorAll('.cleanup-run-row').length === 0) {
		runsDiv.innerHTML = '<div style="padding:12px 0;color:var(--color-text-subtle);font-size:13px;">No backup runs on disk.</div>';
		const bar = document.getElementById('sel-bar-' + policyID);
		if (bar) bar.style.display = 'none';
	}
}

function deleteRecordOnly(runID, policyID, btn) {
	if (!confirm('Remove this DB record? The backup files (if any) are NOT deleted.')) return;
	btn.disabled = true;
	fetch('/cron/cleanup/delete-record-only', {
		method: 'POST',
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify({run_id: runID})
	}).then(r => r.json()).then(d => {
		if (d.success) {
			const row = document.getElementById('run-' + policyID + '-' + runID);
			if (row) row.remove();
			showMsg(policyID, '✓ Record removed.', true);
			checkEmpty(policyID);
		} else {
			showMsg(policyID, d.error || 'Failed.', false);
			btn.disabled = false;
		}
	}).catch(() => { showMsg(policyID, 'Failed.', false); btn.disabled = false; });
}

function clearAllRecords(policyID) {
	if (!confirm('Delete ALL run records from DB for this policy? Files on disk are NOT deleted.')) return;
	fetch('/cron/cleanup/clear-all-records', {
		method: 'POST',
		headers: {'Content-Type': 'application/json'},
		body: JSON.stringify({policy_id: policyID})
	}).then(r => r.json()).then(d => {
		if (d.success) {
			// Remove all orphan rows; disk rows stay but lose their run IDs
			document.querySelectorAll('#runs-' + policyID + ' .cleanup-run-row').forEach(row => {
				if (row.id.match(/^run-\d+-\d+$/)) row.remove(); // orphan rows have numeric run IDs
			});
			showMsg(policyID, '✓ Cleared ' + d.deleted + ' record(s) from DB.', true);
		} else {
			showMsg(policyID, d.error || 'Failed.', false);
		}
	}).catch(() => showMsg(policyID, 'Failed.', false));
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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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
	entries, _ := listRunFolders(destFolder)
	var freed int64
	count := 0
	for _, e := range entries {
		full := filepath.Join(cleanDest, e.Name())
		freed += calculateDirSize(full)
		removeAllSafe(full)
		count++
	}
	// Delete all run records for this policy
	db.Exec("DELETE FROM backup_policy_runs WHERE policy_id=?", body.PolicyID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "freed_bytes": freed, "count": count})
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
