package handlers

import (
	"fmt"
	"log"
	"net/http"
)

// ShowEnvironments displays all environments stored in the database
func ShowEnvironments(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request to show environments.")

	rows, err := db.Query("SELECT id, name, ip FROM environments")
	if err != nil {
		log.Printf("Database query failed: %v", err)
		http.Error(w, fmt.Sprintf("Failed to retrieve environments: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	log.Println("Database query successful, processing results.")

	fmt.Fprintf(w, `
    <!DOCTYPE html>
    <html lang="en">
    <head>
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>Environments</title>
        <link rel="stylesheet" href="/static/styles.css">
    </head>
    <body>
        <h1>Available Environments</h1>
        <ul>
    `)

	count := 0
	for rows.Next() {
		var id int
		var name, ip string
		err := rows.Scan(&id, &name, &ip)
		if err != nil {
			log.Printf("Failed to scan row: %v", err)
			http.Error(w, "Failed to process environments", http.StatusInternalServerError)
			return
		}
		count++
		fmt.Fprintf(w, `<li>ID: %d, Name: %s, IP: %s</li>`, id, name, ip)
	}

	if count == 0 {
		log.Println("No environments found in the database.")
	}

	if err = rows.Err(); err != nil {
		log.Printf("Error iterating rows: %v", err)
		http.Error(w, "Failed to process environments", http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, `
        </ul>
    </body>
    </html>
    `)
}
