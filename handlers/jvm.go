// File: handlers/jvm.go

package handlers

import (
	"bytes"
	"fmt"
	"log"

	"golang.org/x/crypto/ssh"
)

// GetConfig retrieves the content of the setenv.sh file located in installDir/bin.
func GetConfig(client *ssh.Client, appName, installDir, homeDir string) (string, error) {
	session, err := client.NewSession()
	log.Printf("getcofig func")
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	log.Printf("Fetching content of setenv.sh for appName: %s, installDir: %s, homeDir: %s", appName, installDir, homeDir)

	// Command to read the content of the setenv.sh file
	cmd := fmt.Sprintf("sudo cat '%s/bin/setenv.sh' ", installDir)

	var output, stderr bytes.Buffer
	session.Stdout = &output
	session.Stderr = &stderr

	if err := session.Run(cmd); err != nil {
		log.Printf("Failed to retrieve setenv.sh content: %v, stderr: %s", err, stderr.String())
		return "", fmt.Errorf("failed to retrieve setenv.sh content: %v", err)
	}

	log.Printf("Retrieved setenv.sh content successfully")
	return output.String(), nil
}
