package handlers

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	//"path/filepath"
)

// RestoreAttachments dynamically handles the restoration process for either Jira or Confluence Shared Home.
func RestoreNFSDir(appType, sharedHomeDir, installDir, NFSRestoreFile, dataCurrentBackup, dataFolder, remoteUser, serverIPs, serverPassword string) error {
	switch appType {
	case "Jira":
		log.Printf("Restoring Jira Shared Home...")
		return RestoreNFSDirJira(sharedHomeDir, installDir, NFSRestoreFile, dataCurrentBackup, dataFolder, remoteUser, serverIPs, serverPassword)
	case "Confluence":
		log.Printf("Restoring Confluence Shared Home...")
		return RestoreNFSDirConfluence(sharedHomeDir, installDir, NFSRestoreFile, dataCurrentBackup, dataFolder, remoteUser, serverIPs, serverPassword)
	default:
		return fmt.Errorf("unsupported application type: %s", appType)
	}
}

// RestoreAttachments dynamically handles the restoration process for either Jira or Confluence Shared Home.
func RestoreSharedHomeDir(appType, sharedHomeDir, installDir, dataRestoreFile, dataCurrentBackup, dataFolder, remoteUser, serverIPs, serverPassword string) error {
	switch appType {
	case "Jira":
		log.Printf("Restoring Jira Shared Home...")
		return RestoreSharedHomeDirJira(sharedHomeDir, installDir, dataRestoreFile, dataCurrentBackup, dataFolder, remoteUser, serverIPs, serverPassword)
	case "Confluence":
		log.Printf("Restoring Confluence Shared Home...")
		return RestoreSharedHomeDirConfluence(sharedHomeDir, installDir, dataRestoreFile, dataCurrentBackup, dataFolder, remoteUser, serverIPs, serverPassword)
	default:
		return fmt.Errorf("unsupported application type: %s", appType)
	}
}

// RestoreAttachments dynamically handles the restoration process for either Jira or Confluence attachments.
func RestoreAttachments(appType, homeDir, installDir, attachmentsRestoreFile, remoteUser, serverIPs, serverPassword string) error {
	switch appType {
	case "Jira":
		log.Printf("Restoring Jira attachments...")
		return RestoreAttachmentsOnEachServerJira(homeDir, installDir, attachmentsRestoreFile, remoteUser, serverIPs, serverPassword)
	case "Confluence":
		log.Printf("Restoring Confluence attachments...")
		return RestoreAttachmentsOnEachServerConfluence(homeDir, installDir, attachmentsRestoreFile, remoteUser, serverIPs, serverPassword)
	default:
		return fmt.Errorf("unsupported application type: %s", appType)
	}
}

// RestoreAttachmentsOnEachServerConfluence handles the restoration process for Confluence attachments.
func RestoreAttachmentsOnEachServerConfluence(homeDir, installDir, attachmentsRestoreFile, remoteUser, serverIPs, serverPassword string) error {
	log.Printf("Starting Confluence attachments restoration on servers: %s", serverIPs)

	// Split serverIPs by space to handle multiple IPs
	ips := strings.Fields(serverIPs)
	for _, serverIP := range ips {
		log.Printf("Restoring attachments on %s", serverIP)

		// Connect to the server
		client, err := connectToServer(serverIP, remoteUser, serverPassword)
		if err != nil {
			return fmt.Errorf("failed to connect via SSH to %s: %v", serverIP, err)
		}
		defer client.Close()

		// Create the home directory if it doesn't exist
		createDirCmd := fmt.Sprintf("sudo mkdir -p %s", homeDir)
		if err := executeRemoteCommand(client, createDirCmd); err != nil {
			return fmt.Errorf("failed to create home directory on %s: %v", serverIP, err)
		}

		// Stream the attachments restore file and extract it in the homeDir
		streamCmd := fmt.Sprintf(
			"sudo cat %s | sshpass -p '%s' ssh -o StrictHostKeyChecking=no %s@%s 'sudo tar xvzf - -C %s/attachments'",
			attachmentsRestoreFile, serverPassword, remoteUser, serverIP, homeDir,
		)
		if err := exec.Command("sh", "-c", streamCmd).Run(); err != nil {
			return fmt.Errorf("failed to stream and extract attachments on %s: %v", serverIP, err)
		}
		log.Printf("Attachments restored successfully on %s", serverIP)

		// Extract user and group from the server.xml file and adjust ownership
		ownershipCmd := fmt.Sprintf(
			"(if [ -f %s/conf/server.xml ]; then "+
				"user_group_info=$(ssh -o StrictHostKeyChecking=no %s@%s \"sudo stat -c '%%U %%G' %s/conf/server.xml\"); "+
				"read user group <<< \"$user_group_info\"; "+
				"if [ -n \"$user\" ] && [ -n \"$group\" ]; then "+
				"sudo chown -R \"$user:$group\" \"%s/attachments\"; "+
				"else echo 'User or group could not be determined.'; fi; "+
				"else echo '%s/conf/server.xml does not exist.'; fi) > /dev/null 2>&1",
			installDir, remoteUser, serverIP, installDir, homeDir, installDir,
		)
		if err := executeRemoteCommand(client, ownershipCmd); err != nil {
			return fmt.Errorf("failed to adjust ownership for Confluence attachments on %s: %v", serverIP, err)
		}

		log.Printf("Ownership adjusted successfully on %s", serverIP)
	}

	return nil
}

// RestoreAttachmentsOnEachServer handles the restoration process for attachments on each server under homeDir.
func RestoreAttachmentsOnEachServerJira(homeDir, installDir, attachmentsRestoreFile, remoteUser, serverIPs, serverPassword string) error {
	log.Printf("Starting attachments restoration on servers: %s", serverIPs)

	// Split serverIPs by space to handle multiple IPs
	ips := strings.Fields(serverIPs)
	for _, serverIP := range ips {
		log.Printf("Restoring attachments on %s", serverIP)

		client, err := connectToServer(serverIP, remoteUser, serverPassword)
		if err != nil {
			return fmt.Errorf("failed to connect via SSH to %s: %v", serverIP, err)
		}
		defer client.Close()

		// Stream the attachments restore file and extract it
		streamCmd := fmt.Sprintf(
			"sudo cat %s | sshpass -p '%s' ssh -o StrictHostKeyChecking=no %s@%s \"sudo tar xvzf - -C %s/data\"",
			attachmentsRestoreFile, serverPassword, remoteUser, serverIP, homeDir,
		)
		if err := exec.Command("sh", "-c", streamCmd).Run(); err != nil {
			return fmt.Errorf("failed to stream and extract attachments on %s: %v", serverIP, err)
		}

		log.Printf("Attachments restored successfully on %s", serverIP)
		// Adjust ownership of files
		ownershipCmd := fmt.Sprintf(
			"(if [ -f %s/conf/server.xml ]; then "+
				"user_group_info=$(ssh -o StrictHostKeyChecking=no %s@%s \"sudo stat -c '%%U %%G' %s/conf/server.xml\"); "+
				"read user group <<< \"$user_group_info\"; "+
				"if [ -n \"$user\" ] && [ -n \"$group\" ]; then "+
				"sudo chown -R \"$user:$group\" \"%s/data\"; "+
				"else echo 'User or group could not be determined.'; fi; "+
				"else echo '%s/conf/server.xml does not exist.'; fi) > /dev/null 2>&1",
			installDir, remoteUser, serverIP, installDir, installDir, installDir,
		)
		if err := executeRemoteCommand(client, ownershipCmd); err != nil {
			return fmt.Errorf("failed to adjust ownership for NFS restore on %s: %v", serverIP, err)
		}
	}
	return nil
}

// RestoreSharedHomeDir handles the restoration process for a shared home directory (NFS).
func RestoreSharedHomeDirJira(sharedHomeDir, installDir, dataRestoreFile, dataCurrentBackup, dataFolder, remoteUser, serverIPs, serverPassword string) error {
	log.Printf("Starting Attachments sharedhome directory restore on NFS at %s/data", sharedHomeDir)

	// Split serverIPs by space to handle multiple IPs, but we use only the first one for NFS restore
	ips := strings.Fields(serverIPs)
	if len(ips) == 0 {
		return fmt.Errorf("no server IPs provided for NFS restore")
	}
	serverIP := ips[0] // Use only the first server IP for NFS restore

	// Connect to the first server
	client, err := connectToServer(serverIP, remoteUser, serverPassword)
	if err != nil {
		return fmt.Errorf("failed to connect via SSH to %s: %v", serverIP, err)
	}
	defer client.Close()

	commands := []string{
		fmt.Sprintf("if [ ! -d %s ]; then sudo mkdir -p %s; fi", dataCurrentBackup, dataCurrentBackup),
		fmt.Sprintf("sudo rsync -av --remove-source-files %s/data/ %s/data/", sharedHomeDir, dataCurrentBackup),
		fmt.Sprintf("sudo mkdir -p %s/data/", sharedHomeDir),
	}

	if err := executeCommands(client, commands); err != nil {
		return fmt.Errorf("failed to prepare Data folder for restore on %s: %v", serverIP, err)
	}

	// Stream the NFS restore file and extract it
	streamCmd := fmt.Sprintf(
		"sudo cat %s | sshpass -p '%s' ssh -o StrictHostKeyChecking=no %s@%s 'sudo tar xvzf - -C %s/data'",
		dataRestoreFile, serverPassword, remoteUser, serverIP, sharedHomeDir,
	)
	if err := exec.Command("sh", "-c", streamCmd).Run(); err != nil {
		return fmt.Errorf("failed to stream and extract data backup on %s: %v", serverIP, err)
	}

	// Adjust ownership of files
	ownershipCmd := fmt.Sprintf(
		"(if [ -f %s/conf/server.xml ]; then "+
			"user_group_info=$(ssh -o StrictHostKeyChecking=no %s@%s \"sudo stat -c '%%U %%G' %s/conf/server.xml\"); "+
			"read user group <<< \"$user_group_info\"; "+
			"if [ -n \"$user\" ] && [ -n \"$group\" ]; then "+
			"sudo chown -R \"$user:$group\" \"%s/export\" \"%s/import\" \"%s/log\" \"%s/eazybi.toml\"; "+
			"else echo 'User or group could not be determined.'; fi; "+
			"else echo '%s/conf/server.xml does not exist.'; fi) > /dev/null 2>&1",
		installDir, remoteUser, serverIP, installDir, sharedHomeDir, sharedHomeDir, sharedHomeDir, sharedHomeDir, installDir,
	)
	if err := executeRemoteCommand(client, ownershipCmd); err != nil {
		return fmt.Errorf("failed to adjust ownership for NFS restore on %s: %v", serverIP, err)
	}

	log.Printf("Shared home directory restored successfully on NFS server: %s", serverIP)
	return nil
}

// RestoreSharedHomeDir handles the restoration process for a shared home directory (NFS).
func RestoreNFSDirJira(sharedHomeDir, installDir, NFSRestoreFile, dataCurrentBackup, dataFolder, remoteUser, serverIPs, serverPassword string) error {
	log.Printf("Starting sharedhome directory restore on NFS at %s", sharedHomeDir)

	// Split serverIPs by space to handle multiple IPs, but we use only the first one for NFS restore
	ips := strings.Fields(serverIPs)
	if len(ips) == 0 {
		return fmt.Errorf("no server IPs provided for NFS restore")
	}
	serverIP := ips[0] // Use only the first server IP for NFS restore

	// Connect to the first server
	client, err := connectToServer(serverIP, remoteUser, serverPassword)
	if err != nil {
		return fmt.Errorf("failed to connect via SSH to %s: %v", serverIP, err)
	}
	defer client.Close()

	commands := []string{
		fmt.Sprintf("if [ ! -d %s ]; then sudo mkdir -p %s; fi", dataCurrentBackup, dataCurrentBackup),
		fmt.Sprintf("sudo rsync -av --remove-source-files --exclude='Data_Backup_*' --exclude='data/' --exclude='eazybi.toml' %s/ %s/", sharedHomeDir, dataCurrentBackup),
	}

	if err := executeCommands(client, commands); err != nil {
		return fmt.Errorf("failed to prepare Data folder for restore on %s: %v", serverIP, err)
	}

	// Stream the NFS restore file and extract it
	streamCmd := fmt.Sprintf(
		"sudo cat %s | sshpass -p '%s' ssh -o StrictHostKeyChecking=no %s@%s 'sudo tar xvzf - -C %s'",
		NFSRestoreFile, serverPassword, remoteUser, serverIP, sharedHomeDir,
	)
	if err := exec.Command("sh", "-c", streamCmd).Run(); err != nil {
		return fmt.Errorf("failed to stream and extract data backup on %s: %v", serverIP, err)
	}

	// Adjust ownership of files
	ownershipCmd := fmt.Sprintf(
		"(if [ -f %s/conf/server.xml ]; then "+
			"user_group_info=$(ssh -o StrictHostKeyChecking=no %s@%s \"sudo stat -c '%%U %%G' %s/conf/server.xml\"); "+
			"read user group <<< \"$user_group_info\"; "+
			"if [ -n \"$user\" ] && [ -n \"$group\" ]; then "+
			"sudo chown -R \"$user:$group\" \"%s/export\" \"%s/import\" \"%s/log\" \"%s/eazybi.toml\"; "+
			"else echo 'User or group could not be determined.'; fi; "+
			"else echo '%s/conf/server.xml does not exist.'; fi) > /dev/null 2>&1",
		installDir, remoteUser, serverIP, installDir, sharedHomeDir, sharedHomeDir, sharedHomeDir, sharedHomeDir, installDir,
	)
	if err := executeRemoteCommand(client, ownershipCmd); err != nil {
		return fmt.Errorf("failed to adjust ownership for NFS restore on %s: %v", serverIP, err)
	}

	// Ensure the backup folder has root ownership after all operations
	finalOwnershipCmd := fmt.Sprintf("sudo chown -R root:root %s", dataCurrentBackup)
	if err := executeRemoteCommand(client, finalOwnershipCmd); err != nil {
		return fmt.Errorf("failed to adjust ownership of backup folder to root:root on %s: %v", serverIP, err)
	}

	log.Printf("Shared home directory restored successfully on NFS server: %s", serverIP)
	return nil
}

// RestoreSharedHomeDirConfluence handles the restoration process for a shared home directory (NFS) for Confluence.
func RestoreSharedHomeDirConfluence(sharedHomeDir, installDir, dataRestoreFile, dataCurrentBackup, dataFolder, remoteUser, serverIPs, serverPassword string) error {
	log.Printf("Starting Attachments sharedhome directory restore for Confluence on NFS at %s", sharedHomeDir)

	// Split serverIPs by space to handle multiple IPs, but we use only the first one for NFS restore
	ips := strings.Fields(serverIPs)
	if len(ips) == 0 {
		return fmt.Errorf("no server IPs provided for NFS restore")
	}
	serverIP := ips[0] // Use only the first server IP for NFS restore

	// Connect to the first server
	client, err := connectToServer(serverIP, remoteUser, serverPassword)
	if err != nil {
		return fmt.Errorf("failed to connect via SSH to %s: %v", serverIP, err)
	}
	defer client.Close()

	// Prepare backup directory and move existing data to backup
	commands := []string{
		fmt.Sprintf("if [ ! -d %s ]; then sudo mkdir -p %s; fi", dataCurrentBackup, dataCurrentBackup),
		fmt.Sprintf("sudo rsync -av --remove-source-files %s/attachments/ %s/attachments/", sharedHomeDir, dataCurrentBackup),
		fmt.Sprintf("sudo mkdir -p %s/attachments/", sharedHomeDir),
	}

	// Execute the commands on the remote server
	if err := executeCommands(client, commands); err != nil {
		return fmt.Errorf("failed to prepare shared directory for restore on %s: %v", serverIP, err)
	}

	// Stream the NFS restore file and extract it in the sharedHomeDir
	streamCmd := fmt.Sprintf(
		"sudo cat %s | sshpass -p '%s' ssh -o StrictHostKeyChecking=no %s@%s 'sudo tar xvzf - -C %s/attachments'",
		dataRestoreFile, serverPassword, remoteUser, serverIP, sharedHomeDir,
	)
	if err := exec.Command("sh", "-c", streamCmd).Run(); err != nil {
		return fmt.Errorf("failed to stream and extract data backup on %s: %v", serverIP, err)
	}

	// Adjust ownership of files using the user and group from server.xml
	ownershipCmd := fmt.Sprintf(
		"(if [ -f %s/conf/server.xml ]; then "+
			"user_group_info=$(ssh -o StrictHostKeyChecking=no %s@%s \"sudo stat -c '%%U %%G' %s/conf/server.xml\"); "+
			"read user group <<< \"$user_group_info\"; "+
			"if [ -n \"$user\" ] && [ -n \"$group\" ]; then "+
			"sudo chown -R \"$user:$group\" \"%s/attachments\" \"%s/logs\"; "+
			"else echo 'User or group could not be determined.'; fi; "+
			"else echo '%s/conf/server.xml does not exist.'; fi) > /dev/null 2>&1",
		installDir, remoteUser, serverIP, installDir, sharedHomeDir, sharedHomeDir, installDir,
	)
	if err := executeRemoteCommand(client, ownershipCmd); err != nil {
		return fmt.Errorf("failed to adjust ownership for NFS restore on %s: %v", serverIP, err)
	}

	log.Printf("Shared home directory for Confluence restored successfully on NFS server: %s", serverIP)
	return nil
}

// RestoreSharedHomeDirConfluence handles the restoration process for a shared home directory (NFS) for Confluence.
func RestoreNFSDirConfluence(sharedHomeDir, installDir, NFSRestoreFile, dataCurrentBackup, dataFolder, remoteUser, serverIPs, serverPassword string) error {
	log.Printf("Starting sharedhome directory restore for Confluence on NFS at %s", sharedHomeDir)

	// Split serverIPs by space to handle multiple IPs, but we use only the first one for NFS restore
	ips := strings.Fields(serverIPs)
	if len(ips) == 0 {
		return fmt.Errorf("no server IPs provided for NFS restore")
	}
	serverIP := ips[0] // Use only the first server IP for NFS restore

	// Connect to the first server
	client, err := connectToServer(serverIP, remoteUser, serverPassword)
	if err != nil {
		return fmt.Errorf("failed to connect via SSH to %s: %v", serverIP, err)
	}
	defer client.Close()

	// Prepare backup directory and move existing data to backup
	commands := []string{
		fmt.Sprintf("if [ ! -d %s ]; then sudo mkdir -p %s; fi", dataCurrentBackup, dataCurrentBackup),
		fmt.Sprintf("sudo rsync -av --remove-source-files --exclude='Data_Backup_*' --exclude='attachments/' %s/ %s/", sharedHomeDir, dataCurrentBackup),
	}

	// Execute the commands on the remote server
	if err := executeCommands(client, commands); err != nil {
		return fmt.Errorf("failed to prepare shared directory for restore on %s: %v", serverIP, err)
	}

	// Stream the NFS restore file and extract it in the sharedHomeDir
	streamCmd := fmt.Sprintf(
		"sudo cat %s | sshpass -p '%s' ssh -o StrictHostKeyChecking=no %s@%s 'sudo tar xvzf - -C %s'",
		NFSRestoreFile, serverPassword, remoteUser, serverIP, sharedHomeDir,
	)
	if err := exec.Command("sh", "-c", streamCmd).Run(); err != nil {
		return fmt.Errorf("failed to stream and extract data backup on %s: %v", serverIP, err)
	}

	// Adjust ownership of files using the user and group from server.xml
	ownershipCmd := fmt.Sprintf(
		"(if [ -f %s/conf/server.xml ]; then "+
			"user_group_info=$(ssh -o StrictHostKeyChecking=no %s@%s \"sudo stat -c '%%U %%G' %s/conf/server.xml\"); "+
			"read user group <<< \"$user_group_info\"; "+
			"if [ -n \"$user\" ] && [ -n \"$group\" ]; then "+
			"sudo chown -R \"$user:$group\" \"%s/attachments\" \"%s/logs\"; "+
			"else echo 'User or group could not be determined.'; fi; "+
			"else echo '%s/conf/server.xml does not exist.'; fi) > /dev/null 2>&1",
		installDir, remoteUser, serverIP, installDir, sharedHomeDir, sharedHomeDir, installDir,
	)
	if err := executeRemoteCommand(client, ownershipCmd); err != nil {
		return fmt.Errorf("failed to adjust ownership for NFS restore on %s: %v", serverIP, err)
	}

	// Ensure the backup folder has root ownership after all operations
	finalOwnershipCmd := fmt.Sprintf("sudo chown -R root:root %s", dataCurrentBackup)
	if err := executeRemoteCommand(client, finalOwnershipCmd); err != nil {
		return fmt.Errorf("failed to adjust ownership of backup folder to root:root on %s: %v", serverIP, err)
	}

	log.Printf("Shared home directory for Confluence restored successfully on NFS server: %s", serverIP)
	return nil
}
