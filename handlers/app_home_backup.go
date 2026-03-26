package handlers

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

// BackupHomeDirectory backs up the home directory for both Jira and Confluence.
func BackupHomeDirectory(client *ssh.Client, homeDir, backupDir, appType string) error {
	log.Printf("Starting backup for %s home directory: %s", appType, homeDir)

	// Define the backup file path
	backupFilePath := filepath.Join(backupDir, fmt.Sprintf("%sHomeBackup.tar.gz", appType))

	// Build the tar command to run via SSH
	var tarCmd string
	if appType == "Jira" {
		// Jira-specific exclusions
		tarCmd = fmt.Sprintf(
			"sudo tar czf - -C %s --exclude='data' --exclude='export' --exclude='import' --exclude='log' --exclude='*/plugins/.osgi-plugins/*' --exclude='*/plugins/.bundled-plugins/*' .",
			homeDir,
		)
	} else if appType == "Confluence" {
		// Confluence-specific exclusions
		tarCmd = fmt.Sprintf(
			"sudo tar czf - -C %s --exclude='temp' --exclude='logs' --exclude='backups' --exclude='analytics-logs' .",
			homeDir,
		)
	} else {
		return fmt.Errorf("unsupported application type: %s", appType)
	}

	// Create an SSH session
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	// Prepare to capture the output of the tar command
	var output, stderr bytes.Buffer
	session.Stdout = &output
	session.Stderr = &stderr

	// Run the tar command on the remote server
	if err := session.Run(tarCmd); err != nil {
		log.Printf("Failed to execute tar command: %v, stderr: %s", err, stderr.String())
		return fmt.Errorf("failed to execute tar command: %v", err)
	}

	// Save the output to a file
	err = os.WriteFile(backupFilePath, output.Bytes(), 0644)
	if err != nil {
		return fmt.Errorf("failed to write backup file: %v", err)
	}

	log.Printf("Backup completed for %s home directory from %s to %s.", appType, homeDir, backupFilePath)
	return nil
}
