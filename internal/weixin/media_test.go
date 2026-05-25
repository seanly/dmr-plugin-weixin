package weixin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetMimeFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"test.jpg", "image/jpeg"},
		{"test.jpeg", "image/jpeg"},
		{"test.png", "image/png"},
		{"test.gif", "image/gif"},
		{"test.webp", "image/webp"},
		{"test.mp4", "video/mp4"},
		{"test.mov", "video/quicktime"},
		{"test.avi", "video/x-msvideo"},
		{"test.pdf", "application/pdf"},
		{"test.doc", "application/msword"},
		{"test.zip", "application/zip"},
		{"test.txt", "text/plain"},
		{"test.unknown", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := getMimeFromFilename(tt.filename)
			if result != tt.expected {
				t.Errorf("getMimeFromFilename(%q) = %q, want %q", tt.filename, result, tt.expected)
			}
		})
	}
}

func TestAesEcbPaddedSize(t *testing.T) {
	tests := []struct {
		plaintextSize int64
		expected      int64
	}{
		{0, 16},    // Empty file needs full block padding
		{1, 16},    // 1 byte needs 15 bytes padding
		{15, 16},   // 15 bytes needs 1 byte padding
		{16, 32},   // 16 bytes needs full block padding
		{17, 32},   // 17 bytes needs 15 bytes padding
		{100, 112}, // 100 bytes needs 12 bytes padding
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := aesEcbPaddedSize(tt.plaintextSize)
			if result != tt.expected {
				t.Errorf("aesEcbPaddedSize(%d) = %d, want %d", tt.plaintextSize, result, tt.expected)
			}
		})
	}
}

func TestEncryptAesEcb(t *testing.T) {
	plaintext := []byte("Hello, World!")
	key := []byte("0123456789abcdef") // 16 bytes key

	ciphertext, err := encryptAesEcb(plaintext, key)
	if err != nil {
		t.Fatalf("encryptAesEcb failed: %v", err)
	}

	// Ciphertext should be padded to block size
	if len(ciphertext)%16 != 0 {
		t.Errorf("ciphertext length %d is not a multiple of 16", len(ciphertext))
	}

	// Ciphertext should be longer than plaintext due to padding
	if len(ciphertext) <= len(plaintext) {
		t.Errorf("ciphertext length %d should be > plaintext length %d", len(ciphertext), len(plaintext))
	}
}

func TestGetExtensionFromContentType(t *testing.T) {
	tests := []struct {
		contentType string
		expected    string
	}{
		{"image/jpeg", ".jpg"},
		{"image/png", ".png"},
		{"image/gif", ".gif"},
		{"image/webp", ".webp"},
		{"video/mp4", ".mp4"},
		{"video/quicktime", ".mov"},
		{"application/pdf", ".pdf"},
		{"text/plain", ""},
		{"application/octet-stream", ""},
		{"image/jpeg; charset=utf-8", ".jpg"}, // With parameters
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result := getExtensionFromContentType(tt.contentType)
			if result != tt.expected {
				t.Errorf("getExtensionFromContentType(%q) = %q, want %q", tt.contentType, result, tt.expected)
			}
		})
	}
}

func TestSendFileToolParamsJSON(t *testing.T) {
	result := sendFileToolParamsJSON()
	if result == "" {
		t.Error("sendFileToolParamsJSON returned empty string")
	}

	if result[0] != '{' || result[len(result)-1] != '}' {
		t.Error("sendFileToolParamsJSON did not return valid JSON object")
	}
}

func TestMergeOutboundKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     map[string]any
		want    string
		wantErr string
	}{
		{name: "empty", raw: map[string]any{}, want: ""},
		{name: "media only string", raw: map[string]any{"media_type": "2"}, want: "video"},
		{name: "media numeric JSON float", raw: map[string]any{"media_type": float64(3)}, want: "file"},
		{name: "file only", raw: map[string]any{"file_type": "auto"}, want: "auto"},
		{name: "both matching", raw: map[string]any{"file_type": "image", "media_type": "1"}, want: "image"},
		{name: "conflict", raw: map[string]any{"file_type": "image", "media_type": "2"}, wantErr: "conflicts"},
		{name: "voice", raw: map[string]any{"media_type": "voice"}, wantErr: "not implemented"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeOutboundKind(tt.raw)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("mergeOutboundKind(%v): err=%v want substring %q", tt.raw, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("mergeOutboundKind: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestBuildCdnUploadUrl(t *testing.T) {
	tests := []struct {
		name        string
		cdnBaseURL  string
		uploadParam string
		filekey     string
		wantErr     bool
		wantSubstr  []string // must all appear (openclaw: encrypted_query_param + filekey only, no filetype)
	}{
		{
			name:        "c2c base",
			cdnBaseURL:  "https://cdn.example.com/c2c",
			uploadParam: "test_param",
			filekey:     "test_key",
			wantErr:     false,
			wantSubstr:  []string{"/upload?", "encrypted_query_param=", "filekey="},
		},
		{
			name:        "trailing slash base",
			cdnBaseURL:  "https://cdn.example.com/c2c/",
			uploadParam: "a=b&c",
			filekey:     "k",
			wantErr:     false,
			wantSubstr:  []string{"https://cdn.example.com/c2c/upload?", "encrypted_query_param=", "filekey="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := buildCdnUploadUrl(tt.cdnBaseURL, tt.uploadParam, tt.filekey)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildCdnUploadUrl() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result == "" {
				t.Fatal("buildCdnUploadUrl() returned empty URL")
			}
			if !tt.wantErr && strings.Contains(result, "filetype=") {
				t.Error("buildCdnUploadUrl() must not add filetype= (openclaw-weixin cdn-url.ts parity)")
			}
			for _, sub := range tt.wantSubstr {
				if !strings.Contains(result, sub) {
					t.Errorf("result %q missing %q", result, sub)
				}
			}
		})
	}
}

func TestUploadMediaToCdn(t *testing.T) {
	// Create a temporary test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := []byte("Hello, World! This is a test file.")

	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create minimal bot (no real gateway; exercise pipeline until HTTP fails).
	p := NewWeixinPlugin()
	b := &weixinBot{
		wp:             p,
		gatewayBaseURL: "http://127.0.0.1:9",
		token:          "tok",
		cdnBaseURL:     "https://cdn.example.com/c2c",
		tokens:         newContextTokenStore(),
	}

	// Note: This test will fail without a real CDN endpoint
	// It's mainly to verify the function signature and basic logic
	ctx := context.Background()
	_, err := b.uploadMediaToCdn(ctx, testFile, "test_user", UploadMediaTypeImage)

	// We expect an error since we don't have a real gateway
	if err == nil {
		t.Log("uploadMediaToCdn succeeded (unexpected in unit test)")
	} else {
		t.Logf("uploadMediaToCdn failed as expected: %v", err)
	}
}
