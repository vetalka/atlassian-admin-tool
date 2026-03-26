package handlers

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"strings"
)

// Environment structure to hold the app type and install directory
type Environment struct {
	AppType    string
	InstallDir string
}

// GetEnvironment retrieves the app type and install directory for the given environment
func GetEnvironment(db *sql.DB, environmentName string) (Environment, error) {
	var env Environment
	query := `SELECT app, install_dir FROM environments WHERE name = ?`
	err := db.QueryRow(query, environmentName).Scan(&env.AppType, &env.InstallDir)
	if err != nil {
		return env, fmt.Errorf("failed to retrieve environment details: %v", err)
	}
	return env, nil
}

// DispatchJavaHomeExtraction routes the call to the appropriate extraction function based on the app type
func DispatchJavaHomeExtraction(installDir, appType string) (string, error) {
	switch appType {
	case "Jira":
		// Call the function specific to Jira
		return ExtractJavaHomeForJira(installDir)
	case "Confluence":
		// Call the function specific to Confluence
		return ExtractJavaHomeForConfluence(installDir)
	default:
		return "", fmt.Errorf("unsupported application type: %s", appType)
	}
}

// ExtractJavaHomeForJira extracts JAVA_HOME from Jira's setenv.sh
func ExtractJavaHomeForJira(installDir string) (string, error) {
	setEnvPath := fmt.Sprintf("%s/bin/setenv.sh", installDir)

	file, err := os.Open(setEnvPath)
	if err != nil {
		return "", fmt.Errorf("failed to open setenv.sh for Jira: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var javaHome string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "JAVA_HOME=") {
			// Extract JAVA_HOME
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				javaHome = strings.Trim(parts[1], `"; `)
				break
			}
		}
	}

	if javaHome != "" {
		return javaHome, nil
	}

	return "", fmt.Errorf("JAVA_HOME not found in Jira setenv.sh")
}

// ExtractJavaHomeForConfluence extracts JRE_HOME or JAVA_HOME from Confluence's setenv.sh or setjre.sh
func ExtractJavaHomeForConfluence(installDir string) (string, error) {
	setEnvPath := fmt.Sprintf("%s/bin/setenv.sh", installDir)
	setJrePath := fmt.Sprintf("%s/bin/setjre.sh", installDir)

	// Check for JRE_HOME in setenv.sh first
	jreHome, err := extractJavaHome(setEnvPath, "JRE_HOME")
	if err == nil && jreHome != "" {
		return jreHome, nil
	}

	// Check for JAVA_HOME in setenv.sh if JRE_HOME is not found
	javaHome, err := extractJavaHome(setEnvPath, "JAVA_HOME")
	if err == nil && javaHome != "" {
		return javaHome, nil
	}

	// If neither JRE_HOME nor JAVA_HOME is found in setenv.sh, check setjre.sh
	jreHome, err = extractJavaHome(setJrePath, "JRE_HOME")
	if err == nil && jreHome != "" {
		return jreHome, nil
	}

	return "", fmt.Errorf("neither JRE_HOME nor JAVA_HOME found in Confluence setenv.sh or setjre.sh")
}

// extractJavaHome is a helper function that extracts the value of JAVA_HOME or JRE_HOME from a given file
func extractJavaHome(filePath string, envVar string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s: %v", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, envVar+"=") {
			// Extract the value after '=' and remove surrounding quotes and spaces
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.Trim(parts[1], `"; `), nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to scan file %s: %v", filePath, err)
	}

	return "", fmt.Errorf("%s not found in file %s", envVar, filePath)
}
