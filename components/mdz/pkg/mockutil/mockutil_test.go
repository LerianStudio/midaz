package mockutil

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadJSONResponse(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.json")
	testContent := `{"test": "data"}`

	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test successful file reading
	data, err := loadJSONResponse(testFile)
	if err != nil {
		t.Errorf("loadJSONResponse should not return error: %v", err)
	}
	if string(data) != testContent {
		t.Errorf("Expected content %q, got %q", testContent, string(data))
	}

	// Test file that doesn't exist
	nonExistentFile := filepath.Join(tmpDir, "nonexistent.json")
	_, err = loadJSONResponse(nonExistentFile)
	if err == nil {
		t.Error("loadJSONResponse should return error for non-existent file")
	}
}

func TestMockResponseFromFile_Success(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "success.json")
	testContent := `{"status": "success"}`

	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test successful response creation
	responder := MockResponseFromFile(http.StatusOK, testFile)
	if responder == nil {
		t.Fatal("MockResponseFromFile should not return nil responder")
	}

	// Create a mock HTTP request to test the responder
	req, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Fatalf("Failed to create test request: %v", err)
	}

	resp, err := responder(req)
	if err != nil {
		t.Errorf("Responder should not return error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
}

func TestMockResponseFromFile_FileNotFound(t *testing.T) {
	// Test with non-existent file
	nonExistentFile := "/path/that/does/not/exist/file.json"

	responder := MockResponseFromFile(http.StatusOK, nonExistentFile)
	if responder == nil {
		t.Fatal("MockResponseFromFile should not return nil responder even for missing files")
	}

	// Create a mock HTTP request to test the responder
	req, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Fatalf("Failed to create test request: %v", err)
	}

	resp, err := responder(req)
	if err != nil {
		t.Errorf("Responder should not return error: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status %d for missing file, got %d", http.StatusInternalServerError, resp.StatusCode)
	}
}

func TestMockResponseFromFile_DifferentStatusCodes(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.json")
	testContent := `{"message": "test"}`

	err := os.WriteFile(testFile, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	statusCodes := []int{
		http.StatusOK,
		http.StatusCreated,
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusInternalServerError,
	}

	for _, statusCode := range statusCodes {
		responder := MockResponseFromFile(statusCode, testFile)
		req, err := http.NewRequest("GET", "http://example.com", nil)
		if err != nil {
			t.Fatalf("Failed to create test request: %v", err)
		}

		resp, err := responder(req)
		if err != nil {
			t.Errorf("Responder should not return error: %v", err)
		}
		if resp.StatusCode != statusCode {
			t.Errorf("Expected status %d, got %d", statusCode, resp.StatusCode)
		}
	}
}
