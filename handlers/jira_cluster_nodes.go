package handlers

import (
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// RetrieveClusterInfo retrieves node information from the database and processes the IP addresses via SSH.
func RetrieveClusterInfo(dbType, dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName, envName string, db *sql.DB) error {
	var ips string
	var err error

	switch dbType {
	case "postgresql":
		ips, err = retrieveClusterInfoPostgreSQLSSH(dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName)
	case "sqlserver":
		ips, err = retrieveClusterInfoSQLServerSSH(dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName)
	default:
		return fmt.Errorf("unsupported database type: %s", dbType)
	}

	if err != nil {
		return fmt.Errorf("failed to retrieve cluster info: %v", err)
	}

	log.Printf("Retrieved IPs: %s", ips)

	// Save the IPs to the environments table if they are not empty
	if ips != "" {
		err = saveIPsToEnvironment(envName, ips, db)
		if err != nil {
			return fmt.Errorf("failed to save IPs to environment: %v", err)
		}
		log.Printf("Successfully saved IPs: %s", ips)
	} else {
		log.Println("No valid IPs retrieved. Skipping update of IP column in the environments table.")
	}

	return nil
}

// retrieveClusterInfoPostgreSQLSSH retrieves node IPs from a PostgreSQL database using an SSH connection.
func retrieveClusterInfoPostgreSQLSSH(dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName string) (string, error) {
	sqlQuery := "SELECT ip FROM public.clusternode;"

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

	// Prepare the command to retrieve cluster IPs
	cmd := fmt.Sprintf(`export PGPASSWORD='%s'; psql -h %s -U %s -d %s -p %s -t -A -c "%s"`, dbPass, dbHostForDB, dbUser, dbName, dbPort, sqlQuery)

	var out, stderr bytes.Buffer
	session.Stdout = &out
	session.Stderr = &stderr

	if err := session.Run(cmd); err != nil {
		return "", fmt.Errorf("error executing PostgreSQL command: %v, stderr: %s", err, stderr.String())
	}

	ips := strings.TrimSpace(out.String())
	// Replace line breaks with a space
	ips = strings.ReplaceAll(ips, "\r\n", " ")
	ips = strings.ReplaceAll(ips, "\n", " ")
	return ips, nil
}

// retrieveClusterInfoSQLServerSSH retrieves node IPs from a SQL Server database using an SSH connection.
func retrieveClusterInfoSQLServerSSH(dbHost, serverUser, serverPassword, dbHostForDB, dbPort, dbUser, dbPass, dbName string) (string, error) {
	sqlQuery := "SET NOCOUNT ON; SELECT ip FROM dbo.clusternode;"

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

	// Prepare the command to retrieve cluster IPs
	cmd := fmt.Sprintf(`sqlcmd -S %s,%s -U %s -P %s -d %s -h -1 -W -Q "%s"`, dbHostForDB, dbPort, dbUser, dbPass, dbName, sqlQuery)

	var out, stderr bytes.Buffer
	session.Stdout = &out
	session.Stderr = &stderr

	if err := session.Run(cmd); err != nil {
		return "", fmt.Errorf("error executing SQL Server command: %v, stderr: %s", err, stderr.String())
	}

	ips := strings.TrimSpace(out.String())
	// Replace line breaks with a space
	ips = strings.ReplaceAll(ips, "\r\n", " ")
	ips = strings.ReplaceAll(ips, "\n", " ")
	return ips, nil
}

// saveIPsToEnvironment saves the provided IPs to the 'ip' field of the environments table.
func saveIPsToEnvironment(envName, ips string, db *sql.DB) error {
	if ips == "" {
		log.Println("No IPs to save. Skipping update of IP column.")
		return nil
	}
	query := "UPDATE environments SET ip = ? WHERE name = ?"
	_, err := db.Exec(query, ips, envName)
	return err
}
