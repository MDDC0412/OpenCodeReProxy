package protocol

import (
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- stop reasons

func TestChatStopMapping(t *testing.T) {
	cases := map[string]string{
		"tool_use":       "tool_calls",
		"tool_calls":     "tool_calls",
		"max_tokens":     "length",
		"length":         "length",
		"content_filter": "content_filter",
		"error":          "error",
		"stop":           "stop",
		"stop_sequence":  "stop", // not a Chat reason: falls back
		"":               "stop",
		"refusal":        "stop",
	}
	for input, want := range cases {
		if got := chatStop(input); got != want {
			t.Errorf("chatStop(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAnthropicStopMapping(t *testing.T) {
	cases := map[string]string{
		"tool_use":       "tool_use",
		"tool_calls":     "tool_use",
		"max_tokens":     "max_tokens",
		"length":         "max_tokens",
		"stop_sequence":  "stop_sequence",
		"pause_turn":     "pause_turn",
		"refusal":        "refusal",
		"content_filter": "end_turn", // not an Anthropic reason
		"error":          "end_turn",
		"stop":           "end_turn",
		"":               "end_turn",
	}
	for input, want := range cases {
		if got := anthropicStop(input); got != want {
			t.Errorf("anthropicStop(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCanonicalChatStopMapping(t *testing.T) {
	cases := map[string]string{
		"tool_calls":     "tool_calls",
		"function_call":  "tool_calls",
		"tool_use":       "stop", // Chat canonicalizer has no tool_use arm
		"length":         "length",
		"content_filter": "content_filter",
		"error":          "error",
		"network_error":  "error",
		"server_error":   "error",
		"stop":           "stop",
		"end_turn":       "stop",
		"":               "stop",
	}
	for input, want := range cases {
		if got := canonicalChatStop(input); got != want {
			t.Errorf("canonicalChatStop(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCanonicalAnthropicStopMapping(t *testing.T) {
	cases := map[string]string{
		"tool_use":      "tool_calls", // canonical name is the Chat one
		"tool_calls":    "stop",       // symmetric surprise in the other direction
		"max_tokens":    "length",
		"stop_sequence": "stop_sequence",
		"pause_turn":    "pause_turn",
		"refusal":       "refusal",
		"error":         "error",
		"network_error": "error",
		"server_error":  "error",
		"end_turn":      "stop",
		"":              "stop",
	}
	for input, want := range cases {
		if got := canonicalAnthropicStop(input); got != want {
			t.Errorf("canonicalAnthropicStop(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestIsToolStop(t *testing.T) {
	for _, stop := range []string{"tool_calls", "tool_use", "function_call"} {
		if !isToolStop(stop) {
			t.Errorf("isToolStop(%q) = false, want true", stop)
		}
	}
	for _, stop := range []string{"stop", "length", "content_filter", "error", ""} {
		if isToolStop(stop) {
			t.Errorf("isToolStop(%q) = true, want false", stop)
		}
	}
}

func TestResponsesIncompleteMapping(t *testing.T) {
	// Every reason except content_filter collapses onto "length".
	for _, reason := range []string{"max_output_tokens", "tool_calls", "", "content_filter_x"} {
		if got := canonicalResponsesIncomplete(reason); got != "length" {
			t.Errorf("canonicalResponsesIncomplete(%q) = %q, want length", reason, got)
		}
	}
	if got := canonicalResponsesIncomplete("content_filter"); got != "content_filter" {
		t.Errorf("canonicalResponsesIncomplete(content_filter) = %q", got)
	}
	if got := responsesIncompleteReason("content_filter"); got != "content_filter" {
		t.Errorf("responsesIncompleteReason(content_filter) = %q", got)
	}
	for _, stop := range []string{"length", "stop", "error", ""} {
		if got := responsesIncompleteReason(stop); got != "max_output_tokens" {
			t.Errorf("responsesIncompleteReason(%q) = %q, want max_output_tokens", stop, got)
		}
	}
}

// --------------------------------------------------------------------- usage

func TestDecodeOpenAIUsage(t *testing.T) {
	cases := []struct {
		name  string
		input map[string]any
		want  Usage
	}{
		{
			name: "chat style with prompt details",
			input: map[string]any{
				"prompt_tokens": 100, "completion_tokens": 20,
				"prompt_tokens_details":     map[string]any{"cached_tokens": 30},
				"completion_tokens_details": map[string]any{"reasoning_tokens": 8},
			},
			// total_tokens is absent, so it is derived.
			want: Usage{Input: 100, Output: 20, Total: 120, Cached: 30, Reasoning: 8},
		},
		{
			name: "responses style with input details",
			input: map[string]any{
				"input_tokens": 5, "output_tokens": 2,
				"input_tokens_details":  map[string]any{"cached_tokens": 1, "cache_creation_input_tokens": 3},
				"output_tokens_details": map[string]any{"reasoning_tokens": 4},
			},
			want: Usage{Input: 5, Output: 2, Total: 7, Cached: 1, CacheCreation: 3, Reasoning: 4},
		},
		{
			name: "cache write tokens under prompt details",
			input: map[string]any{
				"prompt_tokens": 10, "completion_tokens": 1,
				"total_tokens":          99, // honored when non-zero
				"prompt_tokens_details": map[string]any{"cache_write_tokens": 6},
			},
			want: Usage{Input: 10, Output: 1, Total: 99, CacheCreation: 6},
		},
		{
			name:  "empty usage",
			input: map[string]any{},
			want:  Usage{},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := decodeOpenAIUsage(test.input); got != test.want {
				t.Errorf("decodeOpenAIUsage = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestDecodeAnthropicUsage(t *testing.T) {
	// Anthropic reports uncached input separately, so the bridge Input is the
	// sum: this is what makes the Chat prompt_tokens total comparable.
	got := decodeAnthropicUsage(map[string]any{
		"input_tokens": 50, "output_tokens": 20,
		"cache_read_input_tokens": 30, "cache_creation_input_tokens": 20,
	})
	want := Usage{Input: 100, Output: 20, Total: 120, Cached: 30, CacheCreation: 20}
	if got != want {
		t.Fatalf("decodeAnthropicUsage = %+v, want %+v", got, want)
	}
	// Anthropic has no reasoning token field: Reasoning stays zero.
	if got.Reasoning != 0 {
		t.Errorf("Reasoning = %d, want 0", got.Reasoning)
	}
}

func TestOpenAIUsageEncoding(t *testing.T) {
	got := openAIUsage(Usage{Input: 100, Output: 20, Total: 120, Cached: 30, CacheCreation: 10, Reasoning: 8})
	want := map[string]any{
		"prompt_tokens": 100, "completion_tokens": 20, "total_tokens": 120,
		"prompt_tokens_details":     map[string]any{"cached_tokens": 30},
		"completion_tokens_details": map[string]any{"reasoning_tokens": 8},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("openAIUsage = %#v, want %#v", got, want)
	}
	// Cache creation has no OpenAI field: it is silently dropped rather than
	// folded into cached_tokens.
	if _, exists := got["prompt_tokens_details"].(map[string]any)["cache_creation_input_tokens"]; exists {
		t.Error("cache creation leaked into the OpenAI usage shape")
	}
}

func TestAnthropicUsageEncoding(t *testing.T) {
	got := anthropicUsage(Usage{Input: 100, Output: 20, Total: 120, Cached: 30, CacheCreation: 10, Reasoning: 8})
	want := map[string]any{
		"input_tokens": 60, "output_tokens": 20,
		"cache_read_input_tokens": 30, "cache_creation_input_tokens": 10,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("anthropicUsage = %#v, want %#v", got, want)
	}
	// input_tokens excludes the cache fields, so it must never go negative.
	clamped := anthropicUsage(Usage{Input: 5, Cached: 30, CacheCreation: 10})
	if clamped["input_tokens"] != 0 {
		t.Errorf("input_tokens = %v, want 0 (clamped)", clamped["input_tokens"])
	}
}

func TestUsageRoundTripAcrossStyles(t *testing.T) {
	original := Usage{Input: 100, Output: 20, Total: 120, Cached: 30, CacheCreation: 10}
	viaAnthropic := decodeAnthropicUsage(anthropicUsage(original))
	// Total is recomputed as input+output on the Anthropic side.
	if viaAnthropic != (Usage{Input: 100, Output: 20, Total: 120, Cached: 30, CacheCreation: 10}) {
		t.Errorf("Anthropic round trip = %+v", viaAnthropic)
	}
	viaOpenAI := decodeOpenAIUsage(openAIUsage(original))
	// Reasoning and cache creation are not representable in the OpenAI shape.
	if viaOpenAI.Input != 100 || viaOpenAI.Output != 20 || viaOpenAI.Cached != 30 {
		t.Errorf("OpenAI round trip lost fields: %+v", viaOpenAI)
	}
	if viaOpenAI.Reasoning != 0 || viaOpenAI.CacheCreation != 0 {
		t.Errorf("OpenAI round trip invented fields: %+v", viaOpenAI)
	}
}

// ---------------------------------------------------------------- tool choice

func TestChatToolChoiceRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		wire any
		want bridgeToolChoice
	}{
		{"auto", "auto", bridgeToolChoice{Mode: "auto"}},
		{"none", "none", bridgeToolChoice{Mode: "none"}},
		{"required", "required", bridgeToolChoice{Mode: "required"}},
		{"named", map[string]any{"type": "function", "function": map[string]any{"name": "lookup"}},
			bridgeToolChoice{Mode: "named", Name: "lookup"}},
		{"absent", nil, bridgeToolChoice{}},
		{"unknown object", map[string]any{"type": "custom"}, bridgeToolChoice{}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := decodeChatToolChoice(test.wire)
			if got != test.want {
				t.Fatalf("decodeChatToolChoice(%#v) = %+v, want %+v", test.wire, got, test.want)
			}
			encoded := encodeChatToolChoice(got)
			if got.Mode == "" {
				// An unrecognized shape decodes to the zero choice, which then
				// encodes to nothing rather than round-tripping.
				if encoded != nil {
					t.Errorf("encodeChatToolChoice(empty) = %#v, want nil", encoded)
				}
				return
			}
			if !reflect.DeepEqual(encoded, test.wire) {
				t.Errorf("encodeChatToolChoice = %#v, want %#v", encoded, test.wire)
			}
		})
	}
}

func TestResponsesToolChoiceRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		wire any
		want bridgeToolChoice
	}{
		{"auto", "auto", bridgeToolChoice{Mode: "auto"}},
		{"required", "required", bridgeToolChoice{Mode: "required"}},
		// Responses names the function at the top level, not under "function".
		{"named", map[string]any{"type": "function", "name": "lookup"},
			bridgeToolChoice{Mode: "named", Name: "lookup"}},
		{"absent", nil, bridgeToolChoice{}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := decodeResponsesToolChoice(test.wire)
			if got != test.want {
				t.Fatalf("decodeResponsesToolChoice(%#v) = %+v, want %+v", test.wire, got, test.want)
			}
			if encoded := encodeResponsesToolChoice(got); !reflect.DeepEqual(encoded, test.wire) {
				t.Errorf("encodeResponsesToolChoice = %#v, want %#v", encoded, test.wire)
			}
		})
	}
}

func TestAnthropicToolChoiceRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		wire any
		want bridgeToolChoice
	}{
		{"auto", map[string]any{"type": "auto"}, bridgeToolChoice{Mode: "auto"}},
		{"any", map[string]any{"type": "any"}, bridgeToolChoice{Mode: "required"}},
		{"tool", map[string]any{"type": "tool", "name": "lookup"},
			bridgeToolChoice{Mode: "named", Name: "lookup"}},
		// The string forms are folded through the same table.
		{"string auto", "auto", bridgeToolChoice{Mode: "auto"}},
		{"string required", "required", bridgeToolChoice{Mode: "required"}},
		{"string any", "any", bridgeToolChoice{Mode: "required"}},
		{"none", map[string]any{"type": "none"}, bridgeToolChoice{Mode: "none"}},
		{"absent", nil, bridgeToolChoice{}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got := decodeAnthropicToolChoice(test.wire)
			if got != test.want {
				t.Fatalf("decodeAnthropicToolChoice(%#v) = %+v, want %+v", test.wire, got, test.want)
			}
		})
	}
}

func TestEncodeAnthropicToolChoice(t *testing.T) {
	cases := []struct {
		choice bridgeToolChoice
		want   any
	}{
		{bridgeToolChoice{Mode: "named", Name: "lookup"}, map[string]any{"type": "tool", "name": "lookup"}},
		{bridgeToolChoice{Mode: "required"}, map[string]any{"type": "any"}},
		{bridgeToolChoice{Mode: "auto"}, map[string]any{"type": "auto"}},
		// Anthropic has no "none" value: the mode encodes to nothing.
		{bridgeToolChoice{Mode: "none"}, nil},
		{bridgeToolChoice{}, nil},
	}
	for _, test := range cases {
		if got := encodeAnthropicToolChoice(test.choice); !reflect.DeepEqual(got, test.want) {
			t.Errorf("encodeAnthropicToolChoice(%+v) = %#v, want %#v", test.choice, got, test.want)
		}
	}
}

// ------------------------------------------------------------ block codecs

func TestDecodeOpenAIBlocks(t *testing.T) {
	blocks, err := decodeOpenAIBlocksChecked("hello")
	if err != nil || len(blocks) != 1 || blocks[0].Kind != "text" || blocks[0].Text != "hello" {
		t.Fatalf("string content = %+v, err %v", blocks, err)
	}
	if empty, err := decodeOpenAIBlocksChecked(""); err != nil || len(empty) != 0 {
		t.Fatalf("empty string = %+v, err %v", empty, err)
	}
	if none, err := decodeOpenAIBlocksChecked(nil); err != nil || len(none) != 0 {
		t.Fatalf("nil = %+v, err %v", none, err)
	}
	// A bare object is treated as a one-part array.
	single, err := decodeOpenAIBlocksChecked(map[string]any{"type": "input_text", "text": "hi"})
	if err != nil || len(single) != 1 || single[0].Text != "hi" {
		t.Fatalf("object content = %+v, err %v", single, err)
	}
	parts, err := decodeOpenAIBlocksChecked([]any{
		map[string]any{"type": "text", "text": "a"},
		map[string]any{"type": "output_text", "text": "b"},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://x/y.png"}},
		map[string]any{"type": "input_image", "image_url": "https://x/z.png"},
		map[string]any{"type": "file", "file": map[string]any{"file_id": "f1", "filename": "n.txt", "media_type": "text/plain", "file_data": "abc"}},
	})
	if err != nil {
		t.Fatalf("array content: %v", err)
	}
	want := []bridgeBlock{
		{Kind: "text", Text: "a"},
		{Kind: "text", Text: "b"},
		{Kind: "image", URL: "https://x/y.png"},
		{Kind: "image", URL: "https://x/z.png"},
		{Kind: "file", FileID: "f1", Filename: "n.txt", MediaType: "text/plain", Data: "abc"},
	}
	if !reflect.DeepEqual(parts, want) {
		t.Fatalf("parts = %+v, want %+v", parts, want)
	}
	if _, err := decodeOpenAIBlocksChecked([]any{map[string]any{"type": "video"}}); err == nil {
		t.Error("unsupported block type accepted")
	}
	if _, err := decodeOpenAIBlocksChecked([]any{"nope"}); err == nil {
		t.Error("non-object part accepted")
	}
	if _, err := decodeOpenAIBlocksChecked(true); err == nil {
		t.Error("bool content accepted")
	}
}

func TestDecodeAnthropicBlocks(t *testing.T) {
	blocks, err := decodeAnthropicBlocksChecked("hello")
	if err != nil || len(blocks) != 1 || blocks[0].Kind != "text" {
		t.Fatalf("string content = %+v, err %v", blocks, err)
	}
	parts, err := decodeAnthropicBlocksChecked([]any{
		map[string]any{"type": "thinking", "thinking": "why", "signature": "sig"},
		map[string]any{"type": "redacted_thinking", "data": "enc"},
		map[string]any{"type": "text", "text": "answer"},
		map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": "https://x/i.png"}},
		map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "AAA"}},
		map[string]any{"type": "document", "source": map[string]any{"type": "text", "data": "doc body"}},
		map[string]any{"type": "document", "title": "f.pdf", "source": map[string]any{"type": "base64", "media_type": "application/pdf", "data": "BBB"}},
		map[string]any{"type": "tool_use", "id": "tu1", "name": "lookup", "input": map[string]any{"q": "x"}},
		map[string]any{"type": "tool_result", "tool_use_id": "tu1", "content": "42", "is_error": true},
	})
	if err != nil {
		t.Fatalf("array content: %v", err)
	}
	want := []bridgeBlock{
		{Kind: "reasoning", Text: "why", Signature: "sig"},
		{Kind: "reasoning", Encrypted: "enc"},
		{Kind: "text", Text: "answer"},
		{Kind: "image", URL: "https://x/i.png"},
		{Kind: "image", MediaType: "image/png", Data: "AAA"},
		// A text-sourced document degrades to plain text.
		{Kind: "text", Text: "doc body"},
		{Kind: "file", MediaType: "application/pdf", Data: "BBB", Filename: "f.pdf"},
		{Kind: "tool_call", ID: "tu1", Name: "lookup", Arguments: map[string]any{"q": "x"}},
		{Kind: "tool_result", CallID: "tu1", Result: "42", IsError: true},
	}
	if !reflect.DeepEqual(parts, want) {
		t.Fatalf("parts = %+v, want %+v", parts, want)
	}
	if _, err := decodeAnthropicBlocksChecked([]any{map[string]any{"type": "audio"}}); err == nil {
		t.Error("unsupported block type accepted")
	}
	if _, err := decodeAnthropicBlocksChecked([]any{"nope"}); err == nil {
		t.Error("non-object part accepted")
	}
}

func TestEncodeChatBlocks(t *testing.T) {
	// A lone text block collapses to the plain string form.
	if got := encodeChatBlocks([]bridgeBlock{{Kind: "text", Text: "hi"}}); got != any("hi") {
		t.Errorf("single text = %#v, want \"hi\"", got)
	}
	got := encodeChatBlocks([]bridgeBlock{
		{Kind: "text", Text: "a"},
		{Kind: "image", URL: "https://x/i.png"},
		{Kind: "image", MediaType: "image/png", Data: "AAA"},
		{Kind: "file", FileID: "f1"}, // empty file_data/filename are kept as empty strings
		// Reasoning and tool blocks have no Chat content representation here.
		{Kind: "reasoning", Text: "hidden"},
		{Kind: "tool_call", ID: "c1", Name: "n"},
	})
	want := []any{
		map[string]any{"type": "text", "text": "a"},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://x/i.png"}},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,AAA"}},
		map[string]any{"type": "file", "file": map[string]any{"file_id": "f1", "file_data": "", "filename": ""}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("encodeChatBlocks = %#v, want %#v", got, want)
	}
}

func TestEncodeAnthropicBlocks(t *testing.T) {
	got := encodeAnthropicBlocks([]bridgeBlock{
		{Kind: "reasoning", Text: "why", Signature: "sig"},
		{Kind: "reasoning", Encrypted: "enc"},
		{Kind: "text", Text: "answer"},
		{Kind: "image", URL: "https://x/i.png"},
		{Kind: "image", MediaType: "image/png", Data: "AAA"},
		{Kind: "file", Filename: "orphan.bin"}, // no URL and no data: dropped
		{Kind: "file", URL: "https://x/f.pdf", Filename: "f.pdf"},
		{Kind: "tool_call", ID: "tu1", Name: "lookup", ArgumentsJSON: `{"q":"x"}`},
		{Kind: "tool_call", ID: "tu2", Name: "raw", ArgumentsJSON: "not json"},
		{Kind: "tool_result", CallID: "tu1", Result: "42", IsError: true},
	})
	want := []any{
		map[string]any{"type": "thinking", "thinking": "why", "signature": "sig"},
		map[string]any{"type": "redacted_thinking", "data": "enc"},
		map[string]any{"type": "text", "text": "answer"},
		map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": "https://x/i.png"}},
		map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "AAA"}},
		map[string]any{"type": "document", "source": map[string]any{"type": "url", "url": "https://x/f.pdf"}, "title": "f.pdf"},
		map[string]any{"type": "tool_use", "id": "tu1", "name": "lookup", "input": map[string]any{"q": "x"}},
		map[string]any{"type": "tool_use", "id": "tu2", "name": "raw", "input": "not json"},
		map[string]any{"type": "tool_result", "tool_use_id": "tu1", "content": "42", "is_error": true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("encodeAnthropicBlocks = %#v, want %#v", got, want)
	}
}

func TestBridgeReasoningText(t *testing.T) {
	if text, ok := bridgeReasoningText(nil); ok || text != "" {
		t.Errorf("empty blocks = (%q, %v)", text, ok)
	}
	// Non-empty but blank still reports ok: the field is present upstream.
	if text, ok := bridgeReasoningText([]bridgeBlock{{Kind: "reasoning"}}); !ok || text != "" {
		t.Errorf("blank block = (%q, %v)", text, ok)
	}
	text, ok := bridgeReasoningText([]bridgeBlock{
		{Kind: "reasoning", Text: "a"},
		{Kind: "reasoning"},
		{Kind: "reasoning", Text: "b"},
	})
	if !ok || text != "ab" {
		t.Errorf("concatenation = (%q, %v)", text, ok)
	}
	// Encrypted reasoning has no readable text and gets the placeholder.
	text, ok = bridgeReasoningText([]bridgeBlock{{Kind: "reasoning", Encrypted: "enc"}})
	if !ok || text != anthropicRedactedThinkingPlaceholder {
		t.Errorf("encrypted = (%q, %v)", text, ok)
	}
}

func TestBridgeReasoningSignature(t *testing.T) {
	if got := bridgeReasoningSignature(nil); got != "" {
		t.Errorf("nil = %q", got)
	}
	if got := bridgeReasoningSignature([]bridgeBlock{{Kind: "reasoning", Text: "x"}}); got != "" {
		t.Errorf("unsigned = %q", got)
	}
	// The last signed block wins.
	got := bridgeReasoningSignature([]bridgeBlock{
		{Kind: "reasoning", Signature: "first"},
		{Kind: "reasoning"},
		{Kind: "reasoning", Signature: "last"},
	})
	if got != "last" {
		t.Errorf("signature = %q, want last", got)
	}
}

// ------------------------------------------------------------------- effort

func TestClientEffortExplicit(t *testing.T) {
	cases := []struct {
		protocol Protocol
		body     map[string]any
		want     bool
	}{
		{Chat, map[string]any{"reasoning_effort": "high"}, true},
		{Chat, map[string]any{"reasoning": map[string]any{"effort": "high"}}, false},
		{Chat, map[string]any{"reasoning_effort": 3}, false},
		{Chat, map[string]any{}, false},
		{Anthropic, map[string]any{"output_config": map[string]any{"effort": "max"}}, true},
		{Anthropic, map[string]any{"effort": "max"}, true},
		{Anthropic, map[string]any{"output_config": map[string]any{"effort": "  "}}, false},
		{Anthropic, map[string]any{"thinking": map[string]any{"type": "enabled"}}, false},
		{Responses, map[string]any{"reasoning": map[string]any{"effort": "low"}}, true},
		{Responses, map[string]any{"reasoning": map[string]any{"type": "enabled"}}, false},
		{Responses, map[string]any{}, false},
		{SystemOne, map[string]any{"effort": "max"}, false},
	}
	for _, test := range cases {
		if got := clientEffortExplicit(test.protocol, test.body); got != test.want {
			t.Errorf("clientEffortExplicit(%s, %#v) = %v, want %v", test.protocol, test.body, got, test.want)
		}
	}
}

func TestValidForcedEffort(t *testing.T) {
	for _, effort := range []string{"minimal", "low", "medium", "high", "xhigh", "max", "none"} {
		if !validForcedEffort(effort) {
			t.Errorf("validForcedEffort(%q) = false", effort)
		}
	}
	// The value is trimmed and lowercased by the caller, not here.
	for _, effort := range []string{"", "HIGH", "auto", "  ", "off"} {
		if validForcedEffort(effort) {
			t.Errorf("validForcedEffort(%q) = true", effort)
		}
	}
}

func TestApplyAnthropicForcedEffort(t *testing.T) {
	body := map[string]any{
		"max_tokens":    100,
		"output_config": map[string]any{"other": 1},
		"thinking":      map[string]any{"type": "disabled"},
	}
	applyAnthropicForcedEffort(body, "max", false)
	thinking := body["thinking"].(map[string]any)
	if thinking["type"] != "enabled" || thinking["budget_tokens"] != float64(32768) {
		t.Errorf("thinking = %#v", thinking)
	}
	if body["output_config"].(map[string]any)["effort"] != "max" {
		t.Errorf("output_config = %#v", body["output_config"])
	}
	// Unrelated output_config keys survive.
	if body["output_config"].(map[string]any)["other"] != 1 {
		t.Errorf("output_config lost keys: %#v", body["output_config"])
	}
	// max_tokens must stay strictly above the budget.
	if body["max_tokens"] != 32768+4096 {
		t.Errorf("max_tokens = %v, want %d", body["max_tokens"], 32768+4096)
	}

	// A budget already above max_tokens leaves max_tokens alone.
	roomy := map[string]any{"max_tokens": 100000}
	applyAnthropicForcedEffort(roomy, "low", false)
	if roomy["max_tokens"] != 100000 {
		t.Errorf("max_tokens = %v, want 100000", roomy["max_tokens"])
	}
	if roomy["thinking"].(map[string]any)["budget_tokens"] != float64(1024) {
		t.Errorf("thinking = %#v", roomy["thinking"])
	}

	// Disabling removes both reasoning fields and leaves the rest alone.
	disabled := map[string]any{
		"thinking":      map[string]any{"type": "enabled"},
		"output_config": map[string]any{"effort": "max"},
		"max_tokens":    16,
	}
	applyAnthropicForcedEffort(disabled, "none", true)
	if _, exists := disabled["thinking"]; exists {
		t.Error("thinking survived disable")
	}
	if _, exists := disabled["output_config"]; exists {
		t.Error("output_config survived disable")
	}
	if disabled["max_tokens"] != 16 {
		t.Errorf("max_tokens = %v, want 16", disabled["max_tokens"])
	}
}

func TestForcedEffort(t *testing.T) {
	// Chat: a level is written only when the client stated none.
	chat := map[string]any{"model": "m"}
	ForcedEffort(Chat, chat, "HIGH")
	if chat["reasoning_effort"] != "high" {
		t.Errorf("chat = %#v", chat)
	}
	explicit := map[string]any{"reasoning_effort": "low"}
	ForcedEffort(Chat, explicit, "max")
	if explicit["reasoning_effort"] != "low" {
		t.Errorf("client effort overwritten: %#v", explicit)
	}
	// The explicit-effort check runs before the disable branch, so an explicit
	// client level is preserved even when the operator forces "none".
	off := map[string]any{"reasoning_effort": "high", "model": "m"}
	ForcedEffort(Chat, off, "none")
	if off["reasoning_effort"] != "high" {
		t.Errorf("explicit effort overwritten by none: %#v", off)
	}
	// With no stated level, "none" just removes the field.
	clear := map[string]any{"reasoning": map[string]any{"type": "enabled"}, "model": "m"}
	ForcedEffort(Chat, clear, "none")
	if _, exists := clear["reasoning_effort"]; exists {
		t.Errorf("none invented reasoning_effort: %#v", clear)
	}

	// Responses writes reasoning.effort, creating the object when needed.
	responses := map[string]any{"model": "m"}
	ForcedEffort(Responses, responses, "max")
	if responses["reasoning"].(map[string]any)["effort"] != "max" {
		t.Errorf("responses = %#v", responses)
	}
	existing := map[string]any{"reasoning": map[string]any{"summary": "auto"}}
	ForcedEffort(Responses, existing, "low")
	reasoning := existing["reasoning"].(map[string]any)
	if reasoning["effort"] != "low" || reasoning["summary"] != "auto" {
		t.Errorf("responses reasoning = %#v", reasoning)
	}

	// An invalid level is a no-op on every protocol.
	invalid := map[string]any{"model": "m"}
	ForcedEffort(Chat, invalid, "turbo")
	ForcedEffort(Anthropic, invalid, "turbo")
	ForcedEffort(Responses, invalid, "turbo")
	if !reflect.DeepEqual(invalid, map[string]any{"model": "m"}) {
		t.Errorf("invalid effort mutated the body: %#v", invalid)
	}
	ForcedEffort(Chat, nil, "max")
}

func TestIsReasoningVendorIdentifier(t *testing.T) {
	for _, value := range []string{"deepseek-chat", "KIMI-k2", "moonshot-v1", "XiaomiMiMo", "https://api.mimo.ai/v1"} {
		if !isReasoningVendorIdentifier(value) {
			t.Errorf("isReasoningVendorIdentifier(%q) = false", value)
		}
	}
	for _, value := range []string{"", "gpt-5", "claude-sonnet-4", "glm-4"} {
		if isReasoningVendorIdentifier(value) {
			t.Errorf("isReasoningVendorIdentifier(%q) = true", value)
		}
	}
}

func TestRequestEnablesReasoning(t *testing.T) {
	cases := []struct {
		name  string
		input map[string]any
		want  bool
	}{
		{"effort string", map[string]any{"reasoning_effort": "high"}, true},
		{"effort none", map[string]any{"reasoning_effort": "none"}, false},
		{"effort disabled", map[string]any{"reasoning_effort": "disabled"}, false},
		{"blank string", map[string]any{"reasoning_effort": ""}, false},
		{"thinking block", map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": 2048}}, true},
		{"thinking adaptive", map[string]any{"thinking": map[string]any{"type": "adaptive"}}, true},
		{"thinking budget only", map[string]any{"thinking": map[string]any{"budget_tokens": 2048}}, true},
		{"reasoning disabled object", map[string]any{"reasoning": map[string]any{"type": "disabled"}}, false},
		{"reasoning effort none", map[string]any{"reasoning": map[string]any{"effort": "none"}}, false},
		{"reasoning effort high", map[string]any{"reasoning": map[string]any{"effort": "high"}}, true},
		{"bool true", map[string]any{"effort": true}, true},
		{"bool false", map[string]any{"effort": false}, false},
		{"number", map[string]any{"effort": 42}, true},
		{"nil value", map[string]any{"reasoning": nil}, false},
		{"empty", map[string]any{}, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := requestEnablesReasoning(test.input); got != test.want {
				t.Errorf("requestEnablesReasoning(%#v) = %v, want %v", test.input, got, test.want)
			}
		})
	}
}

func TestMissingToolIDs(t *testing.T) {
	if got := missingToolIDs([]string{"b", "a"}, map[string]bool{"b": true, "a": true}); !reflect.DeepEqual(got, []string{"b", "a"}) {
		t.Errorf("all pending in order = %#v", got)
	}
	// An id missing from the order is appended and the list is sorted.
	got := missingToolIDs([]string{"b"}, map[string]bool{"b": true, "a": true, "c": true})
	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("partial order = %#v", got)
	}
	if got := missingToolIDs(nil, nil); len(got) != 0 {
		t.Errorf("nothing pending = %#v", got)
	}
	// Resolved ids are dropped.
	if got := missingToolIDs([]string{"a", "b"}, map[string]bool{"b": true}); !reflect.DeepEqual(got, []string{"b"}) {
		t.Errorf("resolved dropped wrong = %#v", got)
	}
}

// ------------------------------------------------------- request decoding

func TestDecodeBridgeRequestChat(t *testing.T) {
	input := map[string]any{
		"model":  "gpt-5",
		"stream": true,
		"messages": []any{
			map[string]any{"role": "system", "content": "be brief"},
			map[string]any{"role": "developer", "content": "use tools"},
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{
				"role": "assistant", "content": "checking", "reasoning_content": "think",
				"reasoning_budget_tokens": 32000,
				"tool_calls": []any{
					map[string]any{"id": "call_1", "type": "function", "function": map[string]any{"name": "lookup", "arguments": `{"q":"x"}`}},
				},
			},
			map[string]any{"role": "tool", "tool_call_id": "call_1", "content": "42"},
		},
		"tools": []any{
			map[string]any{"type": "function", "function": map[string]any{
				"name": "lookup", "description": "d", "parameters": map[string]any{"type": "object"}, "strict": true,
			}},
		},
		"tool_choice":           map[string]any{"type": "function", "function": map[string]any{"name": "lookup"}},
		"max_completion_tokens": 256,
		"reasoning_effort":      "high",
	}
	request, err := decodeBridgeRequest(Chat, input)
	if err != nil {
		t.Fatalf("decodeBridgeRequest: %v", err)
	}
	if request.Model != "gpt-5" || !request.Stream {
		t.Errorf("model/stream = %q/%v", request.Model, request.Stream)
	}
	if request.MaxTokens != 256 {
		t.Errorf("max tokens = %v", request.MaxTokens)
	}
	if len(request.System) != 1 || request.System[0].Text != "be brief" {
		t.Errorf("system = %+v", request.System)
	}
	if len(request.Developer) != 1 || request.Developer[0].Text != "use tools" {
		t.Errorf("developer = %+v", request.Developer)
	}
	// The tool role becomes a user turn carrying a tool_result block.
	if len(request.Messages) != 3 {
		t.Fatalf("messages = %d: %+v", len(request.Messages), request.Messages)
	}
	if request.Messages[2].Role != "user" || request.Messages[2].Blocks[0].Kind != "tool_result" ||
		request.Messages[2].Blocks[0].CallID != "call_1" {
		t.Errorf("tool message = %+v", request.Messages[2])
	}
	// Reasoning text is prepended to the assistant turn and the budget the
	// gateway carried rides alongside the effort.
	assistant := request.Messages[1]
	if assistant.Blocks[0].Kind != "reasoning" || assistant.Blocks[0].Text != "think" {
		t.Errorf("assistant reasoning = %+v", assistant.Blocks[0])
	}
	want := map[string]any{"effort": "high", "budget_tokens": 32000}
	if !reflect.DeepEqual(request.Reasoning, want) {
		t.Errorf("reasoning = %#v, want %#v", request.Reasoning, want)
	}
	if request.ToolChoice != (bridgeToolChoice{Mode: "named", Name: "lookup"}) {
		t.Errorf("tool choice = %+v", request.ToolChoice)
	}
	wantTool := bridgeTool{Name: "lookup", Description: "d", Schema: map[string]any{"type": "object"}, Strict: true}
	if len(request.Tools) != 1 || !reflect.DeepEqual(request.Tools[0], wantTool) {
		t.Errorf("tools = %+v", request.Tools)
	}
}

func TestDecodeBridgeRequestErrors(t *testing.T) {
	cases := []struct {
		name     string
		protocol Protocol
		input    map[string]any
		contains string
	}{
		{"chat message not an object", Chat, map[string]any{"messages": []any{"hi"}}, "messages[0] must be an object"},
		{"chat unsupported role", Chat, map[string]any{"messages": []any{map[string]any{"role": "function"}}}, `unsupported role "function"`},
		{"chat tool call not an object", Chat, map[string]any{"messages": []any{
			map[string]any{"role": "assistant", "tool_calls": []any{7}},
		}}, "tool_calls[0] must be an object"},
		{"chat non-function tool", Chat, map[string]any{"tools": []any{map[string]any{"type": "retrieval"}}}, "must be a function tool"},
		{"responses bad input", Responses, map[string]any{"input": 42}, "input must be a string or array"},
		{"responses bad item", Responses, map[string]any{"input": []any{"x"}}, "input[0] must be an object"},
		{"responses unsupported item", Responses, map[string]any{"input": []any{
			map[string]any{"type": "web_search_call"},
		}}, "unsupported Responses item type"},
		{"responses bad block", Responses, map[string]any{"input": []any{
			map[string]any{"type": "message", "content": []any{map[string]any{"type": "video"}}},
		}}, "unsupported OpenAI content block type"},
		{"anthropic unsupported role", Anthropic, map[string]any{"messages": []any{
			map[string]any{"role": "tool", "content": "x"},
		}}, `unsupported role "tool"`},
		{"anthropic bad system", Anthropic, map[string]any{"system": []any{map[string]any{"type": "audio"}}}, "system: unsupported Anthropic content block type"},
		{"anthropic tool not an object", Anthropic, map[string]any{"tools": []any{"x"}}, "tools[0] must be an object"},
		{"unsupported protocol", SystemOne, map[string]any{}, `unsupported input protocol "systemone"`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeBridgeRequest(test.protocol, test.input)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Errorf("err = %v, want containing %q", err, test.contains)
			}
		})
	}
}

func TestDecodeBridgeRequestResponses(t *testing.T) {
	input := map[string]any{
		"model":        "gpt-5",
		"instructions": "be brief",
		"input": []any{
			map[string]any{"type": "reasoning", "id": "rs_1", "summary": []any{map[string]any{"text": "thought"}}},
			map[string]any{"type": "function_call", "call_id": "call_1", "name": "lookup", "arguments": `{"q":"x"}`},
			map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "42"},
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "hi"}}},
		},
		"tools": []any{map[string]any{
			"type": "function", "name": "lookup", "description": "d", "parameters": map[string]any{"type": "object"},
		}},
		"tool_choice":       map[string]any{"type": "function", "name": "lookup"},
		"max_output_tokens": 128,
		"reasoning":         map[string]any{"effort": "high"},
		"text":              map[string]any{"format": map[string]any{"type": "json_object"}},
	}
	request, err := decodeBridgeRequest(Responses, input)
	if err != nil {
		t.Fatalf("decodeBridgeRequest: %v", err)
	}
	if request.MaxTokens != 128 {
		t.Errorf("max tokens = %v", request.MaxTokens)
	}
	if len(request.System) != 1 || request.System[0].Text != "be brief" {
		t.Errorf("system = %+v", request.System)
	}
	if request.ToolChoice != (bridgeToolChoice{Mode: "named", Name: "lookup"}) {
		t.Errorf("tool choice = %+v", request.ToolChoice)
	}
	if !reflect.DeepEqual(request.ResponseFormat, map[string]any{"type": "json_object"}) {
		t.Errorf("response format = %#v", request.ResponseFormat)
	}
	wantTool := bridgeTool{Name: "lookup", Description: "d", Schema: map[string]any{"type": "object"}}
	if len(request.Tools) != 1 || !reflect.DeepEqual(request.Tools[0], wantTool) {
		t.Errorf("tools = %+v", request.Tools)
	}
	// Reasoning and the following function_call do NOT merge: a reasoning block
	// stops appendBridgeBlock from grouping, so the turn splits in two.
	if len(request.Messages) != 4 {
		t.Fatalf("messages = %d: %+v", len(request.Messages), request.Messages)
	}
	if request.Messages[0].Role != "assistant" || request.Messages[0].Blocks[0].ID != "rs_1" {
		t.Errorf("reasoning message = %+v", request.Messages[0])
	}
	if request.Messages[1].Role != "assistant" || request.Messages[1].Blocks[0].Kind != "tool_call" {
		t.Errorf("call message = %+v", request.Messages[1])
	}
	if request.Messages[2].Role != "user" || request.Messages[2].Blocks[0].Kind != "tool_result" {
		t.Errorf("output message = %+v", request.Messages[2])
	}
	if request.Messages[3].Role != "user" || request.Messages[3].Blocks[0].Text != "hi" {
		t.Errorf("user message = %+v", request.Messages[3])
	}

	// A string input collapses into a single user turn.
	stringInput, err := decodeBridgeRequest(Responses, map[string]any{"input": "just text"})
	if err != nil || len(stringInput.Messages) != 1 || stringInput.Messages[0].Blocks[0].Text != "just text" {
		t.Errorf("string input = %+v, err %v", stringInput.Messages, err)
	}
}

func TestDecodeBridgeRequestAnthropic(t *testing.T) {
	input := map[string]any{
		"model":          "claude-sonnet-4",
		"system":         "be brief",
		"max_tokens":     1024,
		"stop_sequences": []any{"END"},
		"thinking":       map[string]any{"type": "enabled", "budget_tokens": 2048},
		"messages": []any{
			// Inline system/developer turns are folded into the prompt.
			map[string]any{"role": "system", "content": "extra rules"},
			map[string]any{"role": "developer", "content": "dev note"},
			map[string]any{"role": "user", "content": "hi"},
		},
		"tools":       []any{map[string]any{"name": "lookup", "description": "d", "input_schema": map[string]any{"type": "object"}}},
		"tool_choice": map[string]any{"type": "tool", "name": "lookup"},
	}
	request, err := decodeBridgeRequest(Anthropic, input)
	if err != nil {
		t.Fatalf("decodeBridgeRequest: %v", err)
	}
	if len(request.System) != 2 || request.System[0].Text != "be brief" || request.System[1].Text != "extra rules" {
		t.Errorf("system = %+v", request.System)
	}
	if len(request.Developer) != 1 || request.Developer[0].Text != "dev note" {
		t.Errorf("developer = %+v", request.Developer)
	}
	// Only the user turn survives; system/developer were hoisted out.
	if len(request.Messages) != 1 || request.Messages[0].Role != "user" {
		t.Errorf("messages = %+v", request.Messages)
	}
	if request.MaxTokens != 1024 {
		t.Errorf("max tokens = %v", request.MaxTokens)
	}
	wantTool := bridgeTool{Name: "lookup", Description: "d", Schema: map[string]any{"type": "object"}}
	if len(request.Tools) != 1 || !reflect.DeepEqual(request.Tools[0], wantTool) {
		t.Errorf("tools = %+v", request.Tools)
	}
	if request.ToolChoice != (bridgeToolChoice{Mode: "named", Name: "lookup"}) {
		t.Errorf("tool choice = %+v", request.ToolChoice)
	}
}

func TestDecodeBridgeRequestAnthropicEffortPrecedence(t *testing.T) {
	// output_config.effort wins over the thinking block and the budget rides
	// along, so the level the client named survives.
	request, err := decodeBridgeRequest(Anthropic, map[string]any{
		"thinking":      map[string]any{"type": "enabled", "budget_tokens": 2048},
		"output_config": map[string]any{"effort": "max"},
	})
	if err != nil {
		t.Fatalf("decodeBridgeRequest: %v", err)
	}
	want := map[string]any{"type": "enabled", "budget_tokens": 2048, "effort": "max"}
	if !reflect.DeepEqual(request.Reasoning, want) {
		t.Errorf("reasoning = %#v, want %#v", request.Reasoning, want)
	}

	// "none" clears reasoning entirely instead of falling back to the budget.
	cleared, err := decodeBridgeRequest(Anthropic, map[string]any{
		"thinking": map[string]any{"type": "enabled", "budget_tokens": 2048},
		"effort":   "none",
	})
	if err != nil || cleared.Reasoning != nil {
		t.Errorf("reasoning = %#v, err %v", cleared.Reasoning, err)
	}

	// Without an effort the thinking block itself is the bridge value.
	thinking, err := decodeBridgeRequest(Anthropic, map[string]any{
		"thinking": map[string]any{"type": "enabled", "budget_tokens": 2048},
	})
	if err != nil || !reflect.DeepEqual(thinking.Reasoning, map[string]any{"type": "enabled", "budget_tokens": 2048}) {
		t.Errorf("reasoning = %#v, err %v", thinking.Reasoning, err)
	}
}

// -------------------------------------------------------- request encoding

func TestEncodeBridgeRequestValidation(t *testing.T) {
	cases := []struct {
		name     string
		protocol Protocol
		request  bridgeRequest
		contains string
	}{
		{
			name:     "file in responses instructions",
			protocol: Responses,
			request:  bridgeRequest{System: []bridgeBlock{{Kind: "file", FileID: "f1"}}},
			contains: "file content is not valid inside Responses instructions",
		},
		{
			name:     "anthropic file without data",
			protocol: Anthropic,
			request:  bridgeRequest{System: []bridgeBlock{{Kind: "file", FileID: "f1"}}},
			contains: `file "f1" cannot be represented by Anthropic Messages`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := encodeBridgeRequest(test.protocol, test.request)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Errorf("err = %v, want containing %q", err, test.contains)
			}
		})
	}
	if _, err := encodeBridgeRequest(SystemOne, bridgeRequest{}); err == nil {
		t.Error("unsupported output protocol accepted")
	}
	// An Anthropic file with data is fine.
	if _, err := encodeBridgeRequest(Anthropic, bridgeRequest{
		System: []bridgeBlock{{Kind: "file", FileID: "f1", Data: "AAA"}},
	}); err != nil {
		t.Errorf("valid file rejected: %v", err)
	}
}

func TestEncodeChatRequest(t *testing.T) {
	request := bridgeRequest{
		Model: "gpt-5", Stream: true,
		Temperature: 0.5, TopP: 0.9, MaxTokens: 256, Reasoning: "high",
		System:            []bridgeBlock{{Kind: "text", Text: "be brief"}},
		Developer:         []bridgeBlock{{Kind: "text", Text: "use tools"}},
		ParallelToolCalls: true,
		PromptCacheKey:    "session-1",
		Messages: []bridgeMessage{
			{Role: "user", Blocks: []bridgeBlock{{Kind: "text", Text: "hi"}}},
			{Role: "assistant", Blocks: []bridgeBlock{
				{Kind: "reasoning", Text: "think"},
				{Kind: "tool_call", ID: "call_1", Name: "lookup", ArgumentsJSON: `{"q":"x"}`},
			}},
			{Role: "user", Blocks: []bridgeBlock{
				{Kind: "tool_result", CallID: "call_1", Result: "42"},
				{Kind: "text", Text: "thanks"},
			}},
		},
		Tools:      []bridgeTool{{Name: "lookup", Description: "d", Schema: map[string]any{"type": "object"}}},
		ToolChoice: bridgeToolChoice{Mode: "required"},
	}
	output, err := encodeChatRequest(request)
	if err != nil {
		t.Fatalf("encodeChatRequest: %v", err)
	}
	if output["model"] != "gpt-5" || output["stream"] != true {
		t.Errorf("model/stream = %v/%v", output["model"], output["stream"])
	}
	if output["reasoning_effort"] != "high" {
		t.Errorf("reasoning_effort = %v", output["reasoning_effort"])
	}
	// Streaming asks for usage, and native-fidelity knobs are carried through.
	if !reflect.DeepEqual(output["stream_options"], map[string]any{"include_usage": true}) {
		t.Errorf("stream_options = %#v", output["stream_options"])
	}
	if output["prompt_cache_key"] != "session-1" || output["parallel_tool_calls"] != true {
		t.Errorf("native knobs = %#v", output)
	}
	messages := output["messages"].([]any)
	// system, developer, user, assistant, tool result, then the deferred text.
	if len(messages) != 6 {
		t.Fatalf("messages = %d: %#v", len(messages), messages)
	}
	if messages[0].(map[string]any)["role"] != "system" || messages[1].(map[string]any)["role"] != "developer" {
		t.Errorf("leading roles = %#v", messages[:2])
	}
	assistant := messages[3].(map[string]any)
	if assistant["reasoning_content"] != "think" {
		t.Errorf("assistant reasoning = %#v", assistant)
	}
	calls := assistant["tool_calls"].([]any)
	if len(calls) != 1 || calls[0].(map[string]any)["id"] != "call_1" {
		t.Errorf("tool calls = %#v", calls)
	}
	function := calls[0].(map[string]any)["function"].(map[string]any)
	if function["name"] != "lookup" || function["arguments"] != `{"q":"x"}` {
		t.Errorf("function = %#v", function)
	}
	tool := messages[4].(map[string]any)
	if tool["role"] != "tool" || tool["tool_call_id"] != "call_1" || tool["content"] != "42" {
		t.Errorf("tool message = %#v", tool)
	}
	// The user text was deferred so the tool result is not split from its call.
	last := messages[5].(map[string]any)
	if last["role"] != "user" || last["content"] != any("thanks") {
		t.Errorf("deferred message = %#v", last)
	}
	tools := output["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["type"] != "function" {
		t.Fatalf("tools = %#v", tools)
	}
	// strict is omitted when false rather than sent as false.
	if _, exists := tools[0].(map[string]any)["function"].(map[string]any)["strict"]; exists {
		t.Errorf("strict emitted: %#v", tools)
	}
	if output["tool_choice"] != any("required") {
		t.Errorf("tool_choice = %v", output["tool_choice"])
	}
}

func TestEncodeChatRequestBudgetCarrier(t *testing.T) {
	// With no assistant turn in history the budget rides on an empty assistant
	// message so a later Anthropic decode can restore the exact level.
	output, err := encodeChatRequest(bridgeRequest{
		Model:     "gpt-5",
		Reasoning: map[string]any{"effort": "max", "budget_tokens": 32000},
		Messages:  []bridgeMessage{{Role: "user", Blocks: []bridgeBlock{{Kind: "text", Text: "hi"}}}},
	})
	if err != nil {
		t.Fatalf("encodeChatRequest: %v", err)
	}
	messages := output["messages"].([]any)
	last := messages[len(messages)-1].(map[string]any)
	if last["role"] != "assistant" || last["content"] != nil || last["reasoning_budget_tokens"] != 32000 {
		t.Fatalf("budget carrier = %#v", last)
	}

	// An assistant turn carries it instead, and only once.
	output, err = encodeChatRequest(bridgeRequest{
		Model:     "gpt-5",
		Reasoning: map[string]any{"effort": "max", "budget_tokens": 32000},
		Messages: []bridgeMessage{
			{Role: "assistant", Blocks: []bridgeBlock{{Kind: "text", Text: "a"}}},
			{Role: "user", Blocks: []bridgeBlock{{Kind: "text", Text: "b"}}},
			{Role: "assistant", Blocks: []bridgeBlock{{Kind: "text", Text: "c"}}},
		},
	})
	if err != nil {
		t.Fatalf("encodeChatRequest: %v", err)
	}
	carriers := 0
	for _, raw := range output["messages"].([]any) {
		if _, exists := raw.(map[string]any)["reasoning_budget_tokens"]; exists {
			carriers++
		}
	}
	if carriers != 1 {
		t.Errorf("budget carried %d times, want 1: %#v", carriers, output["messages"])
	}
}

func TestEncodeChatRequestErrors(t *testing.T) {
	cases := []struct {
		name     string
		request  bridgeRequest
		contains string
	}{
		{
			name: "assistant before tool results",
			request: bridgeRequest{Messages: []bridgeMessage{
				{Role: "assistant", Blocks: []bridgeBlock{{Kind: "tool_call", ID: "c1", Name: "n"}}},
				{Role: "assistant", Blocks: []bridgeBlock{{Kind: "text", Text: "later"}}},
			}},
			contains: "assistant message appears before tool results for c1",
		},
		{
			name:     "assistant carries tool results",
			request:  bridgeRequest{Messages: []bridgeMessage{{Role: "assistant", Blocks: []bridgeBlock{{Kind: "tool_result", CallID: "c1"}}}}},
			contains: "assistant message contains tool results",
		},
		{
			name:     "user carries tool calls",
			request:  bridgeRequest{Messages: []bridgeMessage{{Role: "user", Blocks: []bridgeBlock{{Kind: "tool_call", ID: "c1", Name: "n"}}}}},
			contains: "user message contains tool calls",
		},
		{
			name:     "tool call missing name",
			request:  bridgeRequest{Messages: []bridgeMessage{{Role: "assistant", Blocks: []bridgeBlock{{Kind: "tool_call", ID: "c1"}}}}},
			contains: "tool call must contain id and name",
		},
		{
			name: "duplicate tool call id",
			request: bridgeRequest{Messages: []bridgeMessage{
				{Role: "assistant", Blocks: []bridgeBlock{{Kind: "tool_call", ID: "c1", Name: "a"}}},
				{Role: "user", Blocks: []bridgeBlock{{Kind: "tool_call", ID: "c1", Name: "b"}}},
			}},
			contains: "user message contains tool calls",
		},
		{
			name:     "orphan tool result",
			request:  bridgeRequest{Messages: []bridgeMessage{{Role: "user", Blocks: []bridgeBlock{{Kind: "tool_result", CallID: "c1"}}}}},
			contains: `tool result "c1" has no pending tool call`,
		},
		{
			name: "missing tool results",
			request: bridgeRequest{Messages: []bridgeMessage{
				{Role: "assistant", Blocks: []bridgeBlock{{Kind: "tool_call", ID: "c1", Name: "a"}}},
			}},
			contains: "tool calls are missing results for c1",
		},
		{
			name: "duplicate tool result",
			request: bridgeRequest{Messages: []bridgeMessage{
				{Role: "assistant", Blocks: []bridgeBlock{
					{Kind: "tool_call", ID: "c1", Name: "a"},
					{Kind: "tool_call", ID: "c2", Name: "b"},
				}},
				{Role: "user", Blocks: []bridgeBlock{
					{Kind: "tool_result", CallID: "c1", Result: "r1"},
					{Kind: "tool_result", CallID: "c1", Result: "r2"},
				}},
			}},
			contains: `duplicate tool result "c1"`,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := encodeChatRequest(test.request)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Errorf("err = %v, want containing %q", err, test.contains)
			}
		})
	}
}

func TestEncodeResponsesRequest(t *testing.T) {
	output := encodeResponsesRequest(bridgeRequest{
		Model: "gpt-5", MaxTokens: 128, Reasoning: map[string]any{"type": "enabled", "budget_tokens": 2048},
		System:    []bridgeBlock{{Kind: "text", Text: "be brief"}},
		Developer: []bridgeBlock{{Kind: "text", Text: "dev note"}},
		Messages: []bridgeMessage{
			{Role: "user", Blocks: []bridgeBlock{
				{Kind: "text", Text: "look"},
				{Kind: "image", MediaType: "image/png", Data: "AAA"},
			}},
			{Role: "assistant", Blocks: []bridgeBlock{
				{Kind: "reasoning", Text: "unsourced"}, // dropped: no id, no ciphertext
				{Kind: "reasoning", ID: "rs_1", Text: "sourced"},
				{Kind: "text", Text: "sure"},
				{Kind: "tool_call", ID: "call_1", Name: "lookup", ArgumentsJSON: `{"q":"x"}`},
			}},
			{Role: "user", Blocks: []bridgeBlock{{Kind: "tool_result", CallID: "call_1", Result: "42"}}},
		},
		Tools:      []bridgeTool{{Name: "lookup", Description: "d", Schema: map[string]any{"type": "object"}}},
		ToolChoice: bridgeToolChoice{Mode: "named", Name: "lookup"},
	})
	if output["instructions"] != "be brief" {
		t.Errorf("instructions = %v", output["instructions"])
	}
	// A thinking block with a budget is projected onto reasoning.effort rather
	// than copied verbatim: 2048 is the "low" bucket.
	if !reflect.DeepEqual(output["reasoning"], map[string]any{"effort": "low"}) {
		t.Errorf("reasoning = %#v", output["reasoning"])
	}
	if output["max_output_tokens"] != 128 {
		t.Errorf("max_output_tokens = %v", output["max_output_tokens"])
	}
	items := output["input"].([]any)
	// developer item, user message, reasoning item, assistant message, call, output.
	if len(items) != 6 {
		t.Fatalf("input items = %d: %#v", len(items), items)
	}
	developer := items[0].(map[string]any)
	if developer["role"] != "developer" || developer["content"].([]any)[0].(map[string]any)["type"] != "input_text" {
		t.Errorf("developer item = %#v", developer)
	}
	user := items[1].(map[string]any)
	userParts := user["content"].([]any)
	if userParts[0].(map[string]any)["type"] != "input_text" {
		t.Errorf("user content = %#v", userParts[0])
	}
	// Inline image data becomes a data URL.
	image := userParts[1].(map[string]any)
	if image["type"] != "input_image" || image["image_url"] != "data:image/png;base64,AAA" {
		t.Errorf("image part = %#v", image)
	}
	reasoning := items[2].(map[string]any)
	if reasoning["type"] != "reasoning" || reasoning["id"] != "rs_1" {
		t.Errorf("reasoning item = %#v", reasoning)
	}
	assistant := items[3].(map[string]any)
	if assistant["role"] != "assistant" || assistant["content"].([]any)[0].(map[string]any)["type"] != "output_text" {
		t.Errorf("assistant item = %#v", assistant)
	}
	call := items[4].(map[string]any)
	if call["type"] != "function_call" || call["call_id"] != "call_1" || call["arguments"] != `{"q":"x"}` {
		t.Errorf("call item = %#v", call)
	}
	result := items[5].(map[string]any)
	if result["type"] != "function_call_output" || result["output"] != "42" {
		t.Errorf("output item = %#v", result)
	}
	tool := output["tools"].([]any)[0].(map[string]any)
	// Responses always carries strict, unlike the Chat encoder.
	if tool["strict"] != false || tool["name"] != "lookup" {
		t.Errorf("tool = %#v", tool)
	}
	if !reflect.DeepEqual(output["tool_choice"], map[string]any{"type": "function", "name": "lookup"}) {
		t.Errorf("tool_choice = %#v", output["tool_choice"])
	}

	// tool_choice is dropped when there are no tools to constrain.
	bare := encodeResponsesRequest(bridgeRequest{Model: "m", ToolChoice: bridgeToolChoice{Mode: "auto"}})
	if _, exists := bare["tool_choice"]; exists {
		t.Errorf("tool_choice emitted without tools: %#v", bare)
	}
}

func TestEncodeAnthropicRequest(t *testing.T) {
	output := encodeAnthropicRequest(bridgeRequest{
		Model: "claude-sonnet-4",
		// Reasoning carries a budget the client named; it is preserved verbatim.
		Reasoning: map[string]any{"type": "enabled", "budget_tokens": 32000, "effort": "max"},
		System:    []bridgeBlock{{Kind: "text", Text: "be brief"}},
		Developer: []bridgeBlock{{Kind: "text", Text: "dev note"}},
		Messages: []bridgeMessage{
			{Role: "user", Blocks: []bridgeBlock{{Kind: "text", Text: "hi"}}},
			{Role: "user", Blocks: []bridgeBlock{{Kind: "text", Text: "again"}}}, // merged
			{Role: "assistant", Blocks: []bridgeBlock{{Kind: "text", Text: "sure"}}},
			{Role: "assistant", Blocks: []bridgeBlock{
				{Kind: "tool_call", ID: "call_1", Name: "lookup", ArgumentsJSON: `{"q":"x"}`},
			}},
		},
		Tools:      []bridgeTool{{Name: "lookup", Description: "d", Schema: map[string]any{"type": "object"}}},
		ToolChoice: bridgeToolChoice{Mode: "auto"},
	})
	// max_tokens defaults to 4096 when the client named none.
	if output["max_tokens"] != 4096 {
		t.Errorf("max_tokens = %v, want 4096", output["max_tokens"])
	}
	if !reflect.DeepEqual(output["thinking"], map[string]any{"type": "enabled", "budget_tokens": float64(32000)}) {
		t.Errorf("thinking = %#v", output["thinking"])
	}
	if !reflect.DeepEqual(output["output_config"], map[string]any{"effort": "max"}) {
		t.Errorf("output_config = %#v", output["output_config"])
	}
	// System and developer blocks concatenate into one Anthropic system list.
	system := output["system"].([]any)
	if len(system) != 2 {
		t.Fatalf("system = %#v", system)
	}
	messages := output["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %d: %#v", len(messages), messages)
	}
	merged := messages[0].(map[string]any)
	if merged["role"] != "user" || len(merged["content"].([]any)) != 2 {
		t.Errorf("merged user message = %#v", merged)
	}
	assistant := messages[1].(map[string]any)
	if assistant["role"] != "assistant" || len(assistant["content"].([]any)) != 2 {
		t.Errorf("assistant message = %#v", assistant)
	}
	tool := output["tools"].([]any)[0].(map[string]any)
	if tool["name"] != "lookup" || !reflect.DeepEqual(tool["input_schema"], map[string]any{"type": "object"}) {
		t.Errorf("tool = %#v", tool)
	}
	if !reflect.DeepEqual(output["tool_choice"], map[string]any{"type": "auto"}) {
		t.Errorf("tool_choice = %#v", output["tool_choice"])
	}

	// "none" has no Anthropic spelling, so the whole tool set is omitted.
	disabled := encodeAnthropicRequest(bridgeRequest{
		Model:      "m",
		Tools:      []bridgeTool{{Name: "lookup"}},
		ToolChoice: bridgeToolChoice{Mode: "none"},
	})
	if _, exists := disabled["tools"]; exists {
		t.Errorf("tools emitted for tool_choice none: %#v", disabled)
	}

	// An empty turn is skipped rather than emitted with empty content.
	empty := encodeAnthropicRequest(bridgeRequest{
		Model:    "m",
		Messages: []bridgeMessage{{Role: "user", Blocks: nil}},
	})
	if len(empty["messages"].([]any)) != 0 {
		t.Errorf("empty turn emitted: %#v", empty["messages"])
	}
}

// ---------------------------------------------------- cross-protocol requests

func TestConvertRequestChatToAnthropic(t *testing.T) {
	output, err := ConvertRequest(Chat, Anthropic, map[string]any{
		"model": "gpt-5",
		"messages": []any{
			map[string]any{"role": "system", "content": "be brief"},
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{"role": "assistant", "content": "checking", "tool_calls": []any{
				map[string]any{"id": "call_1", "type": "function", "function": map[string]any{"name": "lookup", "arguments": `{"q":"x"}`}},
			}},
			map[string]any{"role": "tool", "tool_call_id": "call_1", "content": "42"},
		},
		"tools": []any{map[string]any{"type": "function", "function": map[string]any{
			"name": "lookup", "description": "d", "parameters": map[string]any{"type": "object"},
		}}},
		"tool_choice": "auto",
		"max_tokens":  1024,
	})
	if err != nil {
		t.Fatalf("ConvertRequest: %v", err)
	}
	if output["model"] != "gpt-5" || output["max_tokens"] != 1024 {
		t.Errorf("model/max_tokens = %v/%v", output["model"], output["max_tokens"])
	}
	system := output["system"].([]any)
	if len(system) != 1 || system[0].(map[string]any)["text"] != "be brief" {
		t.Errorf("system = %#v", system)
	}
	messages := output["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("messages = %d: %#v", len(messages), messages)
	}
	user := messages[0].(map[string]any)
	if user["role"] != "user" || user["content"].([]any)[0].(map[string]any)["text"] != "hi" {
		t.Errorf("user message = %#v", user)
	}
	// The assistant text and its tool call stay on one turn.
	assistant := messages[1].(map[string]any)
	assistantParts := assistant["content"].([]any)
	if assistant["role"] != "assistant" || len(assistantParts) != 2 {
		t.Fatalf("assistant message = %#v", assistant)
	}
	if assistantParts[0].(map[string]any)["text"] != "checking" {
		t.Errorf("assistant text = %#v", assistantParts[0])
	}
	toolUse := assistantParts[1].(map[string]any)
	if toolUse["type"] != "tool_use" || toolUse["name"] != "lookup" || toolUse["id"] != "call_1" {
		t.Errorf("tool_use = %#v", toolUse)
	}
	// Arguments arrive as a JSON string and are parsed into the tool input.
	if !reflect.DeepEqual(toolUse["input"], map[string]any{"q": "x"}) {
		t.Errorf("tool_use input = %#v", toolUse["input"])
	}
	result := messages[2].(map[string]any)
	if result["role"] != "user" || result["content"].([]any)[0].(map[string]any)["tool_use_id"] != "call_1" {
		t.Errorf("tool result turn = %#v", result)
	}
	if !reflect.DeepEqual(output["tool_choice"], map[string]any{"type": "auto"}) {
		t.Errorf("tool_choice = %#v", output["tool_choice"])
	}
}

func TestConvertRequestAnthropicToChat(t *testing.T) {
	output, err := ConvertRequest(Anthropic, Chat, map[string]any{
		"model":      "claude-sonnet-4",
		"max_tokens": 2048,
		"system":     "be brief",
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "thinking", "thinking": "why", "signature": "sig"},
				map[string]any{"type": "text", "text": "sure"},
				map[string]any{"type": "tool_use", "id": "call_1", "name": "lookup", "input": map[string]any{"q": "x"}},
			}},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": "call_1", "content": "42"},
			}},
		},
		"tools":       []any{map[string]any{"name": "lookup", "input_schema": map[string]any{"type": "object"}}},
		"tool_choice": map[string]any{"type": "any"},
	})
	if err != nil {
		t.Fatalf("ConvertRequest: %v", err)
	}
	messages := output["messages"].([]any)
	if len(messages) != 4 {
		t.Fatalf("messages = %d: %#v", len(messages), messages)
	}
	if messages[0].(map[string]any)["role"] != "system" || messages[0].(map[string]any)["content"] != any("be brief") {
		t.Errorf("system message = %#v", messages[0])
	}
	assistant := messages[2].(map[string]any)
	// Only reasoning_content survives: the Anthropic signature has no place in
	// a Chat request body (encodeChatRequest never emits reasoning_signature).
	if assistant["reasoning_content"] != "why" {
		t.Errorf("assistant reasoning = %#v", assistant)
	}
	call := assistant["tool_calls"].([]any)[0].(map[string]any)
	function := call["function"].(map[string]any)
	// A structured input is re-serialized into the Chat arguments string.
	if function["arguments"] != `{"q":"x"}` || function["name"] != "lookup" {
		t.Errorf("function = %#v", function)
	}
	tool := messages[3].(map[string]any)
	if tool["role"] != "tool" || tool["tool_call_id"] != "call_1" || tool["content"] != "42" {
		t.Errorf("tool message = %#v", tool)
	}
	// Anthropic "any" becomes the Chat spelling "required".
	if output["tool_choice"] != any("required") {
		t.Errorf("tool_choice = %v", output["tool_choice"])
	}
}

func TestConvertRequestResponsesToAnthropic(t *testing.T) {
	output, err := ConvertRequest(Responses, Anthropic, map[string]any{
		"model":        "gpt-5",
		"instructions": "be brief",
		"input": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "hi"}}},
			map[string]any{"type": "function_call", "call_id": "call_1", "name": "lookup", "arguments": `{"q":"x"}`},
			map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "42"},
		},
		"max_output_tokens": 512,
		"reasoning":         map[string]any{"effort": "max"},
		"tools":             []any{map[string]any{"type": "function", "name": "lookup"}},
		"tool_choice":       "required",
	})
	if err != nil {
		t.Fatalf("ConvertRequest: %v", err)
	}
	// "max" synthesizes the top-of-range budget, since none was carried.
	if !reflect.DeepEqual(output["thinking"], map[string]any{"type": "enabled", "budget_tokens": float64(32768)}) {
		t.Errorf("thinking = %#v", output["thinking"])
	}
	if !reflect.DeepEqual(output["output_config"], map[string]any{"effort": "max"}) {
		t.Errorf("output_config = %#v", output["output_config"])
	}
	system := output["system"].([]any)
	if len(system) != 1 || system[0].(map[string]any)["text"] != "be brief" {
		t.Errorf("system = %#v", system)
	}
	messages := output["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("messages = %d: %#v", len(messages), messages)
	}
	if messages[1].(map[string]any)["role"] != "assistant" ||
		messages[1].(map[string]any)["content"].([]any)[0].(map[string]any)["type"] != "tool_use" {
		t.Errorf("tool_use turn = %#v", messages[1])
	}
	if messages[2].(map[string]any)["content"].([]any)[0].(map[string]any)["type"] != "tool_result" {
		t.Errorf("tool_result turn = %#v", messages[2])
	}
	if !reflect.DeepEqual(output["tool_choice"], map[string]any{"type": "any"}) {
		t.Errorf("tool_choice = %#v", output["tool_choice"])
	}
}

func TestConvertRequestSameProtocolClones(t *testing.T) {
	input := map[string]any{"model": "m", "vendor_specific": true}
	output, err := ConvertRequest(Chat, Chat, input)
	if err != nil {
		t.Fatalf("ConvertRequest: %v", err)
	}
	if !reflect.DeepEqual(output, input) {
		t.Fatalf("clone = %#v", output)
	}
	// The clone is deep: mutating it cannot reach the input.
	output["model"] = "other"
	if input["model"] != "m" {
		t.Error("ConvertRequest returned the caller's map")
	}
}
