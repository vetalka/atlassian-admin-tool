package handlers

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// executeRemoteCommand runs a single command on the remote server via SSH.
func executeRemoteCommand(client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	// Capture the command output and errors
	var output, stderr strings.Builder
	session.Stdout = &output
	session.Stderr = &stderr

	// Run the command
	if err := session.Run(command); err != nil {
		log.Printf("Command failed: %s", stderr.String())
		return fmt.Errorf("failed to run command: %v", err)
	}

	log.Printf("Command output: %s", output.String())
	return nil
}

// executeCommands runs a list of commands on the remote server via SSH.
func executeCommands1(client *ssh.Client, commands []string) error {
	for _, cmd := range commands {
		if err := executeRemoteCommand(client, cmd); err != nil {
			return err
		}
	}
	return nil
}

// RestoreLocalHomeDir dynamically handles the restoration process for either Jira or Confluence.
func RestoreLocalHomeDir(appType, homeDir, backupDirHome, installDir, remoteUser, serverIPs, serverPassword, tempRestoreFolder string) error {
	// Determine which application type is being restored and call the appropriate function
	switch appType {
	case "Jira":
		log.Printf("Detected application type: Jira")
		return RestoreLocalHomeDirJira(homeDir, backupDirHome, installDir, remoteUser, serverIPs, serverPassword, tempRestoreFolder)
	case "Confluence":
		log.Printf("Detected application type: Confluence")
		return RestoreLocalHomeDirConfluence(homeDir, backupDirHome, installDir, remoteUser, serverIPs, serverPassword, tempRestoreFolder)
	default:
		return fmt.Errorf("unsupported application type: %s", appType)
	}
}

// RestoreLocalHomeDir handles the restoration process for the local home directory.
func RestoreLocalHomeDirJira(homeDir, backupDirHome, installDir, remoteUser, serverIPs, serverPassword, tempRestoreFolder string) error {
	log.Printf("Starting Local Home Directory Restore Process on servers: %s", serverIPs)

	// Split serverIPs by space to handle multiple IPs
	ips := strings.Fields(serverIPs)
	for _, serverIP := range ips {
		log.Printf("Restoring Local Home Directory on %s", serverIP)

		// Use the connectToServer function from ssh_utils.go
		client, err := connectToServer(serverIP, remoteUser, serverPassword)
		if err != nil {
			return fmt.Errorf("failed to connect via SSH to %s: %v", serverIP, err)
		}
		defer client.Close()

		// First, rename the existing home directory with the current date-time
		renameCmd := fmt.Sprintf("sudo mv %s %s", homeDir, backupDirHome)
		if err := executeRemoteCommand(client, renameCmd); err != nil {
			return fmt.Errorf("failed to rename home directory on %s: %v", serverIP, err)
		}
		log.Printf("Renamed existing home directory on %s", serverIP)

		// Create home directory
		createmeCmd := fmt.Sprintf("sudo mkdir -p %s", homeDir)
		if err := executeRemoteCommand(client, createmeCmd); err != nil {
			return fmt.Errorf("failed to create home directory on %s: %v", serverIP, err)
		}
		log.Printf("Create home directory on %s", serverIP)

		// Stream the backup file directly to the remote server and extract it
		localBackupPath := filepath.Join(tempRestoreFolder, "JiraHomeBackup.tar.gz")

		// Correct the SSH command to use sshpass
		streamCmd := fmt.Sprintf(
			"cat %s | sshpass -p '%s' ssh -o StrictHostKeyChecking=no %s@%s 'sudo tar -xzvf - -C %s'",
			localBackupPath, serverPassword, remoteUser, serverIP, homeDir,
		)

		// Execute the streaming command
		cmd := exec.Command("sh", "-c", streamCmd)
		cmd.Stdout = log.Writer() // Redirect standard output to logs
		cmd.Stderr = log.Writer() // Redirect standard error to logs

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to stream and extract backup file on %s: %v", serverIP, err)
		}

		log.Printf("Backup file streamed and extracted successfully on %s", serverIP)

		commands := []string{
			// Remove old configuration files
			fmt.Sprintf("sudo rm -rf %s/dbconfig.xml", homeDir),
			// Copy the configuration files back to the home directory
			fmt.Sprintf("sudo cp -r %s/dbconfig.xml %s", backupDirHome, homeDir),

			// Remove and copy configuration files only if they exist
			fmt.Sprintf("if [ -f %s/azybi.toml ]; then sudo rm -rf %s/azybi.toml && sudo cp -r %s/azybi.toml %s; fi", homeDir, homeDir, backupDirHome, homeDir),
			fmt.Sprintf("if [ -f %s/cluster.properties ]; then sudo rm -rf %s/cluster.properties && sudo cp -r %s/cluster.properties %s; fi", homeDir, homeDir, backupDirHome, homeDir),
			fmt.Sprintf("if [ -f %s/jira-config.properties ]; then sudo rm -rf %s/jira-config.properties && sudo cp -r %s/jira-config.properties %s; fi", homeDir, homeDir, backupDirHome, homeDir),
			fmt.Sprintf("if [ -d %s/kerberos ]; then sudo rm -rf %s/kerberos && sudo cp -r %s/kerberos %s; fi", homeDir, homeDir, backupDirHome, homeDir),

			// Extract keystore file path from server.xml and copy the certificate file if it exists
			fmt.Sprintf("(if [ -f %s/conf/server.xml ]; then "+
				"KEYS_FILE_PATH=$(sudo grep 'keystoreFile=' %s/conf/server.xml | sed -e 's/.*keystoreFile=\"\\([^\\\"]*\\)\".*/\\1/'); "+
				"echo \"Keystore File Path: $KEYS_FILE_PATH\"; "+
				"CERT_FILE_NAME=$(basename \"$KEYS_FILE_PATH\"); "+
				"echo \"Certificate File Name: $CERT_FILE_NAME\"; "+
				"RELATIVE_KEY_PATH=${KEYS_FILE_PATH#%s/}; "+
				"echo \"Relative Keystore Path: $RELATIVE_KEY_PATH\"; "+
				"BACKUP_CERT_FILE=%s/$RELATIVE_KEY_PATH; "+
				"if [ -f \"$BACKUP_CERT_FILE\" ]; then "+
				"DEST_DIR=$(dirname \"$KEYS_FILE_PATH\"); "+
				"sudo mkdir -p \"$DEST_DIR\"; "+
				"sudo cp \"$BACKUP_CERT_FILE\" \"$DEST_DIR/\" && echo \"Certificate file copied successfully.\" || echo \"Error in copying certificate file.\"; "+
				"else echo \"Backup certificate file not found at $BACKUP_CERT_FILE\"; "+
				"fi; "+
				"else echo \"%s/conf/server.xml does not exist.\"; fi) > /dev/null 2>&1",
				installDir, installDir, homeDir, backupDirHome, installDir),

			// Extract user and group from the server.xml file and change ownership
			fmt.Sprintf("if [ -f %s/conf/server.xml ]; then "+
				"user_group_info=$(ls -l %s/conf/server.xml | awk '{print $3, $4}'); "+
				"read user group <<< \"$user_group_info\"; "+
				"echo \"User: $user, Group: $group\"; "+
				"if [ -n \"$user\" ] && [ -n \"$group\" ]; then "+
				"sudo chown -R \"$user:$group\" \"%s\" && echo \"Ownership changed successfully on %s.\" || echo \"Failed to change ownership on %s.\"; "+
				"else echo \"User or group could not be determined.\"; "+
				"fi; "+
				"else echo \"%s/conf/server.xml does not exist.\"; fi",
				installDir, installDir, homeDir, homeDir, homeDir, installDir),
		}

		// Execute the remaining commands
		if err := executeCommands1(client, commands); err != nil {
			return fmt.Errorf("failed to restore local home directory on %s: %v", serverIP, err)
		}

		log.Printf("Local Home Directory restored successfully on %s", serverIP)
	}

	return nil
}

// RestoreLocalHomeDir handles the restoration process for the local home directory.
func RestoreLocalHomeDirConfluence(homeDir, backupDirHome, installDir, remoteUser, serverIPs, serverPassword, tempRestoreFolder string) error {
	log.Printf("Starting Local Home Directory Restore Process on servers: %s", serverIPs)

	// Split serverIPs by space to handle multiple IPs
	ips := strings.Fields(serverIPs)
	for _, serverIP := range ips {
		log.Printf("Restoring Local Home Directory on %s", serverIP)

		// Use the connectToServer function from ssh_utils.go
		client, err := connectToServer(serverIP, remoteUser, serverPassword)
		if err != nil {
			return fmt.Errorf("failed to connect via SSH to %s: %v", serverIP, err)
		}
		defer client.Close()

		// First, rename the existing home directory with the current date-time
		renameCmd := fmt.Sprintf("sudo mv %s %s", homeDir, backupDirHome)
		if err := executeRemoteCommand(client, renameCmd); err != nil {
			return fmt.Errorf("failed to rename home directory on %s: %v", serverIP, err)
		}
		log.Printf("Renamed existing home directory on %s", serverIP)

		// Create home directory
		createHomeCmd := fmt.Sprintf("sudo mkdir -p %s", homeDir)
		if err := executeRemoteCommand(client, createHomeCmd); err != nil {
			return fmt.Errorf("failed to create home directory on %s: %v", serverIP, err)
		}
		log.Printf("Home directory created on %s", serverIP)

		// Stream and extract the backup file directly to the remote server
		localBackupPath := filepath.Join(tempRestoreFolder, "ConfluenceHomeBackup.tar.gz")

		streamCmd := fmt.Sprintf(
			"cat %s | sshpass -p '%s' ssh -o StrictHostKeyChecking=no %s@%s 'sudo tar -xzvf - -C %s'",
			localBackupPath, serverPassword, remoteUser, serverIP, homeDir,
		)

		cmd := exec.Command("sh", "-c", streamCmd)
		cmd.Stdout = log.Writer()
		cmd.Stderr = log.Writer()

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to stream and extract backup file on %s: %v", serverIP, err)
		}
		log.Printf("Backup file streamed and extracted successfully on %s", serverIP)

		// Commands to remove old config files and restore the new ones
		commands := []string{
			// Remove old configuration files
			fmt.Sprintf("sudo rm -rf %s/confluence.cfg.xml", homeDir),
			fmt.Sprintf("sudo cp -r %s/confluence.cfg.xml %s", backupDirHome, homeDir),

			// Remove and copy kerberos folder if it exists
			fmt.Sprintf("if [ -d %s/kerberos ]; then sudo rm -rf %s/kerberos && sudo cp -r %s/kerberos %s; fi", homeDir, homeDir, backupDirHome, homeDir),

			// Extract keystore file path from server.xml and copy the certificate file if it exists
			fmt.Sprintf(`(if [ -f %s/conf/server.xml ]; then 
				KEYS_FILE_PATH=$(sudo grep 'keystoreFile=' %s/conf/server.xml | sed -e 's/.*keystoreFile="\\([^\\"]*\\)".*/\\1/'); 
				CERT_FILE_NAME=$(basename "$KEYS_FILE_PATH");
				RELATIVE_KEY_PATH=${KEYS_FILE_PATH#%s/};
				BACKUP_CERT_FILE=%s/$RELATIVE_KEY_PATH;
				if [ -f "$BACKUP_CERT_FILE" ]; then 
					DEST_DIR=$(dirname "$KEYS_FILE_PATH"); 
					sudo mkdir -p "$DEST_DIR"; 
					sudo cp "$BACKUP_CERT_FILE" "$DEST_DIR/" && echo "Certificate file copied successfully." || echo "Error in copying certificate file.";
				else echo "Backup certificate file not found at $BACKUP_CERT_FILE"; fi;
				else echo "%s/conf/server.xml does not exist."; fi) > /dev/null 2>&1`,
				installDir, installDir, homeDir, backupDirHome, installDir),

			// Change ownership of the restored files based on user and group from server.xml
			fmt.Sprintf(`if [ -f %s/conf/server.xml ]; then 
				user_group_info=$(ls -l %s/conf/server.xml | awk '{print $3, $4}');
				read user group <<< "$user_group_info";
				if [ -n "$user" ] && [ -n "$group" ]; then 
					sudo chown -R "$user:$group" "%s" && echo "Ownership changed successfully on %s." || echo "Failed to change ownership on %s."; 
				else echo "User or group could not be determined."; 
				fi; 
				else echo "%s/conf/server.xml does not exist."; fi`,
				installDir, installDir, homeDir, homeDir, homeDir, installDir),
		}

		// Execute the remaining commands
		if err := executeCommands1(client, commands); err != nil {
			return fmt.Errorf("failed to restore local home directory on %s: %v", serverIP, err)
		}

		log.Printf("Local Home Directory restored successfully on %s", serverIP)
	}

	return nil
}
