package handlers

import (
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"html"
	"log"
	"net/http"
)

// HandleUserManagement renders a page to manage users (list, add, delete, update password)
func HandleLocalUserManagement(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Fetch all users — scan into slice immediately to release the connection
		// before running a second query (avoids deadlock with SetMaxOpenConns(1))
		rows, err := db.Query("SELECT id, username FROM users WHERE directory = 'Local Directory'")
		if err != nil {
			log.Printf("Failed to query users: %v", err)
			http.Error(w, "Failed to load users", http.StatusInternalServerError)
			return
		}
		type localUser struct {
			id       int
			username string
		}
		var users []localUser
		for rows.Next() {
			var u localUser
			if err := rows.Scan(&u.id, &u.username); err != nil {
				log.Printf("Failed to scan user: %v", err)
				continue
			}
			users = append(users, u)
		}
		rows.Close()

		// Now safe to run second query — first connection is released
		groupRows, err := db.Query("SELECT DISTINCT groups FROM groups")
		if err != nil {
			log.Printf("Failed to query groups: %v", err)
			http.Error(w, "Failed to load groups", http.StatusInternalServerError)
			return
		}
		defer groupRows.Close()

		// Build the group dropdown options
		groupOptions := ""
		for groupRows.Next() {
			var groupName string
			if err := groupRows.Scan(&groupName); err != nil {
				log.Printf("Failed to scan group: %v", err)
				continue
			}
			groupOptions += fmt.Sprintf(`<option value="%s">%s</option>`, html.EscapeString(groupName), html.EscapeString(groupName))
		}

		// Start of HTML with adjusted layout
		htmlOut := fmt.Sprintf(`
        
        <div class="user-management-container">
            <div class="user-list-section">
                <div style="display:flex; align-items:center; gap:16px; margin-bottom:16px;">
                    <div class="content-title" style="margin-bottom:0;">Manage Local Directory Users</div>
                </div>
                <div style="margin-bottom:16px;">
                    <input type="text" id="localUserSearch" class="ads-input" placeholder="Search users..." style="width:300px; height:34px; font-size:13px;" oninput="if(typeof filterLocalTable==='function')filterLocalTable()">
                </div>
                <table id="localUserTable">
                    <tr>
                        <th>Username</th>
                        <th>Actions</th>
                        <th>Update Password</th>
                    </tr>`)

		// Add each user to the table with a delete button and password update form
		for _, u := range users {
			id, username := u.id, u.username
			htmlOut += fmt.Sprintf(`
                <tr>
                    <td>%s</td>
                    <td>
                        <form action="/settings/users/local/delete" method="POST" style="display:inline;">
                            <input type="hidden" name="id" value="%d">
                            <button type="submit" class="delete-button">Delete</button>
                        </form>
                    </td>
                    <td>
                        <form action="/settings/users/local/update-password" method="POST" style="display:inline;">
                            <input type="hidden" name="id" value="%d">
                            <input type="password" name="new_password" placeholder="New Password" required>
                            <button type="submit" class="update-button">Update Password</button>
                        </form>
                    </td>
                </tr>`, username, id, id)
		}

		htmlOut += fmt.Sprintf(`
                </table>
            </div>
            <div class="add-user-section">
                <h2>Add a New System User</h2>
                <form action="/settings/local-ad/add" method="POST">
                    <table>
                        <tr>
                            <th><label for="username">Username</label></th>
                            <td><input type="text" id="username" name="username" required></td>
                        </tr>
                        <tr>
                            <th><label for="password">Password</label></th>
                            <td><input type="password" id="password" name="password" required></td>
                        </tr>
                        <tr>
                            <th><label for="groups">Groups</label></th>
                            <td>
                                <select id="groups" name="groups">
                                    %s
                                </select>
                            </td>
                        </tr>
                    </table>
                    <button type="submit" class="add-user-btn">Add User</button>
                </form>
            </div>
        </div>
        <script>
        function filterLocalTable() {
            var q = document.getElementById('localUserSearch').value.toLowerCase();
            var rows = document.getElementById('localUserTable').querySelectorAll('tr:not(:first-child)');
            rows.forEach(function(row) {
                var username = row.cells[0].textContent.toLowerCase();
                row.style.display = username.startsWith(q) ? '' : 'none';
            });
        }
        window._filterLocal = filterLocalTable;
        </script>`, groupOptions)

		fmt.Fprintln(w, htmlOut)
	}
}

// HandleDeleteUser handles deleting a user from the database
func HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		userID := r.FormValue("id")

		// Delete the user from the database
		_, err := db.Exec("DELETE FROM users WHERE id = ?", userID)
		if err != nil {
			log.Printf("Failed to delete user: %v", err)
			http.Error(w, "Failed to delete user", http.StatusInternalServerError)
			return
		}

		// Redirect back to the user management page
		http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
	} else {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
	}
}

// HandleUpdatePassword handles updating a user's password
func HandleUpdatePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		userID := r.FormValue("id")
		newPassword := r.FormValue("new_password")

		// Hash the new password using bcrypt
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Failed to hash new password: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Update the password in the database
		_, err = db.Exec("UPDATE users SET password = ? WHERE id = ?", passwordHash, userID)
		if err != nil {
			log.Printf("Failed to update password: %v", err)
			http.Error(w, "Failed to update password", http.StatusInternalServerError)
			return
		}

		// Redirect back to the user management page
		http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
	} else {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
	}
}

// HandleAddLocalUser handles adding a new local directory user
func HandleAddLocalUser(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")
		groups := r.FormValue("groups")

		// Hash the password using bcrypt
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Insert the new user into the database with groups
		_, err = db.Exec("INSERT INTO users (username, password, directory, groups) VALUES (?, ?, 'Local Directory', ?)", username, passwordHash, groups)
		if err != nil {
			log.Printf("Failed to add user: %v", err)
			http.Error(w, "Failed to add user", http.StatusInternalServerError)
			return
		}

		// Redirect back to the user management page after successful addition
		http.Redirect(w, r, "/settings/users", http.StatusSeeOther)
	} else {
		// If not POST method, return error
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
	}
}
