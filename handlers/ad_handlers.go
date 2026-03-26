package handlers

import (
    "fmt"
    "html"
    "html/template"
    "net/http"
	"log"
	"database/sql" 
	"strconv" 
)

////////////////////////  Users Section ///////////////////////////////////
func HandleUserList(w http.ResponseWriter, r *http.Request) {
    rows, err := db.Query("SELECT username, directory, groups FROM users ORDER BY username")
    if err != nil {
        http.Error(w, "Failed to load users", http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    type userRow struct{ Username, Directory, Groups string }
    var users []userRow
    directorySet := make(map[string]bool)
    groupSet := make(map[string]bool)
    for rows.Next() {
        var u userRow
        if err := rows.Scan(&u.Username, &u.Directory, &u.Groups); err == nil {
            users = append(users, u)
            directorySet[u.Directory] = true
            groupSet[u.Groups] = true
        }
    }

    // Load groups for Add User dropdown
    groupRows, err := db.Query("SELECT DISTINCT groups FROM groups")
    groupOptions := ""
    if err == nil {
        defer groupRows.Close()
        for groupRows.Next() {
            var g string
            if err := groupRows.Scan(&g); err == nil {
                groupOptions += fmt.Sprintf(`<option value="%s">%s</option>`, g, g)
            }
        }
    }

    // Filter dropdowns
    dirFilterOpts := `<option value="">All Directories</option>`
    for d := range directorySet {
        dirFilterOpts += fmt.Sprintf(`<option value="%s">%s</option>`, d, d)
    }
    grpFilterOpts := `<option value="">All Groups</option>`
    for g := range groupSet {
        grpFilterOpts += fmt.Sprintf(`<option value="%s">%s</option>`, g, g)
    }

    // Build table rows
    rowsHTML := ""
    for _, u := range users {
        rowsHTML += fmt.Sprintf(`
        <tr data-user="%s" data-dir="%s" data-grp="%s">
            <td>%s</td>
            <td>%s</td>
            <td>%s</td>
            <td>
                <a href="/settings/users/edit?username=%s">Edit Group</a>
                <span style="color:var(--color-border); margin:0 6px;">|</span>
                <a href="#" onclick="document.getElementById('pw-modal-%s').style.display='flex'; return false;">Change Password</a>
            </td>
        </tr>`, u.Username, u.Directory, u.Groups,
            u.Username, u.Directory, u.Groups, u.Username, u.Username)
    }

    output := fmt.Sprintf(`
    <div class="content-title">User List</div>
    <div style="display:flex; align-items:center; gap:10px; margin-bottom:16px; flex-wrap:wrap;">
        <input type="text" id="userSearch" class="ads-input" placeholder="Search users..." style="width:300px; height:34px; font-size:13px;" oninput="window._filterUsers&&window._filterUsers()">
        <select id="dirFilter" class="ads-input" style="width:170px; height:34px; font-size:13px;" onchange="window._filterUsers&&window._filterUsers()">%s</select>
        <select id="grpFilter" class="ads-input" style="width:170px; height:34px; font-size:13px;" onchange="window._filterUsers&&window._filterUsers()">%s</select>
        <div style="flex:1;"></div>
        <button class="ads-button ads-button-primary" style="white-space:nowrap; height:34px; font-size:13px;" onclick="document.getElementById('add-user-modal').style.display='flex'">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="margin-right:4px;"><line x1="12" y1="5" x2="12" y2="19"></line><line x1="5" y1="12" x2="19" y2="12"></line></svg>
            Add User
        </button>
    </div>
    <table id="userTable">
        <tr><th>Username</th><th>Directory</th><th>Groups</th><th>Actions</th></tr>
        %s
    </table>
    <div id="userPagination" style="display:flex; align-items:center; gap:4px; margin-top:12px; justify-content:center; font-size:13px;"></div>
    <script>
    (function(){
        var PAGE=20, cur=1;
        var allRows=document.querySelectorAll('#userTable tr[data-user]');
        function goPage(p){cur=p;renderPage();}
        window._goPage=goPage;
        function filterUsers(){
            var q=(document.getElementById('userSearch').value||'').toLowerCase();
            var df=document.getElementById('dirFilter').value;
            var gf=document.getElementById('grpFilter').value;
            var vis=[];
            allRows.forEach(function(r){
                var u=r.getAttribute('data-user').toLowerCase();
                var d=r.getAttribute('data-dir');
                var g=r.getAttribute('data-grp');
                var show=true;
                if(q&&!u.startsWith(q))show=false;
                if(df&&d!==df)show=false;
                if(gf&&g!==gf)show=false;
                r.style.display='none';
                if(show)vis.push(r);
            });
            cur=1;
            renderPage();
            function renderPage(){
                var total=Math.ceil(vis.length/PAGE)||1;
                if(cur>total)cur=total;
                var s=(cur-1)*PAGE;
                vis.forEach(function(r,i){r.style.display=(i>=s&&i<s+PAGE)?'':'none';});
                var pg=document.getElementById('userPagination');
                if(total<=1){pg.innerHTML='';return;}
                var h='';
                h+='<button class="ads-button ads-button-default" style="padding:4px 10px;font-size:12px;" onclick="window._goPage('+(cur>1?cur-1:1)+')">&laquo;</button>';
                var sp=Math.max(1,cur-2),ep=Math.min(total,sp+4);
                if(ep-sp<4)sp=Math.max(1,ep-4);
                if(sp>1)h+='<span style="padding:0 4px">...</span>';
                for(var i=sp;i<=ep;i++){
                    if(i===cur)h+='<button class="ads-button ads-button-primary" style="padding:4px 10px;font-size:12px;">'+i+'</button>';
                    else h+='<button class="ads-button ads-button-default" style="padding:4px 10px;font-size:12px;" onclick="window._goPage('+i+')">'+i+'</button>';
                }
                if(ep<total)h+='<span style="padding:0 4px">...</span>';
                h+='<button class="ads-button ads-button-default" style="padding:4px 10px;font-size:12px;" onclick="window._goPage('+(cur<total?cur+1:total)+')">&raquo;</button>';
                h+='<span style="margin-left:8px;color:var(--color-text-subtle)">'+vis.length+' users</span>';
                pg.innerHTML=h;
            }
            window._renderPage=renderPage;
        }
        window._filterUsers=filterUsers;
        filterUsers();
    })();
    </script>`, dirFilterOpts, grpFilterOpts, rowsHTML)

    // Add User modal
    output += fmt.Sprintf(`
    <div id="add-user-modal" style="display:none; position:fixed; top:0; left:0; right:0; bottom:0; background:rgba(0,0,0,0.4); z-index:2000; align-items:center; justify-content:center;" onclick="if(event.target===this)this.style.display='none'">
        <div style="background:var(--color-bg-card); border-radius:12px; padding:24px; width:420px; box-shadow:0 12px 40px rgba(0,0,0,0.2);">
            <div style="font-size:16px; font-weight:600; margin-bottom:4px;">Add New User</div>
            <div style="font-size:13px; color:var(--color-text-subtle); margin-bottom:16px;">Create a new local directory user</div>
            <form action="/settings/local-ad/add" method="POST">
                <div class="ads-form-group" style="margin-bottom:12px;">
                    <label class="ads-form-label">Username</label>
                    <input type="text" name="username" class="ads-input" required placeholder="Enter username">
                </div>
                <div class="ads-form-group" style="margin-bottom:12px;">
                    <label class="ads-form-label">Password</label>
                    <input type="password" name="password" class="ads-input" required minlength="4" placeholder="Enter password">
                </div>
                <div class="ads-form-group" style="margin-bottom:16px;">
                    <label class="ads-form-label">Group</label>
                    <select name="groups" class="ads-input">%s</select>
                </div>
                <div style="display:flex; gap:8px; justify-content:flex-end;">
                    <button type="button" class="ads-button ads-button-default" onclick="document.getElementById('add-user-modal').style.display='none'">Cancel</button>
                    <button type="submit" class="ads-button ads-button-primary">Add User</button>
                </div>
            </form>
        </div>
    </div>`, groupOptions)

    // Password modals
    for _, u := range users {
        var uid int
        db.QueryRow("SELECT id FROM users WHERE username = ?", u.Username).Scan(&uid)
        output += fmt.Sprintf(`
        <div id="pw-modal-%s" style="display:none; position:fixed; top:0; left:0; right:0; bottom:0; background:rgba(0,0,0,0.4); z-index:2000; align-items:center; justify-content:center;" onclick="if(event.target===this)this.style.display='none'">
            <div style="background:var(--color-bg-card); border-radius:12px; padding:24px; width:400px; box-shadow:0 12px 40px rgba(0,0,0,0.2);">
                <div style="font-size:16px; font-weight:600; margin-bottom:4px;">Change Password</div>
                <div style="font-size:13px; color:var(--color-text-subtle); margin-bottom:16px;">Set a new password for <strong>%s</strong></div>
                <form action="/settings/users/local/update-password" method="POST">
                    <input type="hidden" name="id" value="%d">
                    <div class="ads-form-group" style="margin-bottom:12px;">
                        <label class="ads-form-label">New Password</label>
                        <input type="password" name="new_password" class="ads-input" required minlength="4">
                    </div>
                    <div style="display:flex; gap:8px; justify-content:flex-end;">
                        <button type="button" class="ads-button ads-button-default" onclick="this.closest('[id^=pw-modal]').style.display='none'">Cancel</button>
                        <button type="submit" class="ads-button ads-button-primary">Update Password</button>
                    </div>
                </form>
            </div>
        </div>`, u.Username, u.Username, uid)
    }

    fmt.Fprintln(w, output)
}

// HandleEditUser renders the page to edit a user's details (e.g., group)
func HandleEditUser(w http.ResponseWriter, r *http.Request) {

	// Get the current logged-in username to check if the user is an admin
    username_session, err := GetCurrentUsername(r)
    if err != nil {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    isAdmin, err := IsAdminUser(username_session)
    if err != nil {
        http.Error(w, "Failed to check user permissions", http.StatusInternalServerError)
        return
    }

    username := r.URL.Query().Get("username")

    if username == "" {
        log.Printf("No username provided in query")
        http.Error(w, "User not found", http.StatusNotFound)
        return
    }

    log.Printf("Looking up user with username: %s", username)

    // Fetch user details from the database
    var directory, currentGroup string
    query := "SELECT directory, groups FROM users WHERE username = ?"
    err = db.QueryRow(query, username).Scan(&directory, &currentGroup)
    if err == sql.ErrNoRows {
        log.Printf("User not found in the database: %s", username)
        http.Error(w, "User not found", http.StatusNotFound)
        return
    } else if err != nil {
        log.Printf("Error querying user: %v", err)
        http.Error(w, "Database error", http.StatusInternalServerError)
        return
    }

    // Fetch all available groups
    groupRows, err := db.Query("SELECT DISTINCT groups FROM groups")
    if err != nil {
        log.Printf("Failed to query groups: %v", err)
        http.Error(w, "Failed to load groups", http.StatusInternalServerError)
        return
    }
    defer groupRows.Close()

    // Build the group dropdown options, setting the current group as selected
    groupOptions := ""
    for groupRows.Next() {
        var groupName string
        if err := groupRows.Scan(&groupName); err != nil {
            log.Printf("Failed to scan group: %v", err)
            continue
        }
        selected := ""
        if groupName == currentGroup {
            selected = "selected"
        }
        groupOptions += fmt.Sprintf(`<option value="%s" %s>%s</option>`, groupName, selected, groupName)
    }

    content := fmt.Sprintf(`
    <div class="ads-page-centered"><div class="ads-page-content">
        <div class="ads-breadcrumbs">
            <a href="/settings/users">User Management</a> &rarr; Edit User
        </div>
        <div class="ads-card-flat" style="margin-top:16px;">
            <div class="ads-card-header">
                <div style="width:48px; height:48px; background:linear-gradient(135deg, #0747A6, #0065FF); border-radius:12px; display:flex; align-items:center; justify-content:center; flex-shrink:0;">
                    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"></path><circle cx="12" cy="7" r="4"></circle></svg>
                </div>
                <div>
                    <span class="ads-card-title" style="font-size:18px;">Edit User: %s</span>
                    <div style="font-size:13px; color:var(--color-text-subtle); margin-top:2px;">Directory: %s &middot; Current group: <strong>%s</strong></div>
                </div>
            </div>
            <form action="/settings/users/update-group" method="POST" style="padding:0 16px 24px;">
                <input type="hidden" name="username" value="%s">
                <div style="max-width:400px;">
                    <label style="font-weight:600; font-size:13px; color:var(--color-text-subtle); display:block; margin-bottom:6px;">Assign to Group</label>
                    <select name="group" class="ads-select" style="width:100%%;">
                        %s
                    </select>
                </div>
                <div style="margin-top:20px; padding-top:16px; border-top:1px solid var(--color-border); display:flex; align-items:center; gap:12px;">
                    <button type="submit" class="ads-btn ads-btn-primary">Save Changes</button>
                    <a href="/settings/users" class="ads-button ads-button-default">&larr; Back to User Management</a>
                </div>
            </form>
        </div>
    </div></div>
    `, html.EscapeString(username), html.EscapeString(directory), html.EscapeString(currentGroup), html.EscapeString(username), groupOptions)

    RenderPage(w, PageData{
        Title:   "Edit User: " + username,
        IsAdmin: isAdmin,
        Content: template.HTML(content),
    })
}

// HandleUpdateUserGroup handles updating the user's group
func HandleUpdateUserGroup(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodPost {
        username := r.FormValue("username")
        newGroup := r.FormValue("group")

        // Update the user's group in the database
        _, err := db.Exec("UPDATE users SET groups = ? WHERE username = ?", newGroup, username)
        if err != nil {
            log.Printf("Failed to update user group: %v", err)
            http.Error(w, "Failed to update user group", http.StatusInternalServerError)
            return
        }

        // Redirect back to the user list
        http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
    } else {
        http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
    }
}


///////////////////////////////// Group Section ///////////////////////////////////////

func HandleGroupList(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodPost {
        // Handle the addition of a new group
        newGroupName := r.FormValue("new_group")
        directory := "Local Directory"  // Always set directory to "Local Directory"

        if newGroupName != "" {
            // Check if the group already exists
            var exists bool
            err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM groups WHERE groups = ?)", newGroupName).Scan(&exists)
            if err != nil {
                log.Printf("Failed to check if group exists %s: %v", newGroupName, err)
                http.Error(w, "Failed to check if group exists", http.StatusInternalServerError)
                return
            }

            if exists {
                // Group name already exists, show an error
                html := `
                <!DOCTYPE html>
                <html lang="en">
                <head>
                    <meta charset="UTF-8">
                    <meta name="viewport" content="width=device-width, initial-scale=1.0">
                    <title>Error - Group Exists</title>
                    
                </head>
                <body>
                    <div class="error-container">
                        <div class="error-message">Group name already exists. Please choose a different name.</div>
                        <a href="/settings/users" class="back-button">Go Back to Group Management</a>
                    </div>
                </body>
                </html>
                `
                fmt.Fprintln(w, html)
                return
            }

            // Insert the new group
            _, err = db.Exec("INSERT INTO groups (groups, directory) VALUES (?, ?)", newGroupName, directory)
            if err != nil {
                log.Printf("Failed to add new group %s: %v", newGroupName, err)
                http.Error(w, "Failed to add new group", http.StatusInternalServerError)
                return
            }
        }

        // Redirect to avoid form resubmission on refresh
        http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
        return
    }

    // Query the existing groups
    rows, err := db.Query("SELECT groups, directory FROM groups")
    if err != nil {
        http.Error(w, "Failed to load groups", http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    html := `
    <div class="content-title">Group List</div>
    <table>
        <tr>
            <th>Group</th>
            <th>Directory</th>
            <th>Actions</th>
        </tr>`

    for rows.Next() {
        var groupName, directory string
        if err := rows.Scan(&groupName, &directory); err != nil {
            log.Printf("Failed to scan row: %v", err)
            continue
        }

        if groupName == "administrators" {
            // Administrators group has all privileges and is not editable
            html += fmt.Sprintf(`
            <tr>
                <td>%s</td>
                <td>%s</td>
                <td>All Privileges (Administrators)</td>
            </tr>`, groupName, directory)
        } else {
            // For other groups, display the edit and delete options
            html += fmt.Sprintf(`
            <tr>
                <td>%s</td>
                <td>%s</td>
                <td>
                    <a href="/settings/groups/edit?group=%s">Edit</a> | 
                    <a href="/settings/groups/delete?group=%s" onclick="return confirm('Are you sure you want to delete this group?')">Delete</a>
                </td>
            </tr>`, groupName, directory, groupName, groupName)
        }
    }

    html += `
    </table>

    <h2>Add New Group</h2>
    <form method="POST" action="/settings/groups">
        <label for="new_group">Group Name:</label>
        <input type="text" id="new_group" name="new_group" required>

        <!-- Hidden directory field, always set to "Local Directory" -->
        <input type="hidden" id="directory" name="directory" value="Local Directory">

        <button type="submit" class="green-button">Add Group</button>
    </form>

    
    `

    fmt.Fprintln(w, html)
}

func HandleDeleteGroup(w http.ResponseWriter, r *http.Request) {
    groupName := r.URL.Query().Get("group")
    if groupName == "" {
        http.Error(w, "Group not specified", http.StatusBadRequest)
        return
    }

    // Prevent the "administrators" group from being deleted
    if groupName == "administrators" {
        http.Error(w, "Cannot delete the administrators group", http.StatusForbidden)
        return
    }

    // Delete the group from the database
    _, err := db.Exec("DELETE FROM groups WHERE groups = ?", groupName)
    if err != nil {
        log.Printf("Failed to delete group %s: %v", groupName, err)
        http.Error(w, "Failed to delete group", http.StatusInternalServerError)
        return
    }

    // Redirect back to the group list
    http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
}


// HandleAddGroup processes the addition of a new group
func HandleAddGroup(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodPost {
        groupName := r.FormValue("new_group")
        directory := r.FormValue("directory")

        if groupName == "" {
            http.Error(w, "Group name cannot be empty", http.StatusBadRequest)
            return
        }

        // Insert the new group into the 'groups' table with the directory field
        _, err := db.Exec("INSERT INTO groups (groups, directory) VALUES (?, ?)", groupName, directory)
        if err != nil {
            log.Printf("Failed to insert new group %s: %v", groupName, err)
            http.Error(w, "Failed to add group", http.StatusInternalServerError)
            return
        }

        // Redirect back to the group list
        http.Redirect(w, r, "/settings/groups", http.StatusSeeOther)
    } else {
        http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
    }
}

// HandleEditGroup renders the actions form for a specific group
func HandleEditGroup(w http.ResponseWriter, r *http.Request) {

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

    group := r.URL.Query().Get("group")
    if group == "" {
        http.Error(w, "Group not specified", http.StatusBadRequest)
        return
    }

    // Fetch available actions for Jira, Confluence, and Bitbucket
    availableActionsJira, err := GetAvailableActionsForApp("jira")
    if err != nil {
        log.Printf("Failed to load available actions for Jira: %v", err)
        http.Error(w, "Failed to load available actions for Jira", http.StatusInternalServerError)
        return
    }

    availableActionsConfluence, err := GetAvailableActionsForApp("confluence")
    if err != nil {
        log.Printf("Failed to load available actions for Confluence: %v", err)
        http.Error(w, "Failed to load available actions for Confluence", http.StatusInternalServerError)
        return
    }

    availableActionsBitbucket, err := GetAvailableActionsForApp("bitbucket")
    if err != nil {
        log.Printf("Failed to load available actions for Bitbucket: %v", err)
        http.Error(w, "Failed to load available actions for Bitbucket", http.StatusInternalServerError)
        return
    }

    // Fetch current actions for the group
    currentActions, err := GetCurrentActionsForGroup(group)
    if err != nil {
        log.Printf("Failed to query current actions for group %s: %v", group, err)
        http.Error(w, "Failed to load group actions", http.StatusInternalServerError)
        return
    }

    // Build the content using ADS design
    content := fmt.Sprintf(`
    <div class="ads-page-centered"><div class="ads-page-content">
        <div class="ads-breadcrumbs">
            <a href="/settings/users">User Management</a> &rarr; Edit Group Permissions
        </div>
        <div class="ads-card-flat" style="margin-top:16px;">
            <div class="ads-card-header">
                <div style="width:48px; height:48px; background:linear-gradient(135deg, #5243AA, #8777D9); border-radius:12px; display:flex; align-items:center; justify-content:center; flex-shrink:0;">
                    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"></path><circle cx="9" cy="7" r="4"></circle><path d="M23 21v-2a4 4 0 0 0-3-3.87"></path><path d="M16 3.13a4 4 0 0 1 0 7.75"></path></svg>
                </div>
                <div>
                    <span class="ads-card-title" style="font-size:18px;">Edit Permissions: %s</span>
                    <div style="font-size:13px; color:var(--color-text-subtle); margin-top:2px;">Select which actions this group can perform per application</div>
                </div>
            </div>
            <form action="/settings/groups/update-actions" method="POST" style="padding:0 16px 24px;">
                <input type="hidden" name="group" value="%s">
                <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(280px, 1fr)); gap:16px;">
    `, html.EscapeString(group), html.EscapeString(group))

    // Helper to build an app section
    buildAppSection := func(appName, iconColor, iconSVG string, actions []map[string]string) string {
        s := fmt.Sprintf(`
                    <div style="border:1px solid var(--color-border); border-radius:8px; overflow:hidden;">
                        <div style="padding:12px 16px; background:%s; display:flex; align-items:center; gap:10px;">
                            %s
                            <span style="font-weight:600; font-size:14px; color:white;">%s</span>
                        </div>
                        <div style="padding:4px 0;">`, iconColor, iconSVG, appName)
        for _, action := range actions {
            id := action["id"]
            name := action["action"]
            checked := ""
            actionID, _ := strconv.Atoi(id)
            if _, exists := currentActions[actionID]; exists {
                checked = "checked"
            }
            s += fmt.Sprintf(`
                            <label style="display:flex; align-items:center; gap:10px; padding:10px 16px; cursor:pointer; transition:background 0.1s;" onmouseover="this.style.backgroundColor='var(--color-bg-hover)'" onmouseout="this.style.backgroundColor='transparent'">
                                <input type="checkbox" name="actions" value="%s" %s style="width:18px; height:18px; cursor:pointer;">
                                <span style="font-size:14px;">%s</span>
                            </label>`, id, checked, name)
        }
        s += `
                        </div>
                    </div>`
        return s
    }

    content += buildAppSection("Jira", "linear-gradient(135deg, #0052CC, #2684FF)",
        `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"></polygon></svg>`,
        availableActionsJira)

    content += buildAppSection("Confluence", "linear-gradient(135deg, #1868DB, #4C9AFF)",
        `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2"><path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z"></path><path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z"></path></svg>`,
        availableActionsConfluence)

    content += buildAppSection("Bitbucket", "linear-gradient(135deg, #0747A6, #0065FF)",
        `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2"><polyline points="16 18 22 12 16 6"></polyline><polyline points="8 6 2 12 8 18"></polyline></svg>`,
        availableActionsBitbucket)

    content += fmt.Sprintf(`
                </div>
                <div style="margin-top:20px; padding-top:16px; border-top:1px solid var(--color-border); display:flex; align-items:center; gap:12px;">
                    <button type="submit" class="ads-btn ads-btn-primary">Save Permissions</button>
                    <a href="/settings/users" class="ads-button ads-button-default">&larr; Back to User Management</a>
                </div>
            </form>
        </div>
    </div></div>`)

    RenderPage(w, PageData{
        Title:   "Edit Group: " + group,
        IsAdmin: isAdmin,
        Content: template.HTML(content),
    })
}

// buildActionsTable generates HTML rows for actions
func buildActionsTable(availableActions []map[string]string, currentActions map[int]bool) string {
    html := ""
    for _, action := range availableActions {
        id := action["id"]
        name := action["action"]
        checked := ""
        actionID, _ := strconv.Atoi(id)  // Convert the action ID to an integer
        if _, exists := currentActions[actionID]; exists {
            checked = "checked"
        }
        // Use only a checkbox without a hidden input field
        html += fmt.Sprintf(`
            <tr>
                <td>%s</td>
                <td>
                    <input type="checkbox" name="actions" value="%s" %s>
                </td>
            </tr>
        `, name, id, checked) // Use `id` as the value
    }
    return html
}



// HandleUpdateGroupActions handles updating the group's actions
func HandleUpdateGroupActions(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
        return
    }

    groupName := r.FormValue("group")
    if groupName == "" {
        http.Error(w, "Group not found", http.StatusBadRequest)
        return
    }

    // Get selected actions from the form (all checked boxes)
    selectedActions := r.Form["actions"]

    // Corrected log statement
    log.Printf("Selected actions for group %s: %v", groupName, selectedActions)

    // Clear existing actions for this group
    _, err := db.Exec("DELETE FROM group_actions WHERE group_name = ?", groupName)
    if err != nil {
        log.Printf("Failed to clear actions for group %s: %v", groupName, err)
        http.Error(w, "Failed to update group actions", http.StatusInternalServerError)
        return
    }

    // Insert the new selected actions
    for _, actionID := range selectedActions {
        actionIDInt, err := strconv.Atoi(actionID)
        if err != nil {
            log.Printf("Failed to convert action ID to int: %v", err)
            continue
        }
        _, err = db.Exec("INSERT INTO group_actions (group_name, action_id) VALUES (?, ?)", groupName, actionIDInt)
        if err != nil {
            log.Printf("Failed to insert action %s for group %s: %v", actionID, groupName, err)
            http.Error(w, "Failed to update group actions", http.StatusInternalServerError)
            return
        }
    }

    // Redirect back to the user list
    http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
}



func PopulateActionsTable() {
    // Query all actions from the database
    rows, err := db.Query("SELECT action, app FROM actions")
    if err != nil {
        log.Printf("Failed to query actions from the database: %v", err)
        return
    }
    defer rows.Close()

    actionsByApp := make(map[string][]string)

    // Populate the actionsByApp map with the actions from the database
    for rows.Next() {
        var action, app string
        if err := rows.Scan(&action, &app); err != nil {
            log.Printf("Failed to scan action row: %v", err)
            continue
        }
        actionsByApp[app] = append(actionsByApp[app], action)
    }

    // Now process the actionsByApp map as needed, for example, logging them or using them
    for app, actions := range actionsByApp {
        log.Printf("App: %s, Actions: %v", app, actions)
        // You can process these actions as needed
    }
}

func HandleEditGroupActions(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodPost {
        groupName := r.FormValue("group")
        selectedActions := r.Form["actions"] // This will be a slice of selected action IDs

        // Remove all existing actions for the group
        _, err := db.Exec("DELETE FROM group_actions WHERE group_name = ?", groupName)
        if err != nil {
            log.Printf("Failed to clear actions for group %s: %v", groupName, err)
            http.Error(w, "Failed to update actions", http.StatusInternalServerError)
            return
        }

        // Insert the selected actions
        for _, actionID := range selectedActions {
            _, err := db.Exec("INSERT INTO group_actions (group_name, action_id) VALUES (?, ?)", groupName, actionID)
            if err != nil {
                log.Printf("Failed to insert action %s for group %s: %v", actionID, groupName, err)
                http.Error(w, "Failed to update actions", http.StatusInternalServerError)
                return
            }
        }

        // Redirect back to the group list
        http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
    } else {
        http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
    }
}


// GetAvailableActionsForApp returns all available actions for a specific app from the actions table
func GetAvailableActionsForApp(app string) ([]map[string]string, error) {
	query := "SELECT id, action FROM actions WHERE app = ?"
	rows, err := db.Query(query, app)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var actions []map[string]string
	for rows.Next() {
		var id, action string
		if err := rows.Scan(&id, &action); err != nil {
			return nil, err
		}
		actions = append(actions, map[string]string{
			"id":     id,
			"action": action,
		})
	}
	return actions, nil
}


// GetCurrentActionsForGroup returns the actions currently assigned to a group
func GetCurrentActionsForGroup(group string) (map[int]bool, error) {
    query := `
        SELECT a.id 
        FROM group_actions ga
        JOIN actions a ON ga.action_id = a.id
        WHERE ga.group_name = ?
    `
    rows, err := db.Query(query, group)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    currentActions := make(map[int]bool)
    for rows.Next() {
        var actionID int
        if err := rows.Scan(&actionID); err != nil {
            return nil, err
        }
        currentActions[actionID] = true
    }
    return currentActions, nil
}




//////////////////////////////////////////////////////////////////////////////////////////////////

// HandleUserDirectories shows a list of user directories
func HandleUserDirectories(w http.ResponseWriter, r *http.Request) {
    html := `
    <h1>User Directories</h1>
    <table>
        <tr>
            <th>Directory Name</th>
            <th>Type</th>
        </tr>
        <tr>
            <td>Local Directory</td>
            <td>Internal</td>
        </tr>
    </table>
    `
    
    fmt.Fprintln(w, html)
}

// getGroupActions fetches the currently assigned actions for a group from the database
func getGroupActions(groupName string) ([]string, error) {
    query := "SELECT action FROM group_actions WHERE group_name = ?"
    rows, err := db.Query(query, groupName)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var actions []string
    for rows.Next() {
        var action string
        if err := rows.Scan(&action); err != nil {
            return nil, err
        }
        actions = append(actions, action)
    }
    return actions, nil
}

// contains is a helper function to check if a slice contains a specific string
func contains(slice []string, item string) bool {
    for _, value := range slice {
        if value == item {
            return true
        }
    }
    return false
}

/*
// Not in use anable only if will integrate ldap connection
// HandleAddUserDirectory shows the form to add a new directory
func HandleAddUserDirectory(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet {
        html := `
        <h1>Add a New Directory</h1>
        <form method="POST" action="/settings/user-directories/add">
            <label for="name">Directory Name</label>
            <input type="text" id="name" name="name" required><br>
            <label for="type">Type</label>
            <select id="type" name="type">
                <option value="internal">Internal</option>
                <option value="ad">Active Directory</option>
                <option value="saml">SAML</option>
            </select><br>
            <button type="submit">Add Directory</button>
        </form>`

        fmt.Fprintln(w, html)
    } else if r.Method == http.MethodPost {
        // Logic to add the directory to the database
        directoryName := r.FormValue("name")
        directoryType := r.FormValue("type")

        // For simplicity, just outputting it here
        fmt.Fprintf(w, "Added Directory: %s of type %s", directoryName, directoryType)
    }
}


// HandleEditUserDirectory allows the user to edit an existing directory
func HandleEditUserDirectory(w http.ResponseWriter, r *http.Request) {
    html := `
    <h1>Edit Directory</h1>
    <form method="POST" action="/settings/user-directories/edit">
        <label for="name">Directory Name</label>
        <input type="text" id="name" name="name" value="Jira Internal Directory" required><br>
        <label for="type">Type</label>
        <select id="type" name="type">
            <option value="internal">Internal</option>
            <option value="ad">Active Directory</option>
            <option value="saml">SAML</option>
        </select><br>
        <button type="submit">Save Changes</button>
    </form>`

    fmt.Fprintln(w, html)
}
*/