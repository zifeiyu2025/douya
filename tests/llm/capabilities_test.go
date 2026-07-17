package llm_test

import (
	"testing"

	"douya/internal/llm"
)

func TestDeriveModelName_Qwen35U9B(t *testing.T) {
	result := llm.DeriveModelName("Qwen3.5U-9B-Q4_K_M")
	if result != "Qwen3.5U-9B" {
		t.Errorf("DeriveModelName(Qwen3.5U-9B-Q4_K_M) = %q, want %q", result, "Qwen3.5U-9B")
	}
}

func TestDeriveModelName_Gemma4E4BU(t *testing.T) {
	result := llm.DeriveModelName("Gemma-4-E4B-U-Q4_K_M")
	if result != "Gemma-4-E4B-U" {
		t.Errorf("DeriveModelName(Gemma-4-E4B-U-Q4_K_M) = %q, want %q", result, "Gemma-4-E4B-U")
	}
}

func TestDeriveModelName_Llama31_8B(t *testing.T) {
	result := llm.DeriveModelName("Llama-3.1-8B-Q5_K_S")
	if result != "Llama-3.1-8B" {
		t.Errorf("DeriveModelName(Llama-3.1-8B-Q5_K_S) = %q, want %q", result, "Llama-3.1-8B")
	}
}

func TestDeriveModelName_BF16(t *testing.T) {
	result := llm.DeriveModelName("Qwen2.5-72B-BF16")
	if result != "Qwen2.5-72B" {
		t.Errorf("DeriveModelName(Qwen2.5-72B-BF16) = %q, want %q", result, "Qwen2.5-72B")
	}
}

func TestDeriveModelName_NoQuantSuffix(t *testing.T) {
	result := llm.DeriveModelName("Qwen3.5U-9B")
	if result != "Qwen3.5U-9B" {
		t.Errorf("DeriveModelName(Qwen3.5U-9B) = %q, want %q", result, "Qwen3.5U-9B")
	}
}

func TestDeriveModelName_IQ4XS(t *testing.T) {
	result := llm.DeriveModelName("Phi-3-mini-IQ4_XS")
	if result != "Phi-3-mini" {
		t.Errorf("DeriveModelName(Phi-3-mini-IQ4_XS) = %q, want %q", result, "Phi-3-mini")
	}
}

func TestDeriveModelName_Q6K(t *testing.T) {
	result := llm.DeriveModelName("Mistral-7B-Q6_K")
	if result != "Mistral-7B" {
		t.Errorf("DeriveModelName(Mistral-7B-Q6_K) = %q, want %q", result, "Mistral-7B")
	}
}

func TestDeriveModelName_F16(t *testing.T) {
	result := llm.DeriveModelName("DeepSeek-67B-F16")
	if result != "DeepSeek-67B" {
		t.Errorf("DeriveModelName(DeepSeek-67B-F16) = %q, want %q", result, "DeepSeek-67B")
	}
}

func TestDeriveModelName_F32(t *testing.T) {
	result := llm.DeriveModelName("TinyLlama-1.1B-F32")
	if result != "TinyLlama-1.1B" {
		t.Errorf("DeriveModelName(TinyLlama-1.1B-F32) = %q, want %q", result, "TinyLlama-1.1B")
	}
}

func TestDeriveModelName_Q3KL(t *testing.T) {
	result := llm.DeriveModelName("Qwen2.5-7B-Q3_K_L")
	if result != "Qwen2.5-7B" {
		t.Errorf("DeriveModelName(Qwen2.5-7B-Q3_K_L) = %q, want %q", result, "Qwen2.5-7B")
	}
}

func TestDeriveModelName_Q8_0(t *testing.T) {
	result := llm.DeriveModelName("Llama-3.1-8B-Q8_0")
	if result != "Llama-3.1-8B" {
		t.Errorf("DeriveModelName(Llama-3.1-8B-Q8_0) = %q, want %q", result, "Llama-3.1-8B")
	}
}

func TestDeriveModelName_UnderscoreReplacement(t *testing.T) {
	result := llm.DeriveModelName("some_model_name-Q4_K_M")
	if result != "some-model-name" {
		t.Errorf("DeriveModelName(some_model_name-Q4_K_M) = %q, want %q", result, "some-model-name")
	}
}

func TestDeriveModelName_DashU_DashReplacement(t *testing.T) {
	result := llm.DeriveModelName("Model-U-7B-Q4_K_M")
	if result != "Model-7B" {
		t.Errorf("DeriveModelName(Model-U-7B-Q4_K_M) = %q, want %q", result, "Model-7B")
	}
}

func TestStripQuantSuffix_Qwen35U9B(t *testing.T) {
	result := llm.StripQuantSuffix("Qwen3.5U-9B-Q4_K_M")
	if result != "Qwen3.5U-9B" {
		t.Errorf("StripQuantSuffix(Qwen3.5U-9B-Q4_K_M) = %q, want %q", result, "Qwen3.5U-9B")
	}
}

func TestStripQuantSuffix_NoQuantSuffix(t *testing.T) {
	result := llm.StripQuantSuffix("Qwen3.5U-9B")
	if result != "Qwen3.5U-9B" {
		t.Errorf("StripQuantSuffix(Qwen3.5U-9B) = %q, want %q", result, "Qwen3.5U-9B")
	}
}

func TestStripQuantSuffix_BF16(t *testing.T) {
	result := llm.StripQuantSuffix("Qwen2.5-72B-BF16")
	if result != "Qwen2.5-72B" {
		t.Errorf("StripQuantSuffix(Qwen2.5-72B-BF16) = %q, want %q", result, "Qwen2.5-72B")
	}
}

func TestDetectCapabilities_VisionFromServer(t *testing.T) {
	info := llm.ModelInfo{
		Name:         "Qwen3.5U-9B",
		Capabilities: []string{"completion", "chat", "vision"},
	}
	caps := llm.DetectCapabilities(info)
	if !caps.ImageInput {
		t.Error("expected ImageInput=true when server reports 'vision' capability")
	}
	if caps.AudioInput {
		t.Error("expected AudioInput=false when server does not report 'audio' capability")
	}
	if !caps.TextInput {
		t.Error("expected TextInput=true always")
	}
}

func TestDetectCapabilities_AudioFromServer(t *testing.T) {
	info := llm.ModelInfo{
		Name:         "Gemma-4-E4B-U",
		Capabilities: []string{"completion", "chat", "vision", "audio"},
	}
	caps := llm.DetectCapabilities(info)
	if !caps.ImageInput {
		t.Error("expected ImageInput=true when server reports 'vision' capability")
	}
	if !caps.AudioInput {
		t.Error("expected AudioInput=true when server reports 'audio' capability")
	}
}

func TestDetectCapabilities_EmptyCapabilitiesNoFallback(t *testing.T) {
	info := llm.ModelInfo{
		Name:         "Qwen3.5U-9B",
		Capabilities: []string{},
	}
	caps := llm.DetectCapabilities(info)
	if caps.ImageInput {
		t.Error("expected ImageInput=false when capabilities empty and no mmproj fallback")
	}
	if caps.AudioInput {
		t.Error("expected AudioInput=false when capabilities empty and no mmproj fallback")
	}
}

func TestDetectCapabilities_EmptyCapabilitiesWithoutMmproj(t *testing.T) {
	info := llm.ModelInfo{
		Name:         "Llama-3.1-8B",
		Capabilities: []string{},
	}
	caps := llm.DetectCapabilities(info)
	if caps.ImageInput {
		t.Error("expected ImageInput=false when no capabilities and no mmproj")
	}
	if caps.AudioInput {
		t.Error("expected AudioInput=false when no capabilities and no mmproj")
	}
}

func TestDetectCapabilities_NilCapabilitiesWithMmproj(t *testing.T) {
	info := llm.ModelInfo{
		Name:         "Qwen3.5U-9B",
		Capabilities: nil,
	}
	caps := llm.DetectCapabilities(info)
	if caps.ImageInput {
		t.Error("expected ImageInput=false when capabilities nil and no mmproj fallback")
	}
}

func TestDetectCapabilities_NilCapabilitiesWithoutMmproj(t *testing.T) {
	info := llm.ModelInfo{
		Name:         "Llama-3.1-8B",
		Capabilities: nil,
	}
	caps := llm.DetectCapabilities(info)
	if caps.ImageInput {
		t.Error("expected ImageInput=false when no capabilities and no mmproj")
	}
}

func TestDetectCapabilities_ServerCapabilitiesOverrideMmproj(t *testing.T) {
	info := llm.ModelInfo{
		Name:         "Qwen3.5U-9B",
		Capabilities: []string{"completion", "chat", "vision", "audio"},
	}
	caps := llm.DetectCapabilities(info)
	if !caps.ImageInput {
		t.Error("expected ImageInput=true from server capabilities")
	}
	if !caps.AudioInput {
		t.Error("expected AudioInput=true from server capabilities")
	}
}

func TestDetectCapabilities_MultimodalCapability(t *testing.T) {
	info := llm.ModelInfo{
		Name:         "Gemma-4-E4B-U",
		Capabilities: []string{"completion", "multimodal"},
	}
	caps := llm.DetectCapabilities(info)
	if !caps.ImageInput {
		t.Error("expected ImageInput=true when server reports 'multimodal' capability")
	}
	if caps.AudioInput {
		t.Error("expected AudioInput=false when server reports 'multimodal' without explicit 'audio' capability")
	}
	if !caps.TextInput {
		t.Error("expected TextInput=true always")
	}
}

func TestDetectCapabilities_CompletionOnly(t *testing.T) {
	info := llm.ModelInfo{
		Name:         "Llama-3.1-8B",
		Capabilities: []string{"completion"},
	}
	caps := llm.DetectCapabilities(info)
	if caps.ImageInput {
		t.Error("expected ImageInput=false when only 'completion' capability")
	}
	if caps.AudioInput {
		t.Error("expected AudioInput=false when only 'completion' capability")
	}
	if !caps.TextInput {
		t.Error("expected TextInput=true always")
	}
}

func TestDetectCapabilities_MultimodalOnlyEnablesVision(t *testing.T) {
	info := llm.ModelInfo{
		Name:         "Qwen3.5U-9B",
		Capabilities: []string{"completion", "multimodal"},
	}
	caps := llm.DetectCapabilities(info)
	if !caps.ImageInput {
		t.Error("expected ImageInput=true when server reports 'multimodal' capability")
	}
	if caps.AudioInput {
		t.Error("expected AudioInput=false when server reports 'multimodal' without explicit 'audio' capability")
	}
	if !caps.TextInput {
		t.Error("expected TextInput=true always")
	}
}

func TestDetectCapabilities_NoHasMmprojParam(t *testing.T) {
	info := llm.ModelInfo{
		Name:         "TestModel",
		Capabilities: []string{"completion"},
	}
	caps := llm.DetectCapabilities(info)
	if caps.ImageInput {
		t.Error("expected ImageInput=false when no vision/multimodal capability")
	}
	if caps.AudioInput {
		t.Error("expected AudioInput=false when no audio capability")
	}
}

func TestDetectCapabilities_InputModalities(t *testing.T) {
	info := llm.ModelInfo{
		Name:            "Qwen3.5U-9B",
		InputModalities: []string{"text", "image", "audio"},
	}
	caps := llm.DetectCapabilities(info)
	if !caps.ImageInput {
		t.Error("expected ImageInput=true from input_modalities 'image'")
	}
	if !caps.AudioInput {
		t.Error("expected AudioInput=true from input_modalities 'audio'")
	}
}

func TestDetectCapabilities_InputModalitiesVisionOnly(t *testing.T) {
	info := llm.ModelInfo{
		Name:            "Qwen3-VL-7B",
		InputModalities: []string{"text", "image"},
	}
	caps := llm.DetectCapabilities(info)
	if !caps.ImageInput {
		t.Error("expected ImageInput=true from input_modalities 'image'")
	}
	if caps.AudioInput {
		t.Error("expected AudioInput=false, no 'audio' in input_modalities")
	}
}
