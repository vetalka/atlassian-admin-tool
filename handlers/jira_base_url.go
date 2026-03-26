package handlers

import (
    "bytes"
    "fmt"
    "strings"
)

// GetBaseURL retrieves the base URL from the database (PostgreSQL or SQL Server)
func GetBaseURL(dbType, dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName string) (string, error) {
    var baseURL string
    var err error

    switch dbType {
    case "postgresql":
        baseURL, err = getBaseURLPostgreSQL(dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName)
    case "sqlserver":
        baseURL, err = getBaseURLSQLServer(dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName)
    default:
        return "", fmt.Errorf("unsupported database type: %s", dbType)
    }

    if err != nil {
        return "", fmt.Errorf("failed to retrieve base URL: %v", err)
    }

    return baseURL, nil
}

// getBaseURLPostgreSQL retrieves the base URL for PostgreSQL using SSH connection
func getBaseURLPostgreSQL(dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName string) (string, error) {
    sqlQuery := "SELECT propertyvalue FROM propertyentry PE JOIN propertystring PS ON PE.id = PS.id WHERE PE.property_key = 'jira.baseurl';"

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

    fmt.Println("Executing command:", cmd)

    if err := session.Run(cmd); err != nil {
        return "", fmt.Errorf("error executing PostgreSQL command: %v, stderr: %s", err, stderr.String())
    }

    baseURL := strings.TrimSpace(out.String())
    return baseURL, nil
}

// getBaseURLSQLServer retrieves the base URL for SQL Server using SSH connection
func getBaseURLSQLServer(dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName string) (string, error) {
    sqlQuery := "SET NOCOUNT ON; SELECT propertyvalue FROM propertyentry PE JOIN propertystring PS ON PE.id=PS.id WHERE PE.property_key = 'jira.baseurl';"

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

    // Prepare the command to retrieve the base URL, suppressing row count
    cmd := fmt.Sprintf(`sqlcmd -S %s,%s -U %s -P %s -d %s -h -1 -W -Q "%s"`, dbHostForDB, dbPort, dbUser, dbPass, dbName, sqlQuery)

    var out, stderr bytes.Buffer
    session.Stdout = &out
    session.Stderr = &stderr

    if err := session.Run(cmd); err != nil {
        return "", fmt.Errorf("error executing SQL Server command: %v, stderr: %s", err, stderr.String())
    }

    // Clean and trim the output to get only the base URL
    baseURL := strings.TrimSpace(out.String())
    return baseURL, nil
}
