package handlers

import (
    "fmt"
    "log"
    "os"
    "path/filepath"
    "time"
)

// CreateBackupDirectory creates the backup directory structure based on the environment details.
func CreateBackupDirectory(app, name string) (string, error) {
    // Define the base backup directory
    baseBackupDir := "/adminToolBackupDirectory"

    // Construct the date folder: /adminToolBackupDirectory/<app>/<name>/<current_date>/
    currentDate := time.Now().Format("2006-01-02")

    // Construct the time folder: /adminToolBackupDirectory/<app>/<name>/<current_date>/<current_time>/
    currentTime := time.Now().Format("15-04-05") // Format time as HH-MM-SS

    backupDir := filepath.Join(baseBackupDir, app, name, currentDate, currentTime)

    // Check if the backup directory already exists
    if _, err := os.Stat(backupDir); os.IsNotExist(err) {
        // Create the backup directory if it doesn't exist
        err := os.MkdirAll(backupDir, os.ModePerm)
        if err != nil {
            log.Printf("Failed to create backup directory: %v", err)
            return "", fmt.Errorf("failed to create backup directory: %v", err)
        }
        log.Printf("Backup directory created: %s", backupDir)
    } else {
        log.Printf("Backup directory already exists: %s", backupDir)
    }

    return backupDir, nil
}

func CreateBackupDirectoryAttachments(app, name string) (string, error) {
    // Define the base backup directory
    baseBackupDir := "/adminToolBackupDirectory" // Use your base directory here

    // Construct the path to the backup directory without date and time
    backupDirAttachments := filepath.Join(baseBackupDir, app, name)

    // Create the directory if it doesn't exist
    if err := os.MkdirAll(backupDirAttachments, os.ModePerm); err != nil {
        return "", fmt.Errorf("failed to create backup directory: %v", err)
    }

    log.Printf("Backup directory created at: %s", backupDirAttachments)
    return backupDirAttachments, nil
}