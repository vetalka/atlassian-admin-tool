package handlers

import (
    "fmt"
    "log"
    "strings"
    "golang.org/x/crypto/ssh"
    "regexp"
)

// checkIfFileExists checks if a specific file exists on the remote server
func checkIfFileExists(client *ssh.Client, filePath string) (bool, error) {
    cmd := fmt.Sprintf("sudo ls -l '%s' > /dev/null 2>&1", filePath)
    _, err := executeSSHCommand(client, cmd)
    if err != nil {
        if strings.Contains(err.Error(), "No such file or directory") {
            return false, nil // File not found
        }
        return false, err
    }
    return true, nil
}

// extractEazyBIParam is a helper function to extract a specific key's value from the eazybi.toml file under the [database] section using awk
func extractEazyBIParam(client *ssh.Client, key, file string) (string, error) {
    cmd := fmt.Sprintf("sudo awk -F '=' '/\\[database\\]/ {found=1} found && $1 ~ /^%s/ {gsub(/[ \"]/, \"\", $2); print $2; exit}' '%s'", key, file)
    log.Printf("Executing command to extract %s: %s", key, cmd) // Log the command being executed

    value, err := executeSSHCommand(client, cmd)
    if err != nil {
        log.Printf("Error executing command to extract %s: %v", key, err)
        return "", err
    }
    log.Printf("Raw output for %s: %s", key, value) // Log the raw output of the command

    return strings.TrimSpace(value), nil
}

// extractEazyBIDBParams extracts EazyBI database parameters; if the file is not found, it returns empty values
func extractEazyBIDBParams(client *ssh.Client, dataFolder, sharedHomeDir string) (eazybiDBName, eazybiDBHost, eazybiDBPass, eazybiDBPort, eazybiDBUser string, err error) {
    var eazybiFile string
    var exists bool

    // Check if the shared home directory is defined and eazybi.toml exists there
    if sharedHomeDir != "" {
        eazybiFile = fmt.Sprintf("%s/eazybi.toml", sharedHomeDir)
        exists, err = checkIfFileExists(client, eazybiFile)
        if err != nil || !exists {
            log.Printf("eazybi.toml not found at %s; skipping EazyBI parameter extraction.", eazybiFile)
            return "", "", "", "", "", nil // Skip extraction if file not found
        }
    } else {
        // Fallback to the data folder if shared home directory is not defined
        eazybiFile = fmt.Sprintf("%s/eazybi.toml", dataFolder)
        exists, err = checkIfFileExists(client, eazybiFile)
        if err != nil || !exists {
            log.Printf("eazybi.toml not found in %s; skipping EazyBI parameter extraction.", dataFolder)
            return "", "", "", "", "", nil // Skip extraction if file not found
        }
    }

    // Extract the parameters from the eazybi.toml file under [database]
    eazybiDBName, err = extractEazyBIParam(client, "database", eazybiFile)
    if err != nil {
        log.Printf("Failed to extract EazyBI DB Name: %v", err)
        eazybiDBName = ""
    }

    eazybiDBHost, err = extractEazyBIParam(client, "host", eazybiFile)
    if err != nil {
        log.Printf("Failed to extract EazyBI DB Host: %v", err)
        eazybiDBHost = ""
    }

    // Improved password extraction logic using awk
    eazybiDBPass, err = extractEazyBIParam(client, "password", eazybiFile)
    if err != nil {
        log.Printf("Failed to extract EazyBI DB Password: %v", err)
        eazybiDBPass = ""
    } else {
        eazybiDBPass = strings.TrimSpace(eazybiDBPass)
    }

    eazybiDBPort, err = extractEazyBIParam(client, "port", eazybiFile)
    if err != nil {
        log.Printf("Failed to extract EazyBI DB Port: %v", err)
        eazybiDBPort = ""
    } else {
        if !isPortValid(eazybiDBPort) {
            log.Printf("Invalid port detected: %s", eazybiDBPort)
            eazybiDBPort = ""
        }
    }

    eazybiDBUser, err = extractEazyBIParam(client, "username", eazybiFile)
    if err != nil {
        log.Printf("Failed to extract EazyBI DB User: %v", err)
        eazybiDBUser = ""
    }

    // Log the extracted values for debugging purposes
    log.Printf("Extracted EazyBI DB Name: %s", eazybiDBName)
    log.Printf("Extracted EazyBI DB Host: %s", eazybiDBHost)
    log.Printf("Extracted EazyBI DB Password: %s", eazybiDBPass)
    log.Printf("Extracted EazyBI DB Port: %s", eazybiDBPort)
    log.Printf("Extracted EazyBI DB User: %s", eazybiDBUser)

    return strings.TrimSpace(eazybiDBName), strings.TrimSpace(eazybiDBHost), strings.TrimSpace(eazybiDBPass), strings.TrimSpace(eazybiDBPort), strings.TrimSpace(eazybiDBUser), nil
}

// isPortValid ensures that the extracted port is valid and consists only of digits
func isPortValid(port string) bool {
    if port == "" {
        return false
    }
    matched, _ := regexp.MatchString(`^\d+$`, port)
    return matched && port != "3801"
}

// executeEazyBIDBParamsExtraction connects to the server and extracts the EazyBI database parameters
func executeEazyBIDBParamsExtraction(ip, serverUser, serverPassword, dataFolder, sharedHomeDir string) (eazybiDBName, eazybiDBHost, eazybiDBPass, eazybiDBPort, eazybiDBUser string, err error) {
    client, err := connectToServer(ip, serverUser, serverPassword)
    if err != nil {
        return "", "", "", "", "", err
    }
    defer client.Close()

    return extractEazyBIDBParams(client, dataFolder, sharedHomeDir)
}
