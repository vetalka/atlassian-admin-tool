package handlers

import (
    "bytes"
    "fmt"
    "regexp" // Add this package for regex operations
    "strings"
)

// GetBaseURL retrieves the base URL from the database (PostgreSQL or SQL Server)
func GetBaseURLConfluence(dbType, dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName string) (string, error) {
    var baseURL string
    var err error

    switch dbType {
    case "postgresql":
        baseURL, err = getBaseURLPostgreSQLConfluence(dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName)
    case "sqlserver":
        baseURL, err = getBaseURLSQLServerConfluence(dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName)
    default:
        return "", fmt.Errorf("unsupported database type: %s", dbType)
    }

    if err != nil {
        return "", fmt.Errorf("failed to retrieve base URL: %v", err)
    }

    return baseURL, nil
}

// getBaseURLPostgreSQL retrieves the base URL for PostgreSQL using SSH connection
func getBaseURLPostgreSQLConfluence(dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName string) (string, error) {
    sqlQuery := "SELECT BANDANAVALUE FROM BANDANA WHERE BANDANACONTEXT = '_GLOBAL' AND BANDANAKEY = 'atlassian.confluence.settings';"

    // Establish SSH connection to the DB host
    conn, err := connectToServer(dbHost, serverUser, serverPassword)
    if err != nil {
        return "", fmt.Errorf("failed to connect to SSH server: %v", err)
    }
    defer conn.Close()

    // Create a new SSH session
    session, err := conn.NewSession()
    if err != nil {
        return "", fmt.Errorf("failed to create SSH session: %v", err)
    }
    defer session.Close()

    // Check if `psql` is available on the remote server
    checkCmd := "which psql"
    var checkOut, checkErr bytes.Buffer
    session.Stdout = &checkOut
    session.Stderr = &checkErr
    if err := session.Run(checkCmd); err != nil {
        return "", fmt.Errorf("psql command not found on remote server: %v, stderr: %s", err, checkErr.String())
    }

    // Prepare the command to retrieve the base URL
    cmd := fmt.Sprintf(`export PGPASSWORD='%s'; psql -h %s -U %s -d %s -p %s -t -A -c "%s"`, dbPass, dbHostForDB, dbUser, dbName, dbPort, sqlQuery)

    // Reset the session for the next command
    session, err = conn.NewSession()
    if err != nil {
        return "", fmt.Errorf("failed to create SSH session for SQL command: %v", err)
    }
    defer session.Close()

    var out, stderr bytes.Buffer
    session.Stdout = &out
    session.Stderr = &stderr

    if err := session.Run(cmd); err != nil {
        return "", fmt.Errorf("error executing PostgreSQL command: %v, stderr: %s", err, stderr.String())
    }

    bandanaValue := strings.TrimSpace(out.String())

    // Extract base URL from the BANDANAVALUE XML
    baseURL := extractBaseURLFromBandana(bandanaValue)
    if baseURL == "" {
        return "", fmt.Errorf("base URL not found in BANDANAVALUE")
    }

    return baseURL, nil
}

// getBaseURLSQLServer retrieves the base URL for SQL Server using SSH connection
func getBaseURLSQLServerConfluence(dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName string) (string, error) {
    sqlQuery := "SET NOCOUNT ON; SELECT BANDANAVALUE FROM BANDANA WHERE BANDANACONTEXT = '_GLOBAL' AND BANDANAKEY = 'atlassian.confluence.settings';"

    // Establish SSH connection to the DB host
    conn, err := connectToServer(dbHost, serverUser, serverPassword)
    if err != nil {
        return "", fmt.Errorf("failed to connect to SSH server: %v", err)
    }
    defer conn.Close()

    // Create a new SSH session
    session, err := conn.NewSession()
    if err != nil {
        return "", fmt.Errorf("failed to create SSH session: %v", err)
    }
    defer session.Close()

    // Prepare the command to retrieve the base URL
    cmd := fmt.Sprintf(`sqlcmd -S %s,%s -U %s -P %s -d %s -h -1 -W -Q "%s"`, dbHostForDB, dbPort, dbUser, dbPass, dbName, sqlQuery)

    var out, stderr bytes.Buffer
    session.Stdout = &out
    session.Stderr = &stderr

    if err := session.Run(cmd); err != nil {
        return "", fmt.Errorf("error executing SQL Server command: %v, stderr: %s", err, stderr.String())
    }

    bandanaValue := strings.TrimSpace(out.String())

    // Extract base URL from the BANDANAVALUE XML
    baseURL := extractBaseURLFromBandana(bandanaValue)
    if baseURL == "" {
        return "", fmt.Errorf("base URL not found in BANDANAVALUE")
    }

    return baseURL, nil
}

// extractBaseURLFromBandana extracts the base URL from the BANDANAVALUE XML content
func extractBaseURLFromBandana(bandanaValue string) string {
    re := regexp.MustCompile(`<baseUrl>(.*?)</baseUrl>`)
    match := re.FindStringSubmatch(bandanaValue)
    if len(match) > 1 {
        return match[1]
    }
    return ""
}
