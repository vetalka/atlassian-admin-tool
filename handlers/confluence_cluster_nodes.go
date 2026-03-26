package handlers

import (
    "bytes"
    "fmt"
    "net"
    "strings"
)

// getClusterNodes retrieves clustered node IPs from confluence.cfg.xml
func getClusterNodes(ip, serverUser, serverPassword, configFilePath string) ([]string, error) {
    // SSH connection to the server
    conn, err := connectToServer(ip, serverUser, serverPassword)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to server: %v", err)
    }
    defer conn.Close()

    // Create a new SSH session
    session, err := conn.NewSession()
    if err != nil {
        return nil, fmt.Errorf("failed to create SSH session: %v", err)
    }
    defer session.Close()

    // Command to extract cluster node IPs from the confluence.cfg.xml file
    cmd := fmt.Sprintf(`sudo grep -Po '(?<=<property name="confluence.cluster.peers">)[^<]+' %s`, configFilePath)

    var out, stderr bytes.Buffer
    session.Stdout = &out
    session.Stderr = &stderr

    if err := session.Run(cmd); err != nil {
        return nil, fmt.Errorf("failed to execute command: %v, stderr: %s", err, stderr.String())
    }

    clusterNodeIPs := strings.TrimSpace(out.String())

    // If no IPs are retrieved, default to the server IP of the chosen environment
    if clusterNodeIPs == "" {
        return []string{ip}, nil
    }

    // Split the IPs into an array
    ips := strings.Split(clusterNodeIPs, ",")

    // Validate and collect valid IPs
    var validIPs []string
    for _, ip := range ips {
        trimmedIP := strings.TrimSpace(ip)
        if isValidIP(trimmedIP) {
            validIPs = append(validIPs, trimmedIP)
        } else {
            fmt.Printf("Invalid IP detected: %s\n", trimmedIP)
        }
    }

    return validIPs, nil
}

// isValidIP validates if a given string is a valid IPv4 address
func isValidIP(ip string) bool {
    return net.ParseIP(ip) != nil
}
