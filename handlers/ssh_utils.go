package handlers

import (
    "golang.org/x/crypto/ssh"
    "net"
    "time"
    "errors"
    "log"
    "fmt"
    "strings"
)

// CheckSSHConnection validates the SSH connection to the server using provided credentials
func CheckSSHConnection(ip, serverUser, serverPassword string) error {
    // First, check if the provided IP is valid
    if net.ParseIP(ip) == nil {
        return errors.New("invalid IP address provided")
    }

    log.Printf("CheckSSHConnection: attempting %s@%s with password length=%d", serverUser, ip, len(serverPassword))

    config := &ssh.ClientConfig{
        User: serverUser,
        Auth: []ssh.AuthMethod{
            ssh.Password(serverPassword),
            ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
                answers := make([]string, len(questions))
                for i := range answers {
                    answers[i] = serverPassword
                }
                return answers, nil
            }),
        },
        HostKeyCallback: ssh.InsecureIgnoreHostKey(),
        Timeout:         10 * time.Second, // Increase the timeout
    }

    conn, err := ssh.Dial("tcp", ip+":22", config)
    if err != nil {
        if err.Error() == "ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password], no supported methods remain" {
            return errors.New("invalid provided credentials")
        }
        return fmt.Errorf("failed to connect via SSH: %v", err) // Updated to provide more detailed errors
    }
    defer conn.Close()

    return nil
}

// connectToServer establishes an SSH connection to the remote server
func connectToServer(ip, serverUser, serverPassword string) (*ssh.Client, error) {
    // Trim whitespace from credentials (common DB storage issue)
    serverUser = strings.TrimSpace(serverUser)
    serverPassword = strings.TrimSpace(serverPassword)
    ip = strings.TrimSpace(ip)

    // Check for placeholder passwords
    if strings.Contains(serverPassword, "{ATL_SECURED}") || strings.Contains(serverPassword, "{ENCRYPTED}") || serverPassword == "" {
        log.Printf("ERROR: Password for %s@%s is empty or placeholder — cannot connect", serverUser, ip)
        return nil, fmt.Errorf("SSH password for %s@%s is not set. Update server password in Edit Environment", serverUser, ip)
    }

    // Check for encoding issues (null bytes, non-ASCII)
    cleanPassword := serverPassword
    if strings.ContainsRune(serverPassword, 0) {
        cleanPassword = strings.ReplaceAll(serverPassword, "\x00", "")
        log.Printf("WARNING: Password contained null bytes, cleaned from %d to %d chars", len(serverPassword), len(cleanPassword))
    }

    // Log masked password for debugging (first 2 + last 1 char)
    masked := "***"
    if len(cleanPassword) > 3 {
        masked = cleanPassword[:2] + strings.Repeat("*", len(cleanPassword)-3) + cleanPassword[len(cleanPassword)-1:]
    } else if len(cleanPassword) > 0 {
        masked = cleanPassword[:1] + "**"
    }
    log.Printf("SSH to %s@%s — password: '%s' (len=%d)", serverUser, ip, masked, len(cleanPassword))

    config := &ssh.ClientConfig{
        User: serverUser,
        Auth: []ssh.AuthMethod{
            ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
                answers := make([]string, len(questions))
                for i := range answers {
                    answers[i] = cleanPassword
                }
                return answers, nil
            }),
            ssh.Password(cleanPassword),
        },
        HostKeyCallback: ssh.InsecureIgnoreHostKey(),
        Timeout:         15 * time.Second,
    }

    conn, err := ssh.Dial("tcp", ip+":22", config)
    if err != nil {
        log.Printf("SSH failed to %s@%s: %v", serverUser, ip, err)
        if strings.Contains(err.Error(), "no supported methods remain") {
            log.Printf("DIAGNOSTIC: user='%s', password='%s', ip='%s'", serverUser, masked, ip)
            log.Println("POSSIBLE CAUSES:")
            log.Println("  1. Wrong password stored in database — update in Edit Environment")
            log.Println("  2. User account is locked — check with: sudo passwd -S " + serverUser)
            log.Println("  3. SSH config issue — check: sudo grep -E 'PasswordAuth|KbdInteractive|PermitRoot' /etc/ssh/sshd_config")
            log.Println("  4. PAM issue — check: sudo grep pam_unix /var/log/auth.log | tail -5")
        }
        return nil, fmt.Errorf("SSH connection failed for %s@%s: %v", serverUser, ip, err)
    }

    log.Printf("SSH connected to %s@%s", serverUser, ip)
    return conn, nil
}


// executeCommands runs a list of commands on the remote server.
func executeCommands(client *ssh.Client, commands []string) error {
	for _, cmd := range commands {
		session, err := client.NewSession()
		if err != nil {
			return fmt.Errorf("failed to create SSH session: %v", err)
		}
		defer session.Close()

		if err := session.Run(cmd); err != nil {
			log.Printf("Command failed: %s", cmd)
			return fmt.Errorf("failed to run command: %v", err)
		}
	}
	return nil
}
