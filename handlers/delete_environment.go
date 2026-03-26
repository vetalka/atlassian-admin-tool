package handlers

import (
    "fmt"
    "html"
    "html/template"
    "log"
    "net/http"
)

func HandleDeleteEnvironment(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodPost {
        r.ParseForm()
        deleted := 0
        for _, env := range r.Form["environments"] {
            if _, err := db.Exec("DELETE FROM environments WHERE name = ?", env); err != nil {
                log.Printf("Failed to delete environment %s: %v", env, err)
            } else {
                deleted++
            }
        }
        if deleted > 0 {
            log.Printf("Deleted %d environment(s)", deleted)
        }
        http.Redirect(w, r, "/delete-environment", http.StatusSeeOther)
        return
    }

    rows, err := db.Query("SELECT name, app, ip FROM environments ORDER BY app, name")
    if err != nil {
        http.Error(w, "Failed to load environments", http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    type envRow struct{ Name, App, IP string }
    var envs []envRow
    for rows.Next() {
        var e envRow
        if err := rows.Scan(&e.Name, &e.App, &e.IP); err != nil {
            continue
        }
        envs = append(envs, e)
    }

    rowsHTML := ""
    if len(envs) == 0 {
        rowsHTML = `<tr><td colspan="4" style="text-align:center; padding:32px; color:var(--color-text-subtle);">No environments found.</td></tr>`
    } else {
        for _, e := range envs {
            rowsHTML += fmt.Sprintf(`
                <tr>
                    <td style="width:40px; text-align:center;"><input type="checkbox" name="environments" value="%s" class="env-check"></td>
                    <td><strong>%s</strong></td>
                    <td><span class="ads-lozenge ads-lozenge-info">%s</span></td>
                    <td style="color:var(--color-text-subtle); font-family:monospace; font-size:12px;">%s</td>
                </tr>`, html.EscapeString(e.Name), html.EscapeString(e.Name),
                html.EscapeString(e.App), html.EscapeString(e.IP))
        }
    }

    extraHead := template.HTML(`<script>
        function toggleAll(src) { document.querySelectorAll('.env-check').forEach(function(c){c.checked=src.checked}); updateBtn(); }
        function updateBtn() {
            var n = document.querySelectorAll('.env-check:checked').length;
            var btn = document.getElementById('del-btn');
            btn.textContent = n > 0 ? 'Delete ' + n + ' environment(s)' : 'Delete selected';
            btn.disabled = n === 0; btn.style.opacity = n > 0 ? '1' : '0.5';
        }
        document.addEventListener('DOMContentLoaded', function() {
            document.querySelectorAll('.env-check').forEach(function(c){c.addEventListener('change',updateBtn)});
        });
    </script>`)

    content := fmt.Sprintf(`
        <div class="ads-page-centered"><div class="ads-page-content"><div class="ads-breadcrumbs"><a href="/">Environments</a> &rarr; Delete</div>

        <div class="ads-card-flat" style="margin-top:16px;">
            <div class="ads-card-header">
                <div style="width:48px; height:48px; background:linear-gradient(135deg, #DE350B, #FF5630); border-radius:12px; display:flex; align-items:center; justify-content:center;">
                    <span style="font-size:22px; color:white;">&#x1F5D1;</span>
                </div>
                <div>
                    <span class="ads-card-title" style="font-size:18px;">Delete Environments</span>
                    <div style="font-size:13px; color:var(--color-text-subtle); margin-top:2px;">
                        Select the environments you want to permanently remove. This action cannot be undone.
                    </div>
                </div>
            </div>
            <form method="POST" action="/delete-environment" style="padding:0 24px 24px;">
                <div style="background:var(--color-bg-card); border:1px solid var(--color-border); border-radius:8px; overflow:hidden;">
                    <table class="ads-table" style="width:100%%;">
                        <thead><tr>
                            <th style="width:40px; text-align:center;"><input type="checkbox" onchange="toggleAll(this)"></th>
                            <th>Environment</th>
                            <th>Application</th>
                            <th>IP Address</th>
                        </tr></thead>
                        <tbody>%s</tbody>
                    </table>
                </div>
                <div style="margin-top:16px; display:flex; gap:8px; align-items:center;">
                    <button type="submit" id="del-btn" class="ads-button ads-button-danger" disabled style="opacity:0.5;"
                            onclick="return confirm('Are you sure? This will permanently delete the selected environments and all their settings.')">
                        Delete selected
                    </button>
                    <a href="/" class="ads-button ads-button-default">&larr; Back to Environments</a>
                </div>
            </form>
        </div>
    </div></div>
    `, rowsHTML)

    RenderPage(w, PageData{
        Title:     "Delete Environments",
        IsAdmin:   true,
        ExtraHead: extraHead,
        Content:   template.HTML(content),
    })
}
