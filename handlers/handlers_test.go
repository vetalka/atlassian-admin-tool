package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

// ─────────────────────────────────────────────
// utils.go — hashPassword / checkPasswordHash
// ─────────────────────────────────────────────

func TestHashPassword_ReturnsNonEmptyHash(t *testing.T) {
	hash, err := hashPassword("MySecret123!")
	if err != nil {
		t.Fatalf("hashPassword returned error: %v", err)
	}
	if hash == "" {
		t.Fatal("expected a non-empty hash")
	}
	if hash == "MySecret123!" {
		t.Fatal("hash must not equal the plain-text password")
	}
}

func TestCheckPasswordHash_CorrectPassword(t *testing.T) {
	hash, _ := hashPassword("correct-password")
	if !checkPasswordHash("correct-password", hash) {
		t.Fatal("checkPasswordHash should return true for the correct password")
	}
}

func TestCheckPasswordHash_WrongPassword(t *testing.T) {
	hash, _ := hashPassword("correct-password")
	if checkPasswordHash("wrong-password", hash) {
		t.Fatal("checkPasswordHash should return false for a wrong password")
	}
}

func TestCheckPasswordHash_EmptyPassword(t *testing.T) {
	hash, _ := hashPassword("some-password")
	if checkPasswordHash("", hash) {
		t.Fatal("empty password should not match a non-empty hash")
	}
}

// ─────────────────────────────────────────────
// add_environment.go — isSecuredPassword
// ─────────────────────────────────────────────

func TestIsSecuredPassword_EmptyString(t *testing.T) {
	if !isSecuredPassword("") {
		t.Error("empty password should be considered secured/placeholder")
	}
}

func TestIsSecuredPassword_WhitespaceOnly(t *testing.T) {
	if !isSecuredPassword("   ") {
		t.Error("whitespace-only password should be considered secured/placeholder")
	}
}

func TestIsSecuredPassword_ATLSecuredMarker(t *testing.T) {
	if !isSecuredPassword("{ATL_SECURED}sometoken") {
		t.Error("{ATL_SECURED} prefix should be detected")
	}
}

func TestIsSecuredPassword_ENCRYPTEDMarker(t *testing.T) {
	if !isSecuredPassword("{ENCRYPTED}sometoken") {
		t.Error("{ENCRYPTED} prefix should be detected")
	}
}

func TestIsSecuredPassword_PlainPassword(t *testing.T) {
	if isSecuredPassword("MyRealPassword!") {
		t.Error("plain password should NOT be treated as secured/placeholder")
	}
}

// ─────────────────────────────────────────────
// add_environment.go — mapDBDriverToDBType
// ─────────────────────────────────────────────

func TestMapDBDriverToDBType_Postgres(t *testing.T) {
	got, err := mapDBDriverToDBType("org.postgresql.Driver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "postgresql" {
		t.Errorf("expected postgresql, got %s", got)
	}
}

func TestMapDBDriverToDBType_SQLServer(t *testing.T) {
	got, err := mapDBDriverToDBType("com.microsoft.sqlserver.jdbc.SQLServerDriver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "sqlserver" {
		t.Errorf("expected sqlserver, got %s", got)
	}
}

func TestMapDBDriverToDBType_Unsupported(t *testing.T) {
	_, err := mapDBDriverToDBType("com.mysql.jdbc.Driver")
	if err == nil {
		t.Fatal("expected an error for an unsupported driver")
	}
}

func TestMapDBDriverToDBType_Empty(t *testing.T) {
	_, err := mapDBDriverToDBType("")
	if err == nil {
		t.Fatal("expected an error for an empty driver string")
	}
}

// ─────────────────────────────────────────────
// ad_handlers.go — contains
// ─────────────────────────────────────────────

func TestContains_ItemPresent(t *testing.T) {
	slice := []string{"jira", "confluence", "bitbucket"}
	if !contains(slice, "confluence") {
		t.Error("expected contains to return true")
	}
}

func TestContains_ItemAbsent(t *testing.T) {
	slice := []string{"jira", "confluence"}
	if contains(slice, "bitbucket") {
		t.Error("expected contains to return false")
	}
}

func TestContains_EmptySlice(t *testing.T) {
	if contains([]string{}, "jira") {
		t.Error("contains on empty slice should return false")
	}
}

func TestContains_CaseSensitive(t *testing.T) {
	slice := []string{"Jira"}
	if contains(slice, "jira") {
		t.Error("contains should be case-sensitive")
	}
}

// ─────────────────────────────────────────────
// app_backup.go — extractEnvironmentName
// ─────────────────────────────────────────────

func TestExtractEnvironmentName_StandardPath(t *testing.T) {
	got := extractEnvironmentName("/environment/backup/my-jira")
	if got != "my-jira" {
		t.Errorf("expected my-jira, got %s", got)
	}
}

func TestExtractEnvironmentName_DeepPath(t *testing.T) {
	// len(parts) > 4 → join from index 3 onward
	got := extractEnvironmentName("/environment/backup/prod/jira-dc")
	if got != "prod/jira-dc" {
		t.Errorf("expected prod/jira-dc, got %s", got)
	}
}

func TestExtractEnvironmentName_ShortPath(t *testing.T) {
	got := extractEnvironmentName("/environment/jira")
	if got == "" {
		t.Error("should return the last path segment for short paths")
	}
}

// ─────────────────────────────────────────────
// app_restore.go — extractEnvironmentName1 / extractEnvironmentNameWork
// ─────────────────────────────────────────────

func TestExtractEnvironmentName1_ValidPath(t *testing.T) {
	got := extractEnvironmentName1("/environment/restore/my-confluence/start")
	if got != "my-confluence" {
		t.Errorf("expected my-confluence, got %s", got)
	}
}

func TestExtractEnvironmentName1_ShortPath(t *testing.T) {
	got := extractEnvironmentName1("/short")
	if got != "" {
		t.Errorf("expected empty string for short path, got %s", got)
	}
}

func TestExtractEnvironmentNameWork_ValidPath(t *testing.T) {
	got := extractEnvironmentNameWork("/environment/restore/bitbucket-prod")
	if got != "bitbucket-prod" {
		t.Errorf("expected bitbucket-prod, got %s", got)
	}
}

// ─────────────────────────────────────────────
// app_application_version.go — escapeBackslashes / escapePowerShellQuotes
// ─────────────────────────────────────────────

func TestEscapeBackslashes_WindowsPath(t *testing.T) {
	got := escapeBackslashes(`C:\Program Files\Jira`)
	expected := `C:\\Program Files\\Jira`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestEscapeBackslashes_NoBackslashes(t *testing.T) {
	got := escapeBackslashes("/opt/jira/home")
	if got != "/opt/jira/home" {
		t.Errorf("unix paths should pass through unchanged, got %s", got)
	}
}

func TestEscapePowerShellQuotes_WithQuotes(t *testing.T) {
	got := escapePowerShellQuotes(`Invoke-Command "test"`)
	expected := `Invoke-Command \"test\"`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestEscapePowerShellQuotes_NoQuotes(t *testing.T) {
	got := escapePowerShellQuotes("Get-Process")
	if got != "Get-Process" {
		t.Errorf("string without quotes should be unchanged, got %s", got)
	}
}

// ─────────────────────────────────────────────
// license.go — EncryptLicense / DecryptLicense
// ─────────────────────────────────────────────

func TestEncryptDecryptLicense_RoundTrip(t *testing.T) {
	plaintext := "Methoda-2030.01.01-Community"
	encrypted, err := EncryptLicense(plaintext, encryptionKey)
	if err != nil {
		t.Fatalf("EncryptLicense error: %v", err)
	}
	if encrypted == "" || encrypted == plaintext {
		t.Fatal("encryption produced empty or unchanged output")
	}

	decrypted, err := DecryptLicense(encrypted, encryptionKey)
	if err != nil {
		t.Fatalf("DecryptLicense error: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("expected %q after round-trip, got %q", plaintext, decrypted)
	}
}

func TestDecryptLicense_WrongKey(t *testing.T) {
	encrypted, _ := EncryptLicense("Methoda-2030.01.01-Community", encryptionKey)
	_, err := DecryptLicense(encrypted, "wrong-key")
	if err == nil {
		t.Fatal("expected decryption to fail with the wrong key")
	}
}

func TestDecryptLicense_InvalidHex(t *testing.T) {
	_, err := DecryptLicense("not-valid-hex!!", encryptionKey)
	if err == nil {
		t.Fatal("expected an error for invalid hex input")
	}
}

func TestDecryptLicense_TooShort(t *testing.T) {
	_, err := DecryptLicense("aabbcc", encryptionKey)
	if err == nil {
		t.Fatal("expected an error for a ciphertext that is too short")
	}
}

// ─────────────────────────────────────────────
// app_backup.go — RemoveBackupDirectory
// ─────────────────────────────────────────────

func TestRemoveBackupDirectory_RemovesDirectory(t *testing.T) {
	dir := t.TempDir()
	// Create a file inside
	if err := os.WriteFile(filepath.Join(dir, "backup.sql"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := RemoveBackupDirectory(dir); err != nil {
		t.Fatalf("RemoveBackupDirectory returned error: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("directory should have been removed")
	}
}

func TestRemoveBackupDirectory_NonExistentPath(t *testing.T) {
	// os.RemoveAll on a non-existent path is a no-op — should not error
	err := RemoveBackupDirectory("/tmp/does-not-exist-admintest-xyz")
	if err != nil {
		t.Errorf("expected no error for non-existent path, got: %v", err)
	}
}
