package handlers

import (
	"golang.org/x/crypto/ssh"
	"log"
	"strings"
)

// getJiraInstallDir connects to the server and retrieves the Jira installation directory, home directory, and shared home directory
func getJiraInstallDir(ip, serverUser, serverPassword string) (string, string, string, error) {
	client, err := connectToServer(ip, serverUser, serverPassword)
	if err != nil {
		return "", "", "", err
	}
	defer client.Close()

	// Get Jira installation directory
	installDir, err := executeSSHCommand(client, "ps aux | grep '[j]ava' | grep 'jira' | head -n 1")
	if err != nil {
		log.Printf("Failed to execute command on server: %v", err)
		return "", "", "", err
	}

	installDir = extractInstallDir(installDir)
	if installDir == "" {
		return "", "", "", nil
	}

	// Get Jira home directory
	homeDirCmd := "grep '^jira.home' " + installDir + "/atlassian-jira/WEB-INF/classes/jira-application.properties | cut -d'=' -f2 | tr -d '[:space:]'"
	homeDir, err := executeSSHCommand(client, homeDirCmd)
	if err != nil {
		log.Printf("Failed to retrieve Jira home directory: %v", err)
		return "", "", "", err
	}

	// Get shared home directory (NFS directory)
	sharedHomeDir, err := extractSharedHomeDir(client, strings.TrimSpace(homeDir))
	if err != nil {
		log.Printf("Failed to retrieve shared home directory: %v", err)
	}

	return installDir, strings.TrimSpace(homeDir), sharedHomeDir, nil
}

// extractSharedHomeDir extracts the shared home directory (NFS directory) from the cluster.properties file
func extractSharedHomeDir(client *ssh.Client, dataFolder string) (string, error) {
	clusterFile := dataFolder + "/cluster.properties"
	cmd := "if [ -f " + clusterFile + " ]; then grep '^jira.shared.home' " + clusterFile + " | cut -d'=' -f2 | tr -d '[:space:]'; fi"
	sharedHomeDir, err := executeSSHCommand(client, cmd)
	if err != nil {
		return "", err
	}
	if sharedHomeDir == "" {
		log.Println("No shared home directory found (non-clustered environment).")
		return "", nil
	}
	return strings.TrimSpace(sharedHomeDir), nil
}

// executeSSHCommand executes a command on the server and returns the output
func executeSSHCommand(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		log.Printf("Failed to create SSH session: %v", err)
		return "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		log.Printf("Failed to execute command: %v", err)
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// extractInstallDir extracts the Jira installation directory from the process string
func extractInstallDir(process string) string {
	parts := strings.Fields(process)
	for _, part := range parts {
		if strings.Contains(part, "/opt/atlassian/jira") {
			return strings.Split(part, "/jira/")[0] + "/jira"
		}
	}
	return ""
}
