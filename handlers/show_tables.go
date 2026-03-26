package handlers

import (
    "fmt"
    "log"
    "net/http"
)

// ShowTables displays all tables in the SQLite database
func ShowTables(w http.ResponseWriter, r *http.Request) {
    log.Println("Received request to show tables.")

    // Query to list all tables
    query := `SELECT name FROM sqlite_master WHERE type='table';`
    rows, err := db.Query(query)
    if err != nil {
        log.Printf("Database query failed: %v", err)
        http.Error(w, fmt.Sprintf("Failed to retrieve tables: %v", err), http.StatusInternalServerError)
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
        <link rel="stylesheet" href="/static/styles.css">
        <title>Database Tables</title>
    </head>
    <body>
        <h1>Database Tables</h1>
        <ul>
    `)

    count := 0
    for rows.Next() {
        var tableName string
        err := rows.Scan(&tableName)
        if err != nil {
            log.Printf("Failed to scan row: %v", err)
            http.Error(w, "Failed to process tables", http.StatusInternalServerError)
            return
        }
        count++
        fmt.Fprintf(w, `<li>Table: %s</li>`, tableName)
    }

    if count == 0 {
        log.Println("No tables found in the database.")
    }

    if err = rows.Err(); err != nil {
        log.Printf("Error iterating rows: %v", err)
        http.Error(w, "Failed to process tables", http.StatusInternalServerError)
        return
    }

    fmt.Fprint(w, `
        </ul>
    </body>
    </html>
    `)
}

