package handlers

import (
	"fmt"
	"log"
	"net/http"
)

// HandleUserCreation handles the creation of a new user
func HandleUserCreation(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		html := `
        <!DOCTYPE html>
        <html lang="en">
        <head>
            <meta charset="UTF-8">
            <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <link rel="stylesheet" href="/static/styles.css">
            <title>Create User</title>
            
        </head>
        <body>
            <div class="container">
                <h1>Create Administrator User</h1>
                <form action="/create-user" method="POST">
                    <label for="username">Username</label>
                    <input type="text" id="username" name="username" required><br>
                    <label for="password">Password</label>
                    <input type="password" id="password" name="password" required><br>
                    <button type="submit">Create User</button>
                </form>
            </div>
        </body>
        </html>
        `
		fmt.Fprintln(w, html)
	} else if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")

		passwordHash, err := hashPassword(password)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		_, err = db.Exec("INSERT INTO users (username, password, directory, groups) VALUES (?, ?, ?, ?)", username, passwordHash, "Local Directory", "administrators")
		if err != nil {
			log.Printf("Failed to create user: %v", err)
			http.Error(w, "User creation failed", http.StatusBadRequest)
			return
		}

		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}
