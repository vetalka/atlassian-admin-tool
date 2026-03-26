package handlers

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"regexp"
	"strings"
)

// extractConfluenceDBParams extracts the database parameters for Confluence based on the database type (PostgreSQL/SQL Server)
func extractConfluenceDBParams(ip, serverUser, serverPassword, dataFolder string) (dbHost, dbPort, dbName, dbUser, dbPass, dbDriver string, err error) {
	client, err := connectToServer(ip, serverUser, serverPassword)
	if err != nil {
		return "", "", "", "", "", "", fmt.Errorf("failed to connect to server: %v", err)
	}
	defer client.Close()

	// Define the path to the confluence.cfg.xml file
	configFile := fmt.Sprintf("%s/confluence.cfg.xml", dataFolder)

	// Check if the file contains a JDBC URL for database connection
	dbCheckCmd := fmt.Sprintf(`sudo grep -q '<property name="hibernate.connection.url">jdbc' '%s' && echo "found" || echo "not found"`, configFile)
	dbTypeCheck, err := executeSSHCommand(client, dbCheckCmd)
	if err != nil || strings.TrimSpace(dbTypeCheck) == "not found" {
		return "", "", "", "", "", "", fmt.Errorf("database configuration not found in confluence.cfg.xml")
	}

	// Determine the database type (PostgreSQL, SQL Server)
	dbTypeCmd := fmt.Sprintf(`sudo grep -oP '(?<=<property name="hibernate.connection.url">jdbc:)(postgresql|sqlserver)' '%s'`, configFile)
	dbType, err := executeSSHCommand(client, dbTypeCmd)
	if err != nil || strings.TrimSpace(dbType) == "" {
		return "", "", "", "", "", "", fmt.Errorf("failed to determine database type: %v", err)
	}

	// Based on the DB type, extract the relevant database parameters
	switch strings.TrimSpace(dbType) {
	case "postgresql":
		return getConfluenceDBParamsPostgreSQL(client, configFile)
	case "sqlserver":
		return getConfluenceDBParamsSQLServer(client, configFile)
	default:
		return "", "", "", "", "", "", fmt.Errorf("unsupported database type: %s", dbType)
	}
}

// getConfluenceDBParamsPostgreSQL extracts PostgreSQL database parameters from confluence.cfg.xml
func getConfluenceDBParamsPostgreSQL(client *ssh.Client, configFile string) (dbHost, dbPort, dbName, dbUser, dbPass, dbDriver string, err error) {
	jdbcURLCmd := fmt.Sprintf(`sudo grep -oP '(?<=<property name="hibernate.connection.url">)jdbc:[^<]+' "%s"`, configFile)
	jdbcURL, err := executeSSHCommand(client, jdbcURLCmd)
	if err != nil {
		return "", "", "", "", "", "", fmt.Errorf("failed to extract JDBC URL: %v", err)
	}

	dbHost = extractFromURL(jdbcURL, `jdbc:postgresql://([^:]+)`)
	dbPort = extractFromURL(jdbcURL, `jdbc:postgresql://[^:]+:(\d+)`)
	dbName = extractFromURL(jdbcURL, `jdbc:postgresql://[^:]+:\d+/([^?]+)`)

	dbUserCmd := fmt.Sprintf(`sudo grep -oP '(?<=<property name="hibernate.connection.username">)[^<]+' "%s"`, configFile)
	dbUser, err = executeSSHCommand(client, dbUserCmd)
	if err != nil {
		return "", "", "", "", "", "", fmt.Errorf("failed to extract database username: %v", err)
	}

	dbPassCmd := fmt.Sprintf(`sudo grep -oP '(?<=<property name="hibernate.connection.password">)[^<]+' "%s"`, configFile)
	dbPass, err = executeSSHCommand(client, dbPassCmd)
	if err != nil {
		// Password might be empty or {ATL_SECURED}
		dbPass = ""
	}

	dbDriverCmd := fmt.Sprintf(`sudo grep -oP '(?<=<property name="hibernate.connection.driver_class">)[^<]+' "%s"`, configFile)
	dbDriver, err = executeSSHCommand(client, dbDriverCmd)
	if err != nil {
		return "", "", "", "", "", "", fmt.Errorf("failed to extract database driver: %v", err)
	}

	return strings.TrimSpace(dbHost), strings.TrimSpace(dbPort), strings.TrimSpace(dbName), strings.TrimSpace(dbUser), strings.TrimSpace(dbPass), strings.TrimSpace(dbDriver), nil
}

// getConfluenceDBParamsSQLServer extracts SQL Server database parameters from confluence.cfg.xml
func getConfluenceDBParamsSQLServer(client *ssh.Client, configFile string) (dbHost, dbPort, dbName, dbUser, dbPass, dbDriver string, err error) {
	jdbcURLCmd := fmt.Sprintf(`sudo grep -oP '(?<=<property name="hibernate.connection.url">)jdbc:sqlserver://[^<]+' "%s"`, configFile)
	jdbcURL, err := executeSSHCommand(client, jdbcURLCmd)
	if err != nil {
		return "", "", "", "", "", "", fmt.Errorf("failed to extract JDBC URL: %v", err)
	}

	dbHost = extractFromURL(jdbcURL, `(?<=jdbc:sqlserver://)([^:]+)`)
	dbPort = extractFromURL(jdbcURL, `(?<=:)\d+(?=;)`)
	dbName = extractFromURL(jdbcURL, `(?<=databaseName=)[^;]+`)

	dbUserCmd := fmt.Sprintf(`sudo grep -oP '(?<=<property name="hibernate.connection.username">)[^<]+' "%s"`, configFile)
	dbUser, err = executeSSHCommand(client, dbUserCmd)
	if err != nil {
		return "", "", "", "", "", "", fmt.Errorf("failed to extract database username: %v", err)
	}

	dbPassCmd := fmt.Sprintf(`sudo grep -oP '(?<=<property name="hibernate.connection.password">)[^<]+' "%s"`, configFile)
	dbPass, err = executeSSHCommand(client, dbPassCmd)
	if err != nil {
		// Password might be empty or {ATL_SECURED}
		dbPass = ""
	}

	dbDriverCmd := fmt.Sprintf(`sudo grep -oP '(?<=<property name="hibernate.connection.driver_class">)[^<]+' "%s"`, configFile)
	dbDriver, err = executeSSHCommand(client, dbDriverCmd)
	if err != nil {
		return "", "", "", "", "", "", fmt.Errorf("failed to extract database driver: %v", err)
	}

	return strings.TrimSpace(dbHost), strings.TrimSpace(dbPort), strings.TrimSpace(dbName), strings.TrimSpace(dbUser), strings.TrimSpace(dbPass), strings.TrimSpace(dbDriver), nil
}

// Helper function to extract values from URLs using regex
func extractFromURL(url, regexPattern string) string {
	re := regexp.MustCompile(regexPattern)
	match := re.FindStringSubmatch(url)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}
