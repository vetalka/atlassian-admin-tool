package handlers

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "crypto/sha256"
    "database/sql"
    "encoding/hex"
    "errors"
    "fmt"
    "io"
    "log"
    "net/http"
    "strings"
    "time"
)

const encryptionKey = "your-64-byte-key-that-needs-to-be-hashed-12345678901234567890123456789012" // Replace with your actual key

// hashKey hashes the key using SHA-256 to ensure it is 32 bytes long
func hashKey(key string) []byte {
    hashedKey := sha256.Sum256([]byte(key))
    return hashedKey[:]
}

// EncryptLicense encrypts the license string using AES
func EncryptLicense(license, key string) (string, error) {
    block, err := aes.NewCipher(hashKey(key))
    if err != nil {
        return "", err
    }

    aesGCM, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }

    nonce := make([]byte, aesGCM.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", err
    }

    ciphertext := aesGCM.Seal(nonce, nonce, []byte(license), nil)
    encryptedLicense := hex.EncodeToString(ciphertext)

    log.Printf("Encrypted License: %s", encryptedLicense) // Logging the encrypted license
    return encryptedLicense, nil
}

// DecryptLicense decrypts the encrypted license string
func DecryptLicense(encryptedLicense, key string) (string, error) {
    block, err := aes.NewCipher(hashKey(key))
    if err != nil {
        return "", err
    }

    aesGCM, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }

    data, err := hex.DecodeString(encryptedLicense)
    if err != nil {
        return "", err
    }

    nonceSize := aesGCM.NonceSize()
    if len(data) < nonceSize {
        return "", errors.New("ciphertext too short")
    }

    nonce, ciphertext := data[:nonceSize], data[nonceSize:]

    plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return "", err
    }

    log.Printf("Decrypted License: %s", string(plaintext)) // Logging the decrypted license
    return string(plaintext), nil
}

// ValidateLicense validates the decrypted license and saves it if valid
func ValidateLicense(encryptedLicense string) (bool, error) {
    log.Printf("Validating License: %s", encryptedLicense) // Log before validation

    decryptedLicense, err := DecryptLicense(encryptedLicense, encryptionKey)
    if err != nil {
        return false, fmt.Errorf("invalid license: %v", err)
    }

    parts := strings.Split(decryptedLicense, "-")
    if len(parts) != 3 || parts[0] != "Methoda" {
        return false, errors.New("invalid license format")
    }

    expirationDate, err := time.Parse("2006.01.02", parts[1])
    if err != nil {
        return false, fmt.Errorf("invalid date format in license: %v", err)
    }

    // Check if the license has expired
    if time.Now().After(expirationDate) {
        return false, errors.New("license has expired")
    }

    // Save license to the database
    _, err = db.Exec("INSERT INTO license (key, expiry_date) VALUES (?, ?)", encryptedLicense, expirationDate.Format("2006-01-02"))
    if err != nil {
        return false, fmt.Errorf("failed to save license to the database: %v", err)
    }

    log.Println("License is valid and saved to the database.")
    return true, nil
}

// HandleLicenseSetup handles the license setup page
func HandleLicenseSetup(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet {
        var count int
        err := db.QueryRow("SELECT COUNT(*) FROM license").Scan(&count)
        if err != nil && err != sql.ErrNoRows {
            http.Error(w, "Database error", http.StatusInternalServerError)
            return
        }

        if count > 0 {
            http.Redirect(w, r, "/create-user", http.StatusSeeOther)
            return
        }

        html := `
        <!DOCTYPE html>
        <html lang="en">
        <head>
            <meta charset="UTF-8">
            <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <link rel="stylesheet" href="/static/styles.css">
            <title>License Setup</title>
            
        </head>
        <body>
            <div class="container">
                <h1>License Setup</h1>
                <form action="/license-setup" method="POST">
                    <label for="license">Enter Your License Key:</label>
                    <textarea id="license" name="license" rows="1" required style="width: 100%; height: 180px; padding: 15px; font-size: 16px; box-sizing: border-box; overflow: visible; resize: none;"></textarea><br>
                    <button type="submit">Submit</button>
                </form>
            </div>
        </body>
        </html>
        `
        fmt.Fprintln(w, html)
    } else if r.Method == http.MethodPost {
        encryptedLicense := r.FormValue("license")

        // Log the received license
        log.Printf("Received License: %s", encryptedLicense)

        valid, err := ValidateLicense(encryptedLicense)
        if err != nil || !valid {
            http.Error(w, "Invalid license: "+err.Error(), http.StatusBadRequest)
            return
        }

        http.Redirect(w, r, "/create-user", http.StatusSeeOther)
    }
}


// IsLicenseSetUp checks if a license is already set up in the database
func IsLicenseSetUp() bool {
    var count int
    err := db.QueryRow("SELECT COUNT(*) FROM license").Scan(&count)
    if err != nil && err != sql.ErrNoRows {
        log.Fatalf("Failed to query license setup: %v", err)
    }

    return count > 0
}

