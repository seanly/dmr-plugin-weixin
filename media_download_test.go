package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteMediaFile_PathFormat(t *testing.T) {
	tmpDir := t.TempDir()
	p := &WeixinPlugin{cfg: WeixinConfig{Workspace: tmpDir}}

	path, err := p.writeMediaFile([]byte("test data"), "", ".jpg")
	if err != nil {
		t.Fatal(err)
	}

	// Should be under workspace/weixin/{date}/
	if !strings.HasPrefix(path, filepath.Join(tmpDir, "weixin")) {
		t.Errorf("path %q not under workspace/weixin/", path)
	}
	if !strings.HasSuffix(path, ".jpg") {
		t.Errorf("path %q missing .jpg suffix", path)
	}

	// File should exist
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not found: %v", err)
	}
}

func TestWriteMediaFile_WithFileName(t *testing.T) {
	tmpDir := t.TempDir()
	p := &WeixinPlugin{cfg: WeixinConfig{Workspace: tmpDir}}

	path, err := p.writeMediaFile([]byte("pdf content"), "report.pdf", ".pdf")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(path, "report.pdf") {
		t.Errorf("path %q should contain original filename", path)
	}
}

func TestWriteMediaFile_FallbackWorkspace(t *testing.T) {
	p := &WeixinPlugin{cfg: WeixinConfig{Workspace: ""}}

	path, err := p.writeMediaFile([]byte("data"), "", ".png")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	if !strings.HasPrefix(path, "/tmp/dmr-weixin-media/weixin/") {
		t.Errorf("fallback path %q unexpected", path)
	}
}

func TestExtractMediaInfo(t *testing.T) {
	tests := []struct {
		name     string
		item     messageItem
		wantType string
		wantExt  string
		wantNil  bool
	}{
		{
			name:     "image",
			item:     messageItem{Type: itemTypeImage, ImageItem: &imageItem{Media: &cdnMedia{EncryptQueryParam: "p", AesKey: "k"}}},
			wantType: "image",
			wantExt:  ".jpg",
		},
		{
			name:     "file with name",
			item:     messageItem{Type: itemTypeFile, FileItem: &fileItem{Media: &cdnMedia{EncryptQueryParam: "p", AesKey: "k"}, FileName: "doc.pdf"}},
			wantType: "file",
			wantExt:  ".pdf",
		},
		{
			name:     "video",
			item:     messageItem{Type: itemTypeVideo, VideoItem: &videoItem{Media: &cdnMedia{EncryptQueryParam: "p", AesKey: "k"}}},
			wantType: "video",
			wantExt:  ".mp4",
		},
		{
			name:    "text item returns nil",
			item:    messageItem{Type: itemTypeText, TextItem: &textItem{Text: "hi"}},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			media, typeName, _, ext := extractMediaInfo(tt.item)
			if tt.wantNil {
				if media != nil {
					t.Error("expected nil media")
				}
				return
			}
			if media == nil {
				t.Fatal("expected non-nil media")
			}
			if typeName != tt.wantType {
				t.Errorf("type: got %q, want %q", typeName, tt.wantType)
			}
			if ext != tt.wantExt {
				t.Errorf("ext: got %q, want %q", ext, tt.wantExt)
			}
		})
	}
}

func TestSanitizeFileName(t *testing.T) {
	if got := sanitizeFileName("a/b\\c.txt"); got != "a_b_c.txt" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAttachments(t *testing.T) {
	atts := []InboundAttachment{
		{Type: "image", FilePath: "/ws/weixin/2026-03-26/123.jpg"},
		{Type: "file", FilePath: "/ws/weixin/2026-03-26/456.pdf", FileName: "report.pdf"},
	}
	out := formatAttachments(atts)
	if !strings.Contains(out, "图片: /ws/weixin/2026-03-26/123.jpg") {
		t.Errorf("missing image line in %q", out)
	}
	if !strings.Contains(out, "文件: /ws/weixin/2026-03-26/456.pdf (report.pdf)") {
		t.Errorf("missing file line in %q", out)
	}
}

func TestComposeRunPrompt_RefMediaAndText(t *testing.T) {
	p := &WeixinPlugin{}
	job := &inboundJob{
		Content: "帮我分析这个文件",
		RefAttachments: []InboundAttachment{
			{Type: "file", FilePath: "/ws/weixin/2026-03-26/report.pdf", FileName: "report.pdf"},
		},
	}
	prompt := p.composeRunPrompt(job)
	if !strings.Contains(prompt, "[引用消息]") {
		t.Errorf("missing ref section in prompt")
	}
	if !strings.Contains(prompt, "report.pdf") {
		t.Errorf("missing ref file path in prompt")
	}
	if !strings.Contains(prompt, "[用户消息]") {
		t.Errorf("missing user section in prompt")
	}
	if !strings.Contains(prompt, "帮我分析这个文件") {
		t.Errorf("missing user text in prompt")
	}
}

func TestComposeRunPrompt_RefTextOnly(t *testing.T) {
	p := &WeixinPlugin{}
	job := &inboundJob{
		Content:        "总结一下",
		RefTextContent: "这是一段很长的原始消息内容",
	}
	prompt := p.composeRunPrompt(job)
	if !strings.Contains(prompt, "[引用消息]") {
		t.Errorf("missing ref section")
	}
	if !strings.Contains(prompt, "这是一段很长的原始消息内容") {
		t.Errorf("missing ref text content")
	}
	if !strings.Contains(prompt, "[用户消息]") {
		t.Errorf("missing user section")
	}
}

func TestComposeRunPrompt_NoRef(t *testing.T) {
	p := &WeixinPlugin{}
	job := &inboundJob{Content: "你好"}
	prompt := p.composeRunPrompt(job)
	if strings.Contains(prompt, "[引用消息]") {
		t.Errorf("should not have ref section for plain text")
	}
	if !strings.Contains(prompt, "[用户消息]") {
		t.Errorf("missing user section")
	}
}
