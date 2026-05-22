package llm

import (
	"encoding/json"
	"testing"
)

// --- FixUTF8 ---

func TestFixUTF8_ValidString(t *testing.T) {
	input := "hello 世界"
	got := FixUTF8(input)
	if got != input {
		t.Errorf("FixUTF8 valid string: got %q, want %q", got, input)
	}
}

func TestFixUTF8_InvalidSequence(t *testing.T) {
	input := "hello\xffworld"
	got := FixUTF8(input)
	want := "hello\uFFFDworld"
	if got != want {
		t.Errorf("FixUTF8 invalid: got %q, want %q", got, want)
	}
}

func TestFixUTF8_MultipleInvalid(t *testing.T) {
	input := "\xff\xfe\xfd"
	got := FixUTF8(input)
	want := "\uFFFD\uFFFD\uFFFD"
	if got != want {
		t.Errorf("FixUTF8 multiple invalid: got %q, want %q", got, want)
	}
}

func TestFixUTF8_Empty(t *testing.T) {
	got := FixUTF8("")
	if got != "" {
		t.Errorf("FixUTF8 empty: got %q, want empty", got)
	}
}

// --- TruncateIncompleteUTF8 ---

func TestTruncateIncompleteUTF8_Complete(t *testing.T) {
	valid, pending := TruncateIncompleteUTF8("hello")
	if valid != "hello" || pending != "" {
		t.Errorf("got valid=%q pending=%q, want valid=%q pending=%q", valid, pending, "hello", "")
	}
}

func TestTruncateIncompleteUTF8_IncompleteChinese(t *testing.T) {
	// "世" is E4 B8 96, chop last byte
	original := "hello" + string([]byte{0xE4, 0xB8})
	valid, pending := TruncateIncompleteUTF8(original)
	if valid != "hello" {
		t.Errorf("valid: got %q, want %q", valid, "hello")
	}
	if pending != string([]byte{0xE4, 0xB8}) {
		t.Errorf("pending: got %x, want %x", pending, string([]byte{0xE4, 0xB8}))
	}
}

func TestTruncateIncompleteUTF8_Empty(t *testing.T) {
	valid, pending := TruncateIncompleteUTF8("")
	if valid != "" || pending != "" {
		t.Errorf("got valid=%q pending=%q, want empty", valid, pending)
	}
}

func TestTruncateIncompleteUTF8_OnlyIncomplete(t *testing.T) {
	input := string([]byte{0xE4, 0xB8})
	valid, pending := TruncateIncompleteUTF8(input)
	if valid != "" {
		t.Errorf("valid: got %q, want empty", valid)
	}
	if pending != input {
		t.Errorf("pending: got %x, want %x", pending, input)
	}
}

// --- NewClient ---

func TestNewClient_TrailingSlash(t *testing.T) {
	c := NewClient("http://localhost:8080/")
	if c.BaseURL() != "http://localhost:8080" {
		t.Errorf("got %q, want %q", c.BaseURL(), "http://localhost:8080")
	}
}

func TestNewClient_MultipleTrailingSlashes(t *testing.T) {
	c := NewClient("http://localhost:8080///")
	if c.BaseURL() != "http://localhost:8080" {
		t.Errorf("got %q, want %q", c.BaseURL(), "http://localhost:8080")
	}
}

func TestNewClient_NoTrailingSlash(t *testing.T) {
	c := NewClient("http://localhost:8080")
	if c.BaseURL() != "http://localhost:8080" {
		t.Errorf("got %q, want %q", c.BaseURL(), "http://localhost:8080")
	}
}

// --- parseCapabilitiesRaw ---

func TestParseCapabilitiesRaw_StringArray(t *testing.T) {
	raw := json.RawMessage(`["vision","audio"]`)
	got := parseCapabilitiesRaw(raw)
	if len(got) != 2 || got[0] != "vision" || got[1] != "audio" {
		t.Errorf("got %v", got)
	}
}

func TestParseCapabilitiesRaw_ObjectArray(t *testing.T) {
	raw := json.RawMessage(`[{"name":"vision","type":"chat"}]`)
	got := parseCapabilitiesRaw(raw)
	if len(got) != 2 || got[0] != "vision" || got[1] != "chat" {
		t.Errorf("got %v", got)
	}
}

func TestParseCapabilitiesRaw_SingleString(t *testing.T) {
	raw := json.RawMessage(`"vision"`)
	got := parseCapabilitiesRaw(raw)
	if len(got) != 1 || got[0] != "vision" {
		t.Errorf("got %v", got)
	}
}

func TestParseCapabilitiesRaw_Empty(t *testing.T) {
	got := parseCapabilitiesRaw(nil)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestParseCapabilitiesRaw_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`123`)
	got := parseCapabilitiesRaw(raw)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// --- DetectCapabilities ---

func TestDetectCapabilities_Vision(t *testing.T) {
	info := ModelInfo{Capabilities: []string{"vision"}}
	caps := DetectCapabilities(info)
	if !caps.ImageInput || !caps.TextInput {
		t.Errorf("got %+v", caps)
	}
}

func TestDetectCapabilities_Audio(t *testing.T) {
	info := ModelInfo{Capabilities: []string{"audio"}}
	caps := DetectCapabilities(info)
	if !caps.AudioInput {
		t.Errorf("got %+v", caps)
	}
}

func TestDetectCapabilities_Speech(t *testing.T) {
	info := ModelInfo{Capabilities: []string{"speech"}}
	caps := DetectCapabilities(info)
	if !caps.AudioInput {
		t.Errorf("got %+v", caps)
	}
}

func TestDetectCapabilities_Multimodal(t *testing.T) {
	info := ModelInfo{Capabilities: []string{"multimodal"}}
	caps := DetectCapabilities(info)
	if !caps.ImageInput {
		t.Errorf("got %+v", caps)
	}
}

func TestDetectCapabilities_InputModalities(t *testing.T) {
	info := ModelInfo{InputModalities: []string{"image", "audio"}}
	caps := DetectCapabilities(info)
	if !caps.ImageInput || !caps.AudioInput {
		t.Errorf("got %+v", caps)
	}
}

func TestDetectCapabilities_Empty(t *testing.T) {
	caps := DetectCapabilities(ModelInfo{})
	if !caps.TextInput || caps.ImageInput || caps.AudioInput {
		t.Errorf("got %+v", caps)
	}
}

func TestDetectCapabilities_CaseInsensitive(t *testing.T) {
	info := ModelInfo{Capabilities: []string{"Vision", "AUDIO"}}
	caps := DetectCapabilities(info)
	if !caps.ImageInput || !caps.AudioInput {
		t.Errorf("got %+v", caps)
	}
}

// --- ChatMessage.ContentString ---

func TestContentString_PlainString(t *testing.T) {
	m := ChatMessage{Role: "user", Content: "hello"}
	if got := m.ContentString(); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestContentString_ContentParts(t *testing.T) {
	m := ChatMessage{Role: "user", Content: []ContentPart{
		{Type: "text", Text: "describe this"},
		{Type: "image_url", ImageURL: &ImageURL{URL: "http://img"}},
	}}
	if got := m.ContentString(); got != "describe this" {
		t.Errorf("got %q, want %q", got, "describe this")
	}
}

func TestContentString_InterfaceSlice(t *testing.T) {
	m := ChatMessage{Role: "user", Content: []interface{}{
		map[string]interface{}{"type": "text", "text": "hello"},
	}}
	if got := m.ContentString(); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestContentString_Empty(t *testing.T) {
	m := ChatMessage{Role: "user", Content: nil}
	if got := m.ContentString(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// --- NewTextMessage / NewVisionMessage / NewAudioMessage ---

func TestNewTextMessage(t *testing.T) {
	m := NewTextMessage("user", "hi")
	if m.Role != "user" || m.Content != "hi" {
		t.Errorf("got %+v", m)
	}
}

func TestNewVisionMessage(t *testing.T) {
	m := NewVisionMessage("user", "look", []string{"http://img"})
	if m.Role != "user" {
		t.Errorf("role: got %q", m.Role)
	}
	parts, ok := m.Content.([]ContentPart)
	if !ok {
		t.Fatal("content is not []ContentPart")
	}
	if len(parts) != 2 {
		t.Fatalf("got %d parts, want 2", len(parts))
	}
	if parts[0].Type != "text" || parts[0].Text != "look" {
		t.Errorf("text part: %+v", parts[0])
	}
	if parts[1].Type != "image_url" || parts[1].ImageURL.URL != "http://img" {
		t.Errorf("image part: %+v", parts[1])
	}
}

func TestNewVisionMessage_EmptyText(t *testing.T) {
	m := NewVisionMessage("user", "", []string{"http://img"})
	parts := m.Content.([]ContentPart)
	if parts[0].Text != "." {
		t.Errorf("got %q, want '.'", parts[0].Text)
	}
}

func TestNewAudioMessage(t *testing.T) {
	m := NewAudioMessage("user", "listen", []InputAudio{{Data: "base64", Format: "wav"}})
	parts, ok := m.Content.([]ContentPart)
	if !ok {
		t.Fatal("content is not []ContentPart")
	}
	if len(parts) != 2 {
		t.Fatalf("got %d parts, want 2", len(parts))
	}
	if parts[1].Type != "input_audio" || parts[1].InputAudio.Format != "wav" {
		t.Errorf("audio part: %+v", parts[1])
	}
}

func TestNewAudioMessage_EmptyText(t *testing.T) {
	m := NewAudioMessage("user", "", []InputAudio{{Data: "x", Format: "mp3"}})
	parts := m.Content.([]ContentPart)
	if parts[0].Text != "." {
		t.Errorf("got %q, want '.'", parts[0].Text)
	}
}
