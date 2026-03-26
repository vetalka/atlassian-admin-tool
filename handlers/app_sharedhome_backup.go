package handlers

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

// BackupNFS backs up the nfs from either shared home or home directory for both Jira and Confluence.
func BackupNFS(client *ssh.Client, sharedHomeDir, homeDir, backupDir, app string) error {
	var sourceDir string

	// Determine the source directory based on sharedHomeDir existence
	if sharedHomeDir != "" {
		log.Printf("Shared home (NFS) detected for %s, using %s for NFS backup.", app, sharedHomeDir)
		sourceDir = sharedHomeDir
	} else {
		log.Printf("Shared home (NFS) not detected, using %s for NFS backup.", homeDir)
		sourceDir = homeDir
	}

	// Define the backup file path
	backupFilePath := filepath.Join(backupDir, "NFS.tar.gz")

	// Build the appropriate tar command based on whether it's Jira or Confluence
	var tarCmd string
	if app == "Jira" {
		tarCmd = fmt.Sprintf(
			"sudo tar czf - -C %s --exclude='export' --exclude='import' --exclude='Data_Backup_*' --exclude='log' --exclude='data' --exclude='eazybi.toml' --exclude='*/plugins/.osgi-plugins/*' --exclude='*/plugins/.bundled-plugins/*' .",
			sourceDir,
		)
	} else if app == "Confluence" {
		tarCmd = fmt.Sprintf(
			"sudo tar czf - -C %s --exclude='attachments' --exclude='backups' --exclude='Data_Backup_*' --exclude='logs' .",
			sourceDir,
		)
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

	log.Printf("Backup completed for %s attachments from %s to %s.", app, sourceDir, backupFilePath)
	return nil
}

// BackupAttachments backs up the attachments from either shared home or home directory for both Jira and Confluence.
func BackupAttachments(client *ssh.Client, sharedHomeDir, homeDir, backupDir, app string) error {
	var sourceDir string

	// Determine the source directory based on sharedHomeDir existence
	if sharedHomeDir != "" {
		log.Printf("Shared home (NFS) detected for %s, using %s for attachments backup.", app, sharedHomeDir)
		sourceDir = sharedHomeDir
	} else {
		log.Printf("Shared home (NFS) not detected, using %s for attachments backup.", homeDir)
		sourceDir = homeDir
	}

	// Define the backup file path
	backupFilePath := filepath.Join(backupDir, "Attachments.tar.gz")

	// Build the appropriate tar command based on whether it's Jira or Confluence
	var tarCmd string
	if app == "Jira" {
		tarCmd = fmt.Sprintf(
			"sudo tar czf - -C %s/data  .",
			sourceDir,
		)
	} else if app == "Confluence" {
		tarCmd = fmt.Sprintf(
			"sudo tar czf - -C %s/attachments  .",
			sourceDir,
		)
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

	log.Printf("Backup completed for %s attachments from %s to %s.", app, sourceDir, backupFilePath)
	return nil
}
