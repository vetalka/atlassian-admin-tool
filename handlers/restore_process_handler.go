package handlers

import (
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
)

// HandleRestoreProgress serves the restore progress page with a progress bar
func HandleRestoreProgress(w http.ResponseWriter, r *http.Request) {
	environmentName := extractEnvironmentName(r.URL.Path)
	sanitizedEnv := html.EscapeString(environmentName)

	extraHead := template.HTML(fmt.Sprintf(`
	<style>
		.progress-container { width:100%%; background:var(--color-bg-card); border-radius:8px; overflow:hidden; border:1px solid var(--color-border); height:32px; position:relative; }
		.progress-fill { height:100%%; background:linear-gradient(90deg,#ff7452,#ff991f); transition:width 0.5s ease; border-radius:8px 0 0 8px; }
		.progress-text { position:absolute; top:50%%; left:50%%; transform:translate(-50%%,-50%%); font-weight:600; font-size:13px; color:var(--color-text); text-shadow:0 0 4px var(--color-bg-card); }
		.status-message { font-size:14px; color:var(--color-text-subtle); margin-top:12px; padding:12px 16px; background:var(--color-bg-card); border-radius:6px; border:1px solid var(--color-border); display:flex; align-items:center; gap:10px; }
		.status-icon { font-size:18px; }
		.step-log { margin-top:16px; padding:12px 16px; background:var(--color-bg-card); border-radius:6px; border:1px solid var(--color-border); max-height:200px; overflow-y:auto; font-size:12px; font-family:monospace; color:var(--color-text-subtle); }
		.step-log div { padding:2px 0; }
		.step-log div:last-child { color:var(--color-text); font-weight:500; }
		.action-buttons { margin-top:20px; display:flex; gap:8px; }
	</style>
	<script>
		const stepLog = [];
		function updateProgress() {
			fetch('/environment/restore-status/%s')
				.then(response => response.json())
				.then(data => {
					const pct = data.progress, msg = data.message;
					document.getElementById('progress-fill').style.width = pct + '%%';
					document.getElementById('progress-pct').innerText = pct + '%%';
					document.getElementById('status-text').innerText = msg;
					const icon = document.getElementById('status-icon');
					if (msg.includes('Error') || msg.includes('failed')) icon.innerText = '❌';
					else if (pct >= 100) icon.innerText = '✅';
					else icon.innerText = '⏳';
					if (stepLog.length === 0 || stepLog[stepLog.length-1] !== msg) {
						stepLog.push(msg);
						const logEl = document.getElementById('step-log');
						const div = document.createElement('div');
						div.textContent = new Date().toLocaleTimeString() + ' — ' + msg;
						logEl.appendChild(div);
						logEl.scrollTop = logEl.scrollHeight;
					}
					const cancelBtn = document.getElementById('cancel-btn'), backBtn = document.getElementById('back-btn');
					if (pct >= 100 || msg.includes('Error') || msg.includes('failed') || msg.includes('cancelled')) {
						cancelBtn.style.display = 'none'; backBtn.style.display = 'inline-flex';
					} else {
						cancelBtn.style.display = 'inline-flex'; backBtn.style.display = 'none';
						setTimeout(updateProgress, 1500);
					}
				})
				.catch(() => {
					document.getElementById('status-text').innerText = 'Error fetching status. Check logs.';
					document.getElementById('status-icon').innerText = '❌';
				});
		}
		updateProgress();
	</script>`, sanitizedEnv))

	content := fmt.Sprintf(`
		<div class="ads-page-centered"><div class="ads-page-content"><div class="ads-breadcrumbs"><a href="/">Environments</a> → <a href="/environment/%s">%s</a> → Restore Progress</div>
		
			<div class="ads-card-flat" style="margin-top:16px;">
				<div class="ads-card-header">
					<span style="font-size:24px;">🔄</span>
					<div>
						<span class="ads-card-title">Restore in Progress</span>
						<div style="font-size:13px; color:var(--color-text-subtle); margin-top:2px;">Environment: <strong>%s</strong></div>
					</div>
				</div>
				<div style="padding:0 24px 24px;">
					<div class="progress-container">
						<div class="progress-fill" id="progress-fill" style="width:0%%"></div>
						<span class="progress-text" id="progress-pct">0%%</span>
					</div>
					<div class="status-message">
						<span class="status-icon" id="status-icon">⏳</span>
						<span id="status-text">Starting restore...</span>
					</div>
					<div class="step-log" id="step-log"></div>
					<div class="action-buttons">
						<a id="cancel-btn" href="/environment/cancel-restore/%s" class="ads-button ads-button-danger"
						   onclick="return confirm('Cancel the restore?')">✕ Cancel Restore</a>
						<a id="back-btn" href="/environment/%s" class="ads-button ads-button-primary" style="display:none;">← Back to Environment</a>
					</div>
				</div>
			</div>
		</div>
	</div></div>
	`, sanitizedEnv, sanitizedEnv, sanitizedEnv, sanitizedEnv, sanitizedEnv)

	username, _ := GetCurrentUsername(r)
	isAdmin, _ := IsAdminUser(username)

	RenderPage(w, PageData{
		Title:     "Restore Progress - " + environmentName,
		IsAdmin:   isAdmin,
		ExtraHead: extraHead,
		Content:   template.HTML(content),
	})
}

func HandleRestoreStatus(w http.ResponseWriter, r *http.Request) {
	environmentName := extractEnvironmentName(r.URL.Path)
	progress, message := GetRestoreTaskStatus(environmentName)
	log.Printf("Current status for %s: Progress=%d, Message=%s", environmentName, progress, message)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"progress": %d, "message": "%s"}`, progress, message)
}
