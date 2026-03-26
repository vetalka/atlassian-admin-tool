package handlers

import (
	"bytes"
	"fmt"
	"log"
	"strings"

	"golang.org/x/crypto/ssh"
)

// RestartAppOnServers restarts the app service on all servers listed under the 'ip' column in the 'environments' table for a given environment.
func RestartAppOnServers(envName, appName, serverUser, serverPassword string) error {
	return manageAppOnServers(envName, appName, serverUser, serverPassword, "restart")
}

// StopAppOnServers stops the app service on all servers for a specific environment.
func StopAppOnServers(envName, appName, serverUser, serverPassword string) error {
	return manageAppOnServers(envName, appName, serverUser, serverPassword, "stop")
}

// StartAppOnServers starts the app service on all servers for a specific environment.
func StartAppOnServers(envName, appName, serverUser, serverPassword string) error {
	return manageAppOnServers(envName, appName, serverUser, serverPassword, "start")
}

// manageAppOnServers manages the app service on all servers for a specific environment and action (start, stop, restart).
func manageAppOnServers(envName, appName, serverUser, serverPassword, action string) error {
	// Retrieve the IPs, installDir, and homeDir from the 'environments' table for the specified environment
	var ipList, installDir, homeDir string
	query := "SELECT ip, install_dir, home_dir FROM environments WHERE name = ?"
	err := db.QueryRow(query, envName).Scan(&ipList, &installDir, &homeDir)
	if err != nil {
		return fmt.Errorf("failed to retrieve environment details from environments table: %v", err)
	}

	// Split the IPs into a slice
	ips := strings.Split(ipList, " ")

	// Loop over each IP and manage the app service
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			continue
		}

		log.Printf("Attempting to %s %s service on server: %s", action, appName, ip)

		// Establish SSH connection to the server
		client, err := connectToServer(ip, serverUser, serverPassword)
		if err != nil {
			log.Printf("Failed to connect to server %s: %v", ip, err)
			continue
		}
		defer client.Close()

		// Execute the appropriate command based on the action
		var manageErr error
		if action == "restart" {
			manageErr = restartAppService(client, appName, installDir, homeDir)
		} else if action == "stop" {
			manageErr = stopAppService(client, appName, installDir, homeDir)
		} else if action == "start" {
			manageErr = startAppService(client, appName, installDir)
		}

		if manageErr != nil {
			log.Printf("Failed to %s %s service on server %s: %v", action, appName, ip, manageErr)
			continue
		}

		log.Printf("Successfully %sed %s service on server: %s", action, appName, ip)
	}

	return nil
}

// stopAppService stops the app service on a given SSH client and deletes log files.
func stopAppService(client *ssh.Client, appName, installDir, homeDir string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	// Command to stop app service and delete log files
	log.Printf("appName: %s, installDir: %s, homeDir: %s", appName, installDir, homeDir)
	appName = strings.ToLower(appName)
	cmd := fmt.Sprintf(`
		A_SERVICE=$(sudo systemctl list-units --type=service | grep -i %s | awk '{print $2}' | head -n 1);
	

		if systemctl is-active --quiet $A_SERVICE; then
			echo 'Stopping ' $A_SERVICE;
			sudo service $A_SERVICE stop;
			sudo systemctl stop $A_SERVICE;
			sleep 10;
		else
			echo '%s service ' $A_SERVICE ' is not running.';
		fi;
		if ps aux | grep -i '%s' | grep -v grep; then
			echo 'Forcefully killing %s processes...';
			sudo pkill -9 -f '%s';
			sleep 5;
		fi;
		echo 'Deleting %s Log Files...';
		sudo rm -rf "%s/logs/catalina.out";
		sudo rm -rf "%s/log/atlassian-%s.log";
	`, appName, appName, appName, appName, appName, appName, installDir, homeDir, appName)

	var output, stderr bytes.Buffer
	session.Stdout = &output
	session.Stderr = &stderr
	//log.Printf(cmd)
	if err := session.Run(cmd); err != nil {
		log.Printf("Failed to stop %s service: %v, stderr: %s", appName, err, stderr.String())
		return fmt.Errorf("failed to stop %s service: %v", appName, err)
	}

	log.Printf("Stop command output: %s", output.String())
	return nil
}

// startAppService starts the app service on a given SSH client.
func startAppService(client *ssh.Client, appName, installDir string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %v", err)
	}
	defer session.Close()

	// Command to start app service
	cmd := fmt.Sprintf(`
		A_SERVICE=$(systemctl list-units --type=service | grep -i %s | awk '{print $2}' | head -n 1);
		echo "A_SERVICE determined as: $A_SERVICE";
		if [ -z $A_SERVICE ]; then
			echo '%s service not found.' >&2;
			exit 1;
		fi;
		echo '%s Service: ' $A_SERVICE;
		sudo service $A_SERVICE start;
		sudo systemctl start $A_SERVICE;
		# Check if the %s process is running
		while true; do
			if ps aux | grep -i java | grep -q %s; then
				echo '%s process is running.';
				break;
			else
				echo '%s process is not running. Attempting to start again...';
				sudo service $A_SERVICE start;
				sleep 10;
			fi;
		done;
	`, appName, appName, appName, appName, appName, appName, appName)
	//log.Printf(cmd)
	var output, stderr bytes.Buffer
	session.Stdout = &output
	session.Stderr = &stderr

	if err := session.Run(cmd); err != nil {
		log.Printf("Failed to start %s service: %v, stderr: %s", appName, err, stderr.String())
		return fmt.Errorf("failed to start %s service: %v", appName, err)
	}

	log.Printf("Start command output: %s", output.String())
	return nil
}

// restartAppService restarts the app service on a given SSH client.
func restartAppService(client *ssh.Client, appName, installDir, homeDir string) error {
	if err := stopAppService(client, appName, installDir, homeDir); err != nil {
		return err
	}
	if err := startAppService(client, appName, installDir); err != nil {
		return err
	}
	return nil
}
