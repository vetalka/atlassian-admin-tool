package handlers

import (
    "fmt"
    "strings"
    "regexp"
    "golang.org/x/crypto/ssh"
)

// getDBParamsPostgresql extracts the database parameters for PostgreSQL
func getDBParamsPostgresql(client *ssh.Client, dataFolder, installDir string) (dbHost, dbPort, dbName, dbUser, dbPass, dbDriver string, err error) {
    dbConfigFile := fmt.Sprintf("%s/dbconfig.xml", dataFolder)
    jdbcURL, err := executeSSHCommand(client, fmt.Sprintf("sudo grep -oP 'jdbc:postgresql://[^<]+' %s", dbConfigFile))
    if err != nil {
        return "", "", "", "", "", "", err
    }

    dbHost = extractValue(jdbcURL, `jdbc:postgresql://([^:]+)`)
    dbPort = extractValue(jdbcURL, `:(\d+)`)
    dbName = extractValue(jdbcURL, `/([^/]+)$`)

    dbUser, err = executeSSHCommand(client, fmt.Sprintf("sudo grep -oP '<username>\\K[^<]+' '%s'", dbConfigFile))
    if err != nil {
        return "", "", "", "", "", "", err
    }

    dbDriver, err = executeSSHCommand(client, fmt.Sprintf("sudo grep -oP '<driver-class>\\K[^<]+' '%s'", dbConfigFile))
    if err != nil {
        return "", "", "", "", "", "", err
    }

    encryptedPassMarker := "com.atlassian.db.config.password.ciphers.base64.Base64Cipher"
    encryptedPassExists, err := checkForEncryptedPassword(client, dbConfigFile, encryptedPassMarker)
    if err != nil {
        return "", "", "", "", "", "", err
    }

    if encryptedPassExists {
        dbPass, err = decryptPassword(client, dbConfigFile, installDir)
        if err != nil {
            // Decryption failed — return marker so user can enter manually
            return strings.TrimSpace(dbHost), strings.TrimSpace(dbPort), strings.TrimSpace(dbName), strings.TrimSpace(dbUser), "{ATL_SECURED}", strings.TrimSpace(dbDriver), nil
        }
    } else {
        dbPass, err = executeSSHCommand(client, fmt.Sprintf("sudo grep -oP '<password>\\K[^<]+' '%s'", dbConfigFile))
        if err != nil {
            // Password might be empty or missing
            dbPass = ""
        }
        dbPass = strings.TrimSpace(dbPass)
        // If password is {ATL_SECURED} or {ENCRYPTED}, return as marker
        if strings.Contains(dbPass, "{ATL_SECURED}") || strings.Contains(dbPass, "{ENCRYPTED}") {
            return strings.TrimSpace(dbHost), strings.TrimSpace(dbPort), strings.TrimSpace(dbName), strings.TrimSpace(dbUser), dbPass, strings.TrimSpace(dbDriver), nil
        }
    }

    return strings.TrimSpace(dbHost), strings.TrimSpace(dbPort), strings.TrimSpace(dbName), strings.TrimSpace(dbUser), strings.TrimSpace(dbPass), strings.TrimSpace(dbDriver), nil
}

// extractValue extracts a value using a regex pattern
func extractValue(text, pattern string) string {
    re := regexp.MustCompile(pattern)
    match := re.FindStringSubmatch(text)
    if len(match) > 1 {
        return match[1]
    }
    return ""
}

// Helper function to check if the password is encrypted
func checkForEncryptedPassword(client *ssh.Client, dbConfigFile, encryptedPassMarker string) (bool, error) {
    cmd := fmt.Sprintf("sudo grep -q '%s' '%s'", encryptedPassMarker, dbConfigFile)
    _, err := executeSSHCommand(client, cmd)
    if err != nil {
        return false, nil // No encrypted password found
    }
    return true, nil
}

// Helper function to decrypt the password
func decryptPassword(client *ssh.Client, dbConfigFile, installDir string) (string, error) {
    encryptedPass, err := executeSSHCommand(client, fmt.Sprintf("sudo grep -oP '<password>\\K[^<]+' '%s'", dbConfigFile))
    if err != nil {
        return "", err
    }

    installDirCmd := fmt.Sprintf("cd %s/bin && sudo ./jre/bin/java -cp './*' com.atlassian.db.config.password.tools.CipherTool -m decrypt -p '%s'", installDir, encryptedPass)
    decryptedPassOutput, err := executeSSHCommand(client, installDirCmd)
    if err != nil {
        return "", err
    }

    decryptedPass := extractValue(decryptedPassOutput, `password: (.+)`)
    if decryptedPass == "" {
        return "", fmt.Errorf("failed to decrypt the password")
    }

    return decryptedPass, nil
}

// getDBParamsSQLServer extracts the database parameters for SQL Server
func getDBParamsSQLServer(client *ssh.Client, dataFolder, installDir string) (dbHost, dbPort, dbName, dbUser, dbPass, dbDriver string, err error) {
    dbConfigFile := fmt.Sprintf("%s/dbconfig.xml", dataFolder)
    jdbcURL, err := executeSSHCommand(client, fmt.Sprintf("sudo grep -oP 'jdbc:sqlserver://[^<]+' %s", dbConfigFile))
    if err != nil {
        return "", "", "", "", "", "", err
    }

    dbHost = extractValue(jdbcURL, `serverName=([^;\\]+)`)
    dbPort = extractValue(jdbcURL, `portNumber=(\d+)`)
    dbName = extractValue(jdbcURL, `databaseName=([^;]+)`)

    dbUser, err = executeSSHCommand(client, fmt.Sprintf("sudo grep -oP '<username>\\K[^<]+' '%s'", dbConfigFile))
    if err != nil {
        return "", "", "", "", "", "", err
    }

    dbDriver, err = executeSSHCommand(client, fmt.Sprintf("sudo grep -oP '<driver-class>\\K[^<]+' '%s'", dbConfigFile))
    if err != nil {
        return "", "", "", "", "", "", err
    }

    // Extract the raw password value first
    rawPass, err := executeSSHCommand(client, fmt.Sprintf("sudo grep -oP '<password>\\K[^<]+' '%s'", dbConfigFile))
    if err != nil {
        // Password tag might be empty or missing — return empty password
        return strings.TrimSpace(dbHost), strings.TrimSpace(dbPort), strings.TrimSpace(dbName), strings.TrimSpace(dbUser), "", strings.TrimSpace(dbDriver), nil
    }
    rawPass = strings.TrimSpace(rawPass)

    // If password is {ATL_SECURED} or {ENCRYPTED}, it can't be read from the file
    if strings.Contains(rawPass, "{ATL_SECURED}") || strings.Contains(rawPass, "{ENCRYPTED}") {
        return strings.TrimSpace(dbHost), strings.TrimSpace(dbPort), strings.TrimSpace(dbName), strings.TrimSpace(dbUser), rawPass, strings.TrimSpace(dbDriver), nil
    }

    // Check for Base64 cipher encrypted password
    encryptedPassMarker := "com.atlassian.db.config.password.ciphers.base64.Base64Cipher"
    encryptedPassExists, _ := checkForEncryptedPassword(client, dbConfigFile, encryptedPassMarker)

    if encryptedPassExists {
        dbPass, err = decryptPassword(client, dbConfigFile, installDir)
        if err != nil {
            // Decryption failed — return marker so user can enter manually
            return strings.TrimSpace(dbHost), strings.TrimSpace(dbPort), strings.TrimSpace(dbName), strings.TrimSpace(dbUser), "{ATL_SECURED}", strings.TrimSpace(dbDriver), nil
        }
    } else {
        dbPass = rawPass
    }

    return strings.TrimSpace(dbHost), strings.TrimSpace(dbPort), strings.TrimSpace(dbName), strings.TrimSpace(dbUser), strings.TrimSpace(dbPass), strings.TrimSpace(dbDriver), nil
}

// getDBParams retrieves the database parameters based on the database type
func getDBParams(client *ssh.Client, dataFolder, installDir, dbKind string) (dbHost, dbPort, dbName, dbUser, dbPass, dbDriver string, err error) {
    switch dbKind {
    case "postgresql":
        return getDBParamsPostgresql(client, dataFolder, installDir)
    case "sqlserver":
        return getDBParamsSQLServer(client, dataFolder, installDir)
    default:
        return "", "", "", "", "", "", fmt.Errorf("unsupported database type: %s", dbKind)
    }
}

// executeDBParamsExtraction connects to the server and extracts the database parameters
func executeDBParamsExtraction(ip, serverUser, serverPassword, dataFolder, installDir string) (dbHost, dbPort, dbName, dbUser, dbPass, dbDriver string, err error) {
    client, err := connectToServer(ip, serverUser, serverPassword)
    if err != nil {
        return "", "", "", "", "", "", err
    }
    defer client.Close()

    dbConfigFile := fmt.Sprintf("%s/dbconfig.xml", dataFolder)

    // Try to detect DB kind - multiple approaches for compatibility
    dbKind, err := executeSSHCommand(client, fmt.Sprintf("sudo grep -oP '<url>\\K[^<]+' '%s' | grep -oP 'jdbc:\\K[^:]+'", dbConfigFile))
    if err != nil || strings.TrimSpace(dbKind) == "" {
        // Fallback: use sed instead of grep -P (more portable)
        dbKind, err = executeSSHCommand(client, fmt.Sprintf("sudo sed -n 's/.*<url>jdbc:\\([^:]*\\):.*/\\1/p' '%s'", dbConfigFile))
    }
    if err != nil || strings.TrimSpace(dbKind) == "" {
        // Last resort: check for known driver class
        driverCheck, _ := executeSSHCommand(client, fmt.Sprintf("sudo grep -o 'sqlserver\\|postgresql' '%s' | head -1", dbConfigFile))
        if strings.TrimSpace(driverCheck) != "" {
            dbKind = strings.TrimSpace(driverCheck)
        } else {
            return "", "", "", "", "", "", fmt.Errorf("could not determine database type from %s", dbConfigFile)
        }
    }

    return getDBParams(client, dataFolder, installDir, strings.TrimSpace(dbKind))
}
