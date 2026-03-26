package handlers
/*
import (
    "fmt"
    "log"
    "net/http"
    "time"
)

// HandleSSEBackupRestore pushes the restore status to the client using Server-Sent Events (SSE)
func HandleSSEBackupRestore(w http.ResponseWriter, r *http.Request) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
        return
    }

    // Set headers for SSE
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    // For this example, let's assume the environment name is passed as a query parameter
    environmentName := r.URL.Query().Get("environment")
    if environmentName == "" {
        http.Error(w, "Missing environment name", http.StatusBadRequest)
        return
    }

    // Continuously send updates to the client
    for {
        status, exists := GetRestoreStatus(environmentName)
        if exists {
            // Send the status as an SSE event
            fmt.Fprintf(w, "data: %s: %d%% - %s\n\n", environmentName, status.Progress, status.Message)
            flusher.Flush() // Ensure the message is sent to the client

            // If the restore process is complete, break the loop
            if status.Progress >= 100 {
                log.Printf("Restore process for %s is complete.", environmentName)
                break
            }
        }

        // Sleep before sending the next update (1 second delay)
        time.Sleep(1 * time.Second)
    }
}

*/