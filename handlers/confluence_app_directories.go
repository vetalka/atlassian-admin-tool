package handlers

import (
    "fmt"   // Add the fmt package for formatting
    "log"
    "strings"
)

// GetConfluenceInstallDir connects to the server and retrieves the Confluence installation directory and home directory
func GetConfluenceInstallDir(ip, serverUser, serverPassword string) (string, string, error) {
    client, err := connectToServer(ip, serverUser, serverPassword)
    if err != nil {
        return "", "", err
    }
    defer client.Close()

    // Get Confluence installation directory
    installDir, err := executeSSHCommand(client, "ps aux | grep '[j]ava' | grep 'confluence' | head -n 1")
    if err != nil {
        log.Printf("Failed to execute command on server: %v", err)
        return "", "", err
    }

    installDir = extractConfluenceInstallDir(installDir)
    if installDir == "" {
        return "", "", nil
    }

    // Get Confluence home directory
    homeDirCmd := "grep '^confluence.home' " + installDir + "/confluence/WEB-INF/classes/confluence-init.properties | cut -d'=' -f2 | tr -d '[:space:]'"
    homeDir, err := executeSSHCommand(client, homeDirCmd)
    if err != nil {
        log.Printf("Failed to retrieve Confluence home directory: %v", err)
        return "", "", err
    }

    return installDir, strings.TrimSpace(homeDir), nil
}

// extractConfluenceInstallDir extracts the Confluence installation directory from the process string
func extractConfluenceInstallDir(process string) string {
    parts := strings.Fields(process)
    for _, part := range parts {
        if strings.Contains(part, "/opt/atlassian/confluence") {
            return strings.Split(part, "/confluence")[0] + "/confluence"
        }
    }
    return ""
}

// ExtractDataAndHomeFolders splits the confluence home directory into data path and home folder
func ExtractDataAndHomeFolders(homeDir string) (string, string) {
    dataPath := homeDir[:strings.LastIndex(homeDir, "/")]
    homeFolder := homeDir[strings.LastIndex(homeDir, "/")+1:]
    return dataPath, homeFolder
}

// extractSharedHomeDir connects to the server and extracts the shared home directory from confluence.cfg.xml
func extractSharedHomeDirConfluence(ip, serverUser, serverPassword, dataFolder string) (string, error) {
    client, err := connectToServer(ip, serverUser, serverPassword)
    if err != nil {
        return "", fmt.Errorf("failed to connect to server: %v", err)
    }
    defer client.Close()

    // Define the path to the confluence.cfg.xml file
    clusterFile := fmt.Sprintf("%s/confluence.cfg.xml", dataFolder)

    // Check if the confluence.cfg.xml file exists
    checkFileCmd := fmt.Sprintf("if [ -f %s ]; then echo 'exists'; else echo 'not exists'; fi", clusterFile)
    output, err := executeSSHCommand(client, checkFileCmd)
    if err != nil {
        return "", fmt.Errorf("failed to check confluence.cfg.xml file: %v", err)
    }

    if strings.TrimSpace(output) == "not exists" {
        log.Println("Your environment currently does not support a shared home directory (non-clustered).")
        return "", nil
    }

    // Extract the NFS directory path from confluence.cfg.xml
    extractNFSDirCmd := fmt.Sprintf(`sudo grep -oP '(?<=<property name="confluence.cluster.home">)[^<]+' %s`, clusterFile)
    nfsDir, err := executeSSHCommand(client, extractNFSDirCmd)
    if err != nil {
        return "", fmt.Errorf("failed to extract NFS directory: %v", err)
    }

    if strings.TrimSpace(nfsDir) == "" {
        log.Println("Clustered environment detected, but no NFS directory found.")
        return "", nil
    }

    log.Printf("Clustered environment detected. NFS Directory: %s", nfsDir)
    return strings.TrimSpace(nfsDir), nil
}
