// handlers/create_folder.go
package handlers

import (
	"fmt"
	"net/http"
)

func HandleCreateFolder(w http.ResponseWriter, r *http.Request) {
	html := `
	<!DOCTYPE html>
	<html lang="en">
	<head>
		<meta charset="UTF-8">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
		<title>Create Folder</title>
		<link rel="stylesheet" href="/static/styles.css">
	</head>
	<body>
		<h1>Create Folder</h1>
		<form action="/create-folder" method="POST">
			<label for="folderName">Enter Folder Name:</label>
			<input type="text" id="folderName" name="folderName" required><br>
			<label for="serverIP">Server IP:</label>
			<input type="text" id="serverIP" name="serverIP" required><br>
			<label for="sshUser">SSH Username:</label>
			<input type="text" id="sshUser" name="sshUser" required><br>
			<button type="submit">Create Folder</button>
			<button type="button" onclick="goBack()" class="back-button">Back to Main Menu</button>
		</form>
		<script src="/static/scripts.js"></script>
	</body>
	</html>
	`
	fmt.Fprintln(w, html)
}



