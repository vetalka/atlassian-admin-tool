package handlers

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"log"
	"net"
	"net/http"
	"strings"
)

// HandleRestartAppAJAX handles the AJAX request to restart the application service on all servers for a specific environment.
func HandleRestartAppAJAX() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract the environment name and appName from the URL path
		pathParts := strings.Split(r.URL.Path[len("/environment/restart-app/"):], "/")
		environmentName := pathParts[0]
		appName := pathParts[1]

		// Fetch serverUser and serverPassword from the environments table
		var serverUser, serverPassword string
		err := db.QueryRow("SELECT server_user, server_password FROM environments WHERE name = ?", environmentName).Scan(&serverUser, &serverPassword)
		if err != nil {
			log.Printf("Failed to retrieve server credentials for environment %s: %v", environmentName, err)
			http.Error(w, "Failed to retrieve server credentials. Check logs for details.", http.StatusInternalServerError)
			return
		}

		// Call RestartAppOnServers to restart the app on all servers for the environment
		err = RestartAppOnServers(environmentName, appName, serverUser, serverPassword)
		if err != nil {
			log.Printf("Failed to restart %s on servers for environment %s: %v", appName, environmentName, err)
			http.Error(w, fmt.Sprintf("Failed to restart %s on servers. Check logs for details.", appName), http.StatusInternalServerError)
			return
		}

		// Respond with success status
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("%s service restarted successfully.", appName)))
	}
}

// HandleStopAppAJAX handles the AJAX request to stop the application service on all servers for a specific environment.
func HandleStopAppAJAX() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract the environment name and appName from the URL path
		pathParts := strings.Split(r.URL.Path[len("/environment/stop-app/"):], "/")
		environmentName := pathParts[0]
		appName := pathParts[1]

		// Fetch serverUser and serverPassword from the environments table
		var serverUser, serverPassword string
		err := db.QueryRow("SELECT server_user, server_password FROM environments WHERE name = ?", environmentName).Scan(&serverUser, &serverPassword)
		if err != nil {
			log.Printf("Failed to retrieve server credentials for environment %s: %v", environmentName, err)
			http.Error(w, "Failed to retrieve server credentials. Check logs for details.", http.StatusInternalServerError)
			return
		}

		// Call StopAppOnServers to stop the app on all servers for the environment
		err = StopAppOnServers(environmentName, appName, serverUser, serverPassword)
		if err != nil {
			log.Printf("Failed to stop %s on servers for environment %s: %v", appName, environmentName, err)
			http.Error(w, fmt.Sprintf("Failed to stop %s on servers. Check logs for details.", appName), http.StatusInternalServerError)
			return
		}

		// Respond with success status
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("%s service stopped successfully.", appName)))
	}
}

// HandleStartAppAJAX handles the AJAX request to start the application service on all servers for a specific environment.
func HandleStartAppAJAX() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract the environment name and appName from the URL path
		pathParts := strings.Split(r.URL.Path[len("/environment/start-app/"):], "/")
		environmentName := pathParts[0]
		appName := pathParts[1]

		// Fetch serverUser and serverPassword from the environments table
		var serverUser, serverPassword string
		err := db.QueryRow("SELECT server_user, server_password FROM environments WHERE name = ?", environmentName).Scan(&serverUser, &serverPassword)
		if err != nil {
			log.Printf("Failed to retrieve server credentials for environment %s: %v", environmentName, err)
			http.Error(w, "Failed to retrieve server credentials. Check logs for details.", http.StatusInternalServerError)
			return
		}

		// Call StartAppOnServers to start the app on all servers for the environment
		err = StartAppOnServers(environmentName, appName, serverUser, serverPassword)
		if err != nil {
			log.Printf("Failed to start %s on servers for environment %s: %v", appName, environmentName, err)
			http.Error(w, fmt.Sprintf("Failed to start %s on servers. Check logs for details.", appName), http.StatusInternalServerError)
			return
		}

		// Respond with success status
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("%s service started successfully.", appName)))
	}
}

// HandleGetConfigAJAX handles the AJAX request to get the configuration for a specific environment and app.
func HandleGetConfigAJAX() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path[len("/environment/get-config/"):], "/")
		if len(pathParts) < 2 {
			RenderErrorPage(w, r, "Invalid Path", "Invalid URL path for configuration.", "/", "Back to Home", http.StatusBadRequest)
			return
		}

		environmentName := pathParts[0]
		appName := pathParts[1]

		var ips, serverUser, serverPassword string
		err := db.QueryRow("SELECT ip, server_user, server_password FROM environments WHERE name = ?", environmentName).Scan(&ips, &serverUser, &serverPassword)
		if err != nil {
			log.Printf("Failed to retrieve config for environment %s: %v", environmentName, err)
			RenderErrorPage(w, r, "Configuration Error", "Failed to retrieve server configuration. Check logs for details.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		ipList := strings.Split(ips, " ")

		nodesHTML := ""
		for i, ip := range ipList {
			hostname, err := resolveHostname(ip)
			if err != nil {
				hostname = ip
			}
			link := fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip)
			nodeNum := fmt.Sprintf("Node %d", i+1)
			nodesHTML += fmt.Sprintf(`
				<a href="%s" class="ads-action-card" style="display:flex; align-items:center; gap:16px; padding:20px; text-align:left;">
					<div class="ads-action-card-icon nodes" style="flex-shrink:0;">
						<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"></rect><line x1="8" y1="21" x2="16" y2="21"></line><line x1="12" y1="17" x2="12" y2="21"></line></svg>
					</div>
					<div style="flex:1; min-width:0;">
						<div style="font-weight:600; font-size:15px;">%s</div>
						<div style="font-size:13px; color:var(--color-text-subtle); margin-top:2px; font-family:monospace;">%s</div>
						<div style="font-size:12px; color:var(--color-text-subtlest); margin-top:2px;">%s</div>
					</div>
					<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="flex-shrink:0; color:var(--color-text-subtle);"><polyline points="9 18 15 12 9 6"></polyline></svg>
				</a>`, link, html.EscapeString(hostname), html.EscapeString(ip), nodeNum)
		}

		content := fmt.Sprintf(`
		<div class="ads-page-centered"><div class="ads-page-content">
			<div class="ads-breadcrumbs">
				<a href="/">Environments</a> &rarr;
				<a href="/environment/%s">%s</a> &rarr;
				Select Node
			</div>
			<div class="ads-card-flat" style="margin-top:16px;">
				<div class="ads-card-header">
					<div style="width:48px; height:48px; background:linear-gradient(135deg, #0747A6, #0065FF); border-radius:12px; display:flex; align-items:center; justify-content:center; flex-shrink:0;">
						<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2"><rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect><rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect><line x1="6" y1="6" x2="6.01" y2="6"></line><line x1="6" y1="18" x2="6.01" y2="18"></line></svg>
					</div>
					<div>
						<span class="ads-card-title" style="font-size:18px;">Select a %s Node</span>
						<div style="font-size:13px; color:var(--color-text-subtle); margin-top:2px;">Choose which server to view configuration for</div>
					</div>
				</div>
				<div style="padding:0 16px 24px;">
					<div style="display:grid; grid-template-columns:repeat(auto-fill, minmax(280px, 1fr)); gap:12px;">
						%s
					</div>
					<div style="margin-top:20px; padding-top:16px; border-top:1px solid var(--color-border);">
						<a href="/environment/%s" class="ads-button ads-button-default">&larr; Back to Environment Actions</a>
					</div>
				</div>
			</div>
		</div></div>
		`, html.EscapeString(environmentName), html.EscapeString(environmentName), html.EscapeString(appName), nodesHTML, html.EscapeString(environmentName))

		RenderPage(w, PageData{
			Title:   "Select Node - " + environmentName,
			IsAdmin: func() bool { u, _ := GetCurrentUsername(r); a, _ := IsAdminUser(u); return a }(),
			Content: template.HTML(content),
		})
	}
}

func resolveHostname(ip string) (string, error) {
	addrs, err := net.LookupAddr(ip)
	if err != nil || len(addrs) == 0 {
		return ip, err // fallback to IP if hostname resolution fails
	}
	return strings.TrimSuffix(addrs[0], "."), nil
}

func HandleGetConfigForIP() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path[len("/environment/get-config-ip/"):], "/")
		if len(pathParts) < 3 {
			RenderErrorPage(w, r, "Invalid Path", "Invalid URL path.", "/", "Back to Home", http.StatusBadRequest)
			return
		}

		environmentName := pathParts[0]
		appName := pathParts[1]
		ip := pathParts[2]

		var serverUser, serverPassword, installDir string
		err := db.QueryRow("SELECT server_user, server_password, install_dir FROM environments WHERE name = ?", environmentName).Scan(&serverUser, &serverPassword, &installDir)
		if err != nil {
			log.Printf("Failed to retrieve config for environment %s: %v", environmentName, err)
			RenderErrorPage(w, r, "Configuration Error", "Failed to retrieve server configuration. Check logs for details.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		client, err := connectToServer(ip, serverUser, serverPassword)
		if err != nil {
			log.Printf("Failed to connect via SSH: %v", err)
			RenderErrorPage(w, r, "SSH Connection Failed", fmt.Sprintf("Failed to connect to server %s via SSH. Please verify the server is reachable and credentials are correct.", ip), fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}
		defer client.Close()

		configContent, err := GetConfig(client, appName, installDir, "/path/to/homeDir")
		if err != nil {
			log.Printf("Failed to retrieve setenv.sh content: %v", err)
			RenderErrorPage(w, r, "Configuration Error", "Failed to retrieve setenv.sh content from the server.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		jvmArgsIndex := strings.Index(configContent, "JVM_REQUIRED_ARGS='")
		if jvmArgsIndex == -1 {
			RenderErrorPage(w, r, "Configuration Error", "JVM_REQUIRED_ARGS not found in setenv.sh.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		jvmArgsContent := configContent[jvmArgsIndex+len("JVM_REQUIRED_ARGS='"):]
		jvmArgsEndIndex := strings.Index(jvmArgsContent, "'")
		if jvmArgsEndIndex == -1 {
			RenderErrorPage(w, r, "Configuration Error", "Malformed JVM_REQUIRED_ARGS value in setenv.sh.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		jvmArgs := jvmArgsContent[:jvmArgsEndIndex]
		args := strings.Split(jvmArgs, " -")
		for i := range args {
			if i > 0 {
				args[i] = "-" + args[i]
			}
		}

		minMemory, maxMemory, err := extractJVMValues(configContent)
		if err != nil {
			log.Printf("Failed to extract JVM memory values: %v", err)
			RenderErrorPage(w, r, "Configuration Error", "Failed to extract JVM memory values from setenv.sh.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		// Build table rows
		argsRowsHTML := ""
		for i, arg := range args {
			argsRowsHTML += fmt.Sprintf("<tr><td style=\"width:60px; text-align:center; color:var(--color-text-subtle);\">%d</td><td><code style=\"font-size:13px;\">%s</code></td></tr>", i+1, html.EscapeString(arg))
		}

		sanitizedEnv := html.EscapeString(environmentName)
		sanitizedApp := html.EscapeString(appName)
		sanitizedIP := html.EscapeString(ip)

		extraHead := template.HTML("<script>\n" +
			"function jvmAction(action, param, paramName) {\n" +
			"  var val = document.getElementById(param).value;\n" +
			"  var xhr = new XMLHttpRequest();\n" +
			"  xhr.open('POST', '/environment/' + action + '/" + environmentName + "/" + appName + "/" + ip + "', true);\n" +
			"  xhr.setRequestHeader('Content-Type', 'application/x-www-form-urlencoded');\n" +
			"  xhr.onload = function() { if (xhr.status === 200) { alert('Success!'); location.reload(); } else { alert('Failed. Check logs.'); } };\n" +
			"  xhr.send(paramName + '=' + encodeURIComponent(val));\n" +
			"}\n</script>")

		content := fmt.Sprintf(`
		<div class="ads-page-centered"><div class="ads-page-content">
			<div class="ads-breadcrumbs"><a href="/">Environments</a> &rarr;
				<a href="/environment/%s">%s</a> &rarr;
				<a href="/environment/get-config/%s/%s">Select Node</a> &rarr; %s</div>
				<div class="ads-card-flat" style="margin-top:16px;">
					<div class="ads-card-header">
						<span style="font-size:24px;">&#9881;</span>
						<div>
							<span class="ads-card-title">JVM Configuration</span>
							<div style="font-size:13px; color:var(--color-text-subtle); margin-top:2px;">
								<span class="ads-lozenge ads-lozenge-info">%s</span>
								Node: <strong>%s</strong>
							</div>
						</div>
					</div>
					<div style="padding:0 24px 16px;">
						<table class="ads-table" style="width:100%%;">
							<thead><tr><th style="width:60px;">No.</th><th>Argument</th></tr></thead>
							<tbody>%s</tbody>
						</table>
						<div style="margin-top:12px; display:flex; gap:8px; align-items:center;">
							<input type="text" id="jvmArg" class="ads-input" placeholder="Enter JVM argument (e.g. -Dsome.property=value)" style="flex:1;">
							<button class="ads-button ads-button-primary" onclick="jvmAction('add-jvm-arg','jvmArg','jvmArg')">+ Add</button>
							<button class="ads-button ads-button-danger" onclick="jvmAction('remove-jvm-arg','jvmArg','jvmArg')">- Remove</button>
						</div>
					</div>
				</div>
				<div class="ads-card-flat" style="margin-top:16px;">
					<div class="ads-card-header">
						<span style="font-size:24px;">&#x1F4BE;</span>
						<span class="ads-card-title">JVM Memory Settings</span>
					</div>
					<div style="padding:0 24px 24px;">
						<div style="display:grid; grid-template-columns:1fr 1fr; gap:16px;">
							<div class="ads-form-group">
								<label class="ads-form-label" for="minMemory">Min Memory (Xms)</label>
								<div style="display:flex; gap:8px;">
									<input type="text" id="minMemory" class="ads-input" value="%s" style="flex:1;">
									<button class="ads-button ads-button-primary" onclick="jvmAction('change-jvm-min','minMemory','minMemory')">Apply</button>
								</div>
							</div>
							<div class="ads-form-group">
								<label class="ads-form-label" for="maxMemory">Max Memory (Xmx)</label>
								<div style="display:flex; gap:8px;">
									<input type="text" id="maxMemory" class="ads-input" value="%s" style="flex:1;">
									<button class="ads-button ads-button-primary" onclick="jvmAction('change-jvm-max','maxMemory','maxMemory')">Apply</button>
								</div>
							</div>
						</div>
					</div>
				</div>
				<div style="margin-top:16px;">
					<a href="/environment/get-config/%s/%s" class="ads-button ads-button-default">&larr; Back to Node Selection</a>
				</div>
		</div></div>
		`, sanitizedEnv, sanitizedEnv, sanitizedEnv, sanitizedApp, sanitizedIP,
			sanitizedApp, sanitizedIP, argsRowsHTML,
			html.EscapeString(minMemory), html.EscapeString(maxMemory),
			sanitizedEnv, sanitizedApp)

		username, _ := GetCurrentUsername(r)
		isAdmin, _ := IsAdminUser(username)

		RenderPage(w, PageData{
			Title:     "JVM Config - " + ip,
			IsAdmin:   isAdmin,
			ExtraHead: extraHead,
			Content:   template.HTML(content),
		})
	}
}

func HandleAddJVMArg() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path[len("/environment/add-jvm-arg/"):], "/")
		if len(pathParts) < 3 {
			RenderErrorPage(w, r, "Invalid Path", "Invalid URL path.", "/", "Back to Home", http.StatusBadRequest)
			return
		}

		environmentName := pathParts[0]
		appName := pathParts[1]
		ip := pathParts[2]

		if err := r.ParseForm(); err != nil {
			RenderErrorPage(w, r, "Form Error", "Failed to parse form data.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusBadRequest)
			return
		}
		newArg := r.FormValue("jvmArg")
		if newArg == "" {
			RenderErrorPage(w, r, "Missing Input", "No JVM argument provided.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusBadRequest)
			return
		}

		log.Printf("Received request to add JVM arg. Environment: %s, App: %s, IP: %s, Arg: %s", environmentName, appName, ip, newArg)

		// Fetch server credentials from the database
		var serverUser, serverPassword, installDir string
		err := db.QueryRow("SELECT server_user, server_password, install_dir FROM environments WHERE name = ?", environmentName).Scan(&serverUser, &serverPassword, &installDir)
		if err != nil {
			log.Printf("Failed to retrieve config for environment %s: %v", environmentName, err)
			RenderErrorPage(w, r, "Configuration Error", "Failed to retrieve server configuration. Check logs for details.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		// SSH to the specific IP
		client, err := connectToServer(ip, serverUser, serverPassword)
		if err != nil {
			log.Printf("Failed to connect via SSH: %v", err)
			RenderErrorPage(w, r, "SSH Connection Failed", fmt.Sprintf("Failed to connect to server %s via SSH.", ip), fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}
		defer client.Close()

		// Fetch the current setenv.sh content
		configContent, err := GetConfig(client, appName, installDir, "/path/to/homeDir")
		if err != nil {
			log.Printf("Failed to retrieve setenv.sh content: %v", err)
			RenderErrorPage(w, r, "Configuration Error", "Failed to retrieve setenv.sh content.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusInternalServerError)
			return
		}

		// Find the line with JVM_REQUIRED_ARGS and modify it
		lines := strings.Split(configContent, "\n")
		for i, line := range lines {
			if strings.Contains(line, "JVM_REQUIRED_ARGS='") {
				// Insert the new argument before the closing single quote
				closingQuoteIndex := strings.LastIndex(line, "'")
				if closingQuoteIndex != -1 {
					lines[i] = line[:closingQuoteIndex] + " " + newArg + line[closingQuoteIndex:]
				} else {
					RenderErrorPage(w, r, "Configuration Error", "Malformed JVM_REQUIRED_ARGS value in setenv.sh.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusInternalServerError)
					return
				}
				break
			}
		}

		// Join lines back into the full content
		newContent := strings.Join(lines, "\n")
		log.Printf("Updated setenv.sh content:\n%s", newContent)

		// Write the updated content to a temporary file and then move it with sudo
		tempFile := "/tmp/setenv.sh"
		writeTmpCmd := fmt.Sprintf("echo \"%s\" > %s", newContent, tempFile)
		moveCmd := fmt.Sprintf("sudo mv %s '%s/bin/setenv.sh'", tempFile, installDir)

		// Write the temporary file
		session, err := client.NewSession()
		if err != nil {
			log.Printf("Failed to create SSH session: %v", err)
			RenderErrorPage(w, r, "SSH Error", "Failed to create SSH session.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}
		defer session.Close()

		var output, stderr bytes.Buffer
		session.Stdout = &output
		session.Stderr = &stderr

		if err := session.Run(writeTmpCmd); err != nil {
			log.Printf("Failed to write to temp file: %v, stderr: %s", err, stderr.String())
			RenderErrorPage(w, r, "Write Error", fmt.Sprintf("Failed to write new content to temp file: %s", stderr.String()), fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusInternalServerError)
			return
		}

		// Move the temporary file to the correct location
		session, err = client.NewSession()
		if err != nil {
			log.Printf("Failed to create SSH session: %v", err)
			RenderErrorPage(w, r, "SSH Error", "Failed to create SSH session.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}
		defer session.Close()

		session.Stdout = &output
		session.Stderr = &stderr

		if err := session.Run(moveCmd); err != nil {
			log.Printf("Failed to move temp file to setenv.sh: %v, stderr: %s", err, stderr.String())
			RenderErrorPage(w, r, "Write Error", fmt.Sprintf("Failed to apply configuration changes: %s", stderr.String()), fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("JVM argument added successfully."))
	}
}

func HandleRemoveJVMArg() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path[len("/environment/remove-jvm-arg/"):], "/")
		if len(pathParts) < 3 {
			RenderErrorPage(w, r, "Invalid Path", "Invalid URL path.", "/", "Back to Home", http.StatusBadRequest)
			return
		}

		environmentName := pathParts[0]
		appName := pathParts[1]
		ip := pathParts[2]

		if err := r.ParseForm(); err != nil {
			RenderErrorPage(w, r, "Form Error", "Failed to parse form data.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusBadRequest)
			return
		}
		argToRemove := r.FormValue("jvmArg")
		if argToRemove == "" {
			RenderErrorPage(w, r, "Missing Input", "No JVM argument provided.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusBadRequest)
			return
		}

		// Fetch server credentials from the database
		var serverUser, serverPassword, installDir string
		err := db.QueryRow("SELECT server_user, server_password, install_dir FROM environments WHERE name = ?", environmentName).Scan(&serverUser, &serverPassword, &installDir)
		if err != nil {
			log.Printf("Failed to retrieve config for environment %s: %v", environmentName, err)
			RenderErrorPage(w, r, "Configuration Error", "Failed to retrieve server configuration. Check logs for details.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		// SSH to the specific IP
		client, err := connectToServer(ip, serverUser, serverPassword)
		if err != nil {
			log.Printf("Failed to connect via SSH: %v", err)
			RenderErrorPage(w, r, "SSH Connection Failed", fmt.Sprintf("Failed to connect to server %s via SSH.", ip), fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}
		defer client.Close()

		// Fetch the current setenv.sh content
		configContent, err := GetConfig(client, appName, installDir, "/path/to/homeDir")
		if err != nil {
			log.Printf("Failed to retrieve setenv.sh content: %v", err)
			RenderErrorPage(w, r, "Configuration Error", "Failed to retrieve setenv.sh content.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusInternalServerError)
			return
		}

		// Find the JVM_REQUIRED_ARGS and remove the specified argument
		jvmArgsIndex := strings.Index(configContent, "JVM_REQUIRED_ARGS='")
		if jvmArgsIndex == -1 {
			RenderErrorPage(w, r, "Configuration Error", "JVM_REQUIRED_ARGS not found in setenv.sh.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		// Remove the argument from JVM_REQUIRED_ARGS
		updatedContent := strings.Replace(configContent, " "+argToRemove, "", -1)

		// Write the updated content back to setenv.sh
		session, err := client.NewSession()
		if err != nil {
			log.Printf("Failed to create SSH session: %v", err)
			RenderErrorPage(w, r, "SSH Error", "Failed to create SSH session.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}
		defer session.Close()

		writeCmd := fmt.Sprintf("sudo echo \"%s\" > '%s/bin/setenv.sh'", updatedContent, installDir)
		if err := session.Run(writeCmd); err != nil {
			log.Printf("Failed to write new content to setenv.sh: %v", err)
			RenderErrorPage(w, r, "Write Error", "Failed to write configuration changes. Check logs for details.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("JVM argument removed successfully."))
	}
}

func extractJVMValues(content string) (string, string, error) {
	// Debugging: log the content for visibility
	log.Printf("Extracting JVM values from content: %s", content)

	// Find JVM_MINIMUM_MEMORY
	minMemoryIndex := strings.Index(content, "JVM_MINIMUM_MEMORY=")
	if minMemoryIndex == -1 {
		return "", "", fmt.Errorf("JVM_MINIMUM_MEMORY not found")
	}

	minMemory := content[minMemoryIndex+len("JVM_MINIMUM_MEMORY="):]
	minMemory = strings.TrimSpace(minMemory[:strings.Index(minMemory, "\n")])
	minMemory = strings.Trim(minMemory, "\"") // Remove quotes if present

	// Find JVM_MAXIMUM_MEMORY
	maxMemoryIndex := strings.Index(content, "JVM_MAXIMUM_MEMORY=")
	if maxMemoryIndex == -1 {
		return "", "", fmt.Errorf("JVM_MAXIMUM_MEMORY not found")
	}

	maxMemory := content[maxMemoryIndex+len("JVM_MAXIMUM_MEMORY="):]
	maxMemory = strings.TrimSpace(maxMemory[:strings.Index(maxMemory, "\n")])
	maxMemory = strings.Trim(maxMemory, "\"") // Remove quotes if present

	return minMemory, maxMemory, nil
}

func HandleChangeJVMMin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract environment name, appName, and IP from the URL path
		pathParts := strings.Split(r.URL.Path[len("/environment/change-jvm-min/"):], "/")
		if len(pathParts) < 3 {
			RenderErrorPage(w, r, "Invalid Path", "Invalid URL path.", "/", "Back to Home", http.StatusBadRequest)
			return
		}

		environmentName := pathParts[0]
		appName := pathParts[1]
		ip := pathParts[2]

		// Parse form to get the new minimum memory value
		if err := r.ParseForm(); err != nil {
			RenderErrorPage(w, r, "Form Error", "Failed to parse form data.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusBadRequest)
			return
		}
		newMin := r.FormValue("minMemory")
		if newMin == "" {
			RenderErrorPage(w, r, "Missing Input", "No minimum memory value provided.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusBadRequest)
			return
		}

		// Fetch server credentials from the database
		var serverUser, serverPassword, installDir string
		err := db.QueryRow("SELECT server_user, server_password, install_dir FROM environments WHERE name = ?", environmentName).Scan(&serverUser, &serverPassword, &installDir)
		if err != nil {
			log.Printf("Failed to retrieve config for environment %s: %v", environmentName, err)
			RenderErrorPage(w, r, "Configuration Error", "Failed to retrieve server configuration. Check logs for details.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		// SSH to the specific IP
		client, err := connectToServer(ip, serverUser, serverPassword)
		if err != nil {
			log.Printf("Failed to connect via SSH: %v", err)
			RenderErrorPage(w, r, "SSH Connection Failed", fmt.Sprintf("Failed to connect to server %s via SSH.", ip), fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}
		defer client.Close()

		// Fetch the current setenv.sh content
		configContent, err := GetConfig(client, appName, installDir, "/path/to/homeDir")
		if err != nil {
			log.Printf("Failed to retrieve setenv.sh content: %v", err)
			RenderErrorPage(w, r, "Configuration Error", "Failed to retrieve setenv.sh content.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusInternalServerError)
			return
		}

		// Replace the JVM_MINIMUM_MEMORY value in the content
		lines := strings.Split(configContent, "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "JVM_MINIMUM_MEMORY=") {
				lines[i] = fmt.Sprintf(`JVM_MINIMUM_MEMORY="%s"`, newMin)
				break
			}
		}

		// Join the modified lines back into the updated content
		updatedContent := strings.Join(lines, "\n")

		// Write the updated content back to setenv.sh using a here document
		session, err := client.NewSession()
		if err != nil {
			log.Printf("Failed to create SSH session: %v", err)
			RenderErrorPage(w, r, "SSH Error", "Failed to create SSH session.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}
		defer session.Close()

		// Use a here document to replace the file content
		writeCmd := fmt.Sprintf("sudo tee '%s/bin/setenv.sh' > /dev/null <<EOF\n%s\nEOF", installDir, updatedContent)
		if err := session.Run(writeCmd); err != nil {
			log.Printf("Failed to write new content to setenv.sh: %v", err)
			RenderErrorPage(w, r, "Write Error", "Failed to write configuration changes. Check logs for details.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("JVM minimum memory changed successfully."))
	}
}

func HandleChangeJVMMax() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract environment name, appName, and IP from the URL path
		pathParts := strings.Split(r.URL.Path[len("/environment/change-jvm-max/"):], "/")
		if len(pathParts) < 3 {
			RenderErrorPage(w, r, "Invalid Path", "Invalid URL path.", "/", "Back to Home", http.StatusBadRequest)
			return
		}

		environmentName := pathParts[0]
		appName := pathParts[1]
		ip := pathParts[2]

		// Parse form to get the new maximum memory value
		if err := r.ParseForm(); err != nil {
			RenderErrorPage(w, r, "Form Error", "Failed to parse form data.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusBadRequest)
			return
		}
		newMax := r.FormValue("maxMemory")
		if newMax == "" {
			RenderErrorPage(w, r, "Missing Input", "No maximum memory value provided.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusBadRequest)
			return
		}

		// Fetch server credentials from the database
		var serverUser, serverPassword, installDir string
		err := db.QueryRow("SELECT server_user, server_password, install_dir FROM environments WHERE name = ?", environmentName).Scan(&serverUser, &serverPassword, &installDir)
		if err != nil {
			log.Printf("Failed to retrieve config for environment %s: %v", environmentName, err)
			RenderErrorPage(w, r, "Configuration Error", "Failed to retrieve server configuration. Check logs for details.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		// SSH to the specific IP
		client, err := connectToServer(ip, serverUser, serverPassword)
		if err != nil {
			log.Printf("Failed to connect via SSH: %v", err)
			RenderErrorPage(w, r, "SSH Connection Failed", fmt.Sprintf("Failed to connect to server %s via SSH.", ip), fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}
		defer client.Close()

		// Fetch the current setenv.sh content
		configContent, err := GetConfig(client, appName, installDir, "/path/to/homeDir")
		if err != nil {
			log.Printf("Failed to retrieve setenv.sh content: %v", err)
			RenderErrorPage(w, r, "Configuration Error", "Failed to retrieve setenv.sh content.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusInternalServerError)
			return
		}

		// Replace the JVM_MAXIMUM_MEMORY value in the content
		lines := strings.Split(configContent, "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "JVM_MAXIMUM_MEMORY=") {
				lines[i] = fmt.Sprintf(`JVM_MAXIMUM_MEMORY="%s"`, newMax)
				break
			}
		}

		// Join the modified lines back into the updated content
		updatedContent := strings.Join(lines, "\n")

		// Write the updated content back to setenv.sh using a here document
		session, err := client.NewSession()
		if err != nil {
			log.Printf("Failed to create SSH session: %v", err)
			RenderErrorPage(w, r, "SSH Error", "Failed to create SSH session.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}
		defer session.Close()

		// Use a here document to replace the file content
		writeCmd := fmt.Sprintf("sudo tee '%s/bin/setenv.sh' > /dev/null <<EOF\n%s\nEOF", installDir, updatedContent)
		if err := session.Run(writeCmd); err != nil {
			log.Printf("Failed to write new content to setenv.sh: %v", err)
			RenderErrorPage(w, r, "Write Error", "Failed to write configuration changes. Check logs for details.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("JVM maximum memory changed successfully."))
	}
}

func extractJVMMin(content string) string {
	minMemoryIndex := strings.Index(content, "JVM_MINIMUM_MEMORY=")
	if minMemoryIndex == -1 {
		log.Printf("JVM_MINIMUM_MEMORY not found in the content.")
		return ""
	}

	// Extract the substring starting at JVM_MINIMUM_MEMORY=
	minMemoryContent := content[minMemoryIndex+len("JVM_MINIMUM_MEMORY="):]

	// Locate the end of the line or the end of the value
	endOfLineIndex := strings.Index(minMemoryContent, "\n")
	if endOfLineIndex == -1 {
		endOfLineIndex = len(minMemoryContent)
	}

	minMemory := strings.TrimSpace(minMemoryContent[:endOfLineIndex])
	minMemory = strings.Trim(minMemory, "\"") // Remove quotes if present

	return minMemory
}

func extractJVMMax(content string) string {
	maxMemoryIndex := strings.Index(content, "JVM_MAXIMUM_MEMORY=")
	if maxMemoryIndex == -1 {
		return ""
	}

	maxMemory := content[maxMemoryIndex+len("JVM_MAXIMUM_MEMORY=\""):]
	maxMemory = maxMemory[:strings.Index(maxMemory, "\"")]
	return maxMemory
}
func HandleSSLPoke() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path[len("/environment/java-version/"):], "/")
		if len(pathParts) < 3 {
			RenderErrorPage(w, r, "Invalid Path", "Invalid URL path.", "/", "Back to Home", http.StatusBadRequest)
			return
		}

		environmentName := pathParts[0]
		appName := pathParts[1]
		ip := pathParts[2]

		// Fetch serverUser, serverPassword, installDir from the DB
		var serverUser, serverPassword, installDir string
		err := db.QueryRow("SELECT server_user, server_password, install_dir FROM environments WHERE name = ?", environmentName).Scan(&serverUser, &serverPassword, &installDir)
		if err != nil {
			log.Printf("Failed to retrieve config for environment %s: %v", environmentName, err)
			RenderErrorPage(w, r, "Configuration Error", "Failed to retrieve server configuration. Check logs for details.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
			return
		}

		// Check if this is a POST request (i.e., form submission)
		if r.Method == "POST" {
			if err := r.ParseForm(); err != nil {
				RenderErrorPage(w, r, "Form Error", "Failed to parse form data.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusBadRequest)
				return
			}
			urlPort := r.FormValue("urlPort")
			if urlPort == "" {
				RenderErrorPage(w, r, "Missing Input", "No URL:Port provided.", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusBadRequest)
				return
			}

			// Split the URL:Port
			parts := strings.Split(urlPort, ":")
			if len(parts) != 2 {
				RenderErrorPage(w, r, "Invalid Input", "Invalid URL:Port format. Expected format: url:port", fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusBadRequest)
				return
			}
			url, port := parts[0], parts[1]

			// SSH to the specific IP
			client, err := connectToServer(ip, serverUser, serverPassword)
			if err != nil {
				log.Printf("Failed to connect via SSH: %v", err)
				RenderErrorPage(w, r, "SSH Connection Failed", fmt.Sprintf("Failed to connect to server %s via SSH.", ip), fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
				return
			}
			defer client.Close()

			// Run the java command using the install_dir path and provided URL:Port
			session, err := client.NewSession()
			if err != nil {
				log.Printf("Failed to create SSH session: %v", err)
				RenderErrorPage(w, r, "SSH Error", "Failed to create SSH session.", fmt.Sprintf("/environment/%s", environmentName), "Back to Environment", http.StatusInternalServerError)
				return
			}
			defer session.Close()

			// Corrected command with classpath
			cmd := fmt.Sprintf("%s/jre/bin/java SSLPoke %s %s 2>&1", installDir, url, port)

			var output, stderr strings.Builder
			session.Stdout = &output
			session.Stderr = &stderr

			if err := session.Run(cmd); err != nil {
				log.Printf("Failed to execute java command: %v", err)
				RenderErrorPage(w, r, "Java Error", fmt.Sprintf("Failed to execute Java command: %s", stderr.String()), fmt.Sprintf("/environment/get-config-ip/%s/%s/%s", environmentName, appName, ip), "Back to Configuration", http.StatusInternalServerError)
				return
			}

			// Return the Java command output directly to the client
			w.Write([]byte(output.String()))
		} else {
			// If it's not a POST request, return the HTML form
			html := `
            <!DOCTYPE html>
            <html lang="en">
            <head>
                <meta charset="UTF-8">
                <meta name="viewport" content="width=device-width, initial-scale=1.0">
                <link rel="stylesheet" href="/static/styles.css">
                <script>
                    function executeJavaCommand() {
                        var urlPort = document.getElementById("urlPort").value;
                        var xhr = new XMLHttpRequest();
                        xhr.open("POST", "/environment/java-version/` + environmentName + `/` + appName + `/` + ip + `", true);
                        xhr.setRequestHeader("Content-Type", "application/x-www-form-urlencoded");
                        xhr.onload = function() {
                            if (xhr.status === 200) {
                                document.getElementById("output").innerHTML = xhr.responseText;
                            } else {
                                alert("Failed to execute Java command. Check logs for details.");
                            }
                        };
                        xhr.send("urlPort=" + encodeURIComponent(urlPort));
                    }
                </script>
            </head>
            <body>
                <br>
                <h2>Test SSL Connection to Other Servers</h2>
                <div class="input-group">
                    <input type="text" id="urlPort" placeholder="Enter URL:Port" />
                    <button onclick="executeJavaCommand()">Poke!</button>
                </div>
                <div id="output"></div>
            </body>
            </html>
            `
			w.Write([]byte(html))
		}
	}
}
