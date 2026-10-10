package models

import (
	"reflect"
	"testing"

	"opencode2api/internal/config"
	wire "opencode2api/internal/protocol"
)

func TestIsFreeModel(t *testing.T) {
	cases := []struct {
		model string
		want  bool
	}{
		{"grok-4-free", true},
		{"GLM-4.5-FREE", true},
		{"free", true},
		{"Free", true},
		{"a-free-model", true},
		{"gpt-5", false},
		{"", false},
		{"claude-sonnet-4", false},
		// Substring match only: "freezen" contains "free" and is therefore
		// treated as a free-tier model by the name fallback.
		{"freezen-v2", true},
		{"breeze-4", false},
		{"prefer-online", false},
	}
	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			if got := isFreeModel(tc.model); got != tc.want {
				t.Fatalf("isFreeModel(%q) = %v, want %v", tc.model, got, tc.want)
			}
		})
	}
}

func TestCapabilityTier(t *testing.T) {
	cases := []struct {
		name       string
		providerID string
		api        string
		wantTier   config.Tier
		wantOK     bool
	}{
		{"provider id opencode-zen", "opencode-zen", "", config.TierZen, true},
		{"provider id opencode", "opencode", "", config.TierZen, true},
		{"provider id opencode-go", "opencode-go", "", config.TierGo, true},
		{"api path /zen/", "other", "https://x/v1/zen/chat", config.TierZen, true},
		{"api path /go/", "other", "https://x/v1/go/chat", config.TierGo, true},
		{"matching is case insensitive", "OpenCode-Zen", "", config.TierZen, true},
		{"surrounding whitespace is trimmed", "  opencode-go  ", "", config.TierGo, true},
		{"go wins when both markers appear", "opencode-zen", "/go/", config.TierGo, true},
		{"go in the provider id wins over zen in the api", "opencode-go", "/zen/", config.TierGo, true},
		{"unrelated provider", "openrouter", "https://openrouter.ai/api/v1", "", false},
		{"empty inputs", "", "", "", false},
		{"no marker at all", "gogo", "https://api/v1", "", false},
		// Any occurrence of "opencode" is enough; there is no word boundary.
		{"substring of opencode still matches zen", "opencodex-goo", "", config.TierZen, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tier, ok := capabilityTier(tc.providerID, tc.api)
			if tier != tc.wantTier || ok != tc.wantOK {
				t.Fatalf("capabilityTier(%q, %q) = (%q, %v), want (%q, %v)", tc.providerID, tc.api, tier, ok, tc.wantTier, tc.wantOK)
			}
		})
	}
}

func TestProtocolForSDK(t *testing.T) {
	cases := []struct {
		name   string
		npm    string
		want   wire.Protocol
		wantOK bool
	}{
		{"anthropic sdk", "@ai-sdk/anthropic", wire.Anthropic, true},
		{"anthropic substring wins over the rest", "@ai-sdk/anthropic/openai", wire.Anthropic, true},
		{"exact openai sdk", "@ai-sdk/openai", wire.Responses, true},
		{"openai-responses matches neither rule", "@ai-sdk/openai-responses", "", false},
		{"suffix /openai", "@ai-sdk/openai", wire.Responses, true},
		{"openai-compatible", "@ai-sdk/openai-compatible", wire.Chat, true},
		{"case insensitive and trimmed", "  @AI-SDK/ANTHROPIC  ", wire.Anthropic, true},
		{"unknown sdk", "@ai-sdk/google", "", false},
		{"empty sdk", "", "", false},
		{"openai-compatible prefix is not matched as suffix /openai", "openai-compatible", wire.Chat, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := protocolForSDK(tc.npm)
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("protocolForSDK(%q) = (%q, %v), want (%q, %v)", tc.npm, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestMetadataProviderRank(t *testing.T) {
	cases := []struct {
		key  string
		want int
	}{
		{"opencode", 0},
		{"opencode-zen", 0},
		{"opencode_zen", 0},
		{"OpenCode-Zen", 0},
		{"OPENCODE", 0},
		{"opencode-go", 1},
		{"opencodex", 1},
		{"my-opencode-proxy", 1},
		{"anthropic", 2},
		{"google", 2},
		{"", 2},
		// The rank function does not trim: a trailing space drops the exact
		// rank-0 match to the generic rank-1 bucket.
		{"opencode ", 1},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			if got := metadataProviderRank(tc.key); got != tc.want {
				t.Fatalf("metadataProviderRank(%q) = %d, want %d", tc.key, got, tc.want)
			}
		})
	}
}

func TestMetadataDeprecated(t *testing.T) {
	cases := []struct {
		name  string
		model map[string]any
		want  bool
	}{
		{"empty model", map[string]any{}, false},
		{"explicit deprecated true", map[string]any{"deprecated": true}, true},
		{"explicit deprecated false", map[string]any{"deprecated": false}, false},
		{"status deprecated", map[string]any{"status": "deprecated"}, true},
		{"status retired", map[string]any{"status": "RETIRED"}, true},
		{"status disabled", map[string]any{"status": "disabled"}, true},
		{"status active", map[string]any{"status": "active"}, false},
		{"lifecycle retired", map[string]any{"lifecycle": "Retired"}, true},
		{"lifecycle preview is not deprecated", map[string]any{"lifecycle": "preview"}, false},
		{"status wins over lifecycle", map[string]any{"status": "active", "lifecycle": "retired"}, false},
		{"deprecated_at present", map[string]any{"deprecated_at": "2026-01-01"}, true},
		{"retirement_date present", map[string]any{"retirement_date": "2026-01-01"}, true},
		{"deprecated_at null is not deprecated", map[string]any{"deprecated_at": nil}, false},
		{"retirement_date null is not deprecated", map[string]any{"retirement_date": nil}, false},
		{"unrelated date field is ignored", map[string]any{"released_at": "2020-01-01"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := metadataDeprecated(tc.model); got != tc.want {
				t.Fatalf("metadataDeprecated(%v) = %v, want %v", tc.model, got, tc.want)
			}
		})
	}
}

func TestNumberPointer(t *testing.T) {
	cases := []struct {
		name   string
		object map[string]any
		key    string
		want   *float64
	}{
		{"zero cost is a pointer to zero", map[string]any{"input": 0.0}, "input", ptr(0)},
		{"fractional cost", map[string]any{"input": 1.5}, "input", ptr(1.5)},
		{"missing key", map[string]any{}, "input", nil},
		{"explicit null", map[string]any{"input": nil}, "input", nil},
		// Only float64 is accepted: a Go int built in-process is rejected,
		// even though encoding/json always produces float64 for this map shape.
		{"go int is rejected", map[string]any{"input": 3}, "input", nil},
		{"numeric string is rejected", map[string]any{"input": "3"}, "input", nil},
		{"bool is rejected", map[string]any{"input": true}, "input", nil},
		{"negative cost", map[string]any{"input": -2.0}, "input", ptr(-2)},
		{"nil map", nil, "input", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := numberPointer(tc.object, tc.key)
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("numberPointer = %v, want nil", *got)
			case tc.want != nil && got == nil:
				t.Fatalf("numberPointer = nil, want %v", *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Fatalf("numberPointer = %v, want %v", *got, *tc.want)
			}
		})
	}
}

func ptr(v float64) *float64 { return &v }

func TestNormalizeModelIDs(t *testing.T) {
	cases := []struct {
		name  string
		input []string
		want  []string
	}{
		{"nil stays empty", nil, []string{}},
		{"empty stays empty", []string{}, []string{}},
		{"sorted", []string{"c", "a", "b"}, []string{"a", "b", "c"}},
		{"duplicates removed", []string{"a", "a", "b"}, []string{"a", "b"}},
		{"whitespace trimmed before dedupe", []string{" a", "a ", "a"}, []string{"a"}},
		{"blank entries dropped", []string{"", " ", "  ", "a", ""}, []string{"a"}},
		{"case sensitive keys are distinct", []string{"B", "a", "b"}, []string{"B", "a", "b"}},
		{"already normalized is stable", []string{"a", "b"}, []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeModelIDs(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("normalizeModelIDs(%v) = %#v, want %#v", tc.input, got, tc.want)
			}
		})
	}
}

func TestDecodeModelsDev(t *testing.T) {
	t.Run("full payload", func(t *testing.T) {
		data := []byte(`{
			"opencode": {
				"id": "opencode",
				"models": {
					"zen-free": {"cost": {"input": 0, "output": 0}, "limit": {"context": 200000, "input": 100000, "output": 64000}},
					"shared": {"id": "shared-id", "cost": {"input": 1.5, "output": 3}},
					"gone": {"cost": {"input": 0, "output": 0}, "status": "deprecated"},
					"implicit": {}
				}
			},
			"opencode-go": {
				"name": "OpenCode Go",
				"models": {"shared": {"cost": {"input": 9, "output": 9}}}
			},
			"anthropic": {
				"models": {"claude-x": {"cost": {"input": 3, "output": 15}}}
			}
		}`)
		got, err := decodeModelsDev(data)
		if err != nil {
			t.Fatalf("decodeModelsDev: %v", err)
		}
		want := map[string]Price{
			"zen-free": {ID: "zen-free", Input: ptr(0), Output: ptr(0), ContextWindow: 200000, MaxInput: 100000, MaxOutput: 64000},
			// The map key is "shared" but the model id field wins.
			"shared-id": {ID: "shared-id", Input: ptr(1.5), Output: ptr(3)},
			// The Go provider's key is also "shared", but it carries no id
			// field, so it lands under the raw key instead of colliding with
			// "shared-id" and is not de-duplicated against it.
			"shared": {ID: "shared", Input: ptr(9), Output: ptr(9)},
			"gone":   {ID: "gone", Input: ptr(0), Output: ptr(0), Deprecated: true},
			// No cost/limit at all: known model, unknown cost.
			"implicit": {ID: "implicit"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("decodeModelsDev() = %#v, want %#v", got, want)
		}
	})

	t.Run("opencode provider wins duplicate model ids", func(t *testing.T) {
		data := []byte(`{
			"opencode": {"models": {"dup": {"cost": {"input": 1, "output": 1}}}},
			"opencode-zen": {"models": {"dup": {"cost": {"input": 2, "output": 2}}}}
		}`)
		got, err := decodeModelsDev(data)
		if err != nil {
			t.Fatalf("decodeModelsDev: %v", err)
		}
		if got["dup"].Input == nil || *got["dup"].Input != 1 {
			t.Fatalf("expected the first rank-0 provider to win, got %#v", got["dup"])
		}
	})

	t.Run("rank one provider must identify as opencode", func(t *testing.T) {
		data := []byte(`{
			"opencode-mirror": {"name": "Unrelated Mirror", "models": {"m": {"cost": {"input": 0, "output": 0}}}},
			"opencode-zen": {"id": "opencode", "models": {"kept": {"cost": {"input": 0, "output": 0}}}}
		}`)
		got, err := decodeModelsDev(data)
		if err != nil {
			t.Fatalf("decodeModelsDev: %v", err)
		}
		if _, exists := got["m"]; exists {
			t.Fatalf("rank-1 provider without an opencode identity must be skipped, got %#v", got)
		}
		if _, exists := got["kept"]; !exists {
			t.Fatalf("expected kept model, got %#v", got)
		}
	})

	t.Run("rank two providers are always skipped", func(t *testing.T) {
		data := []byte(`{
			"google": {"id": "opencode-lookalike", "models": {"g": {"cost": {"input": 0, "output": 0}}}}
		}`)
		if _, err := decodeModelsDev(data); err == nil {
			t.Fatal("expected an error when no OpenCode metadata is present")
		}
	})

	t.Run("empty models map errors", func(t *testing.T) {
		if _, err := decodeModelsDev([]byte(`{"opencode":{"models":{}}}`)); err == nil {
			t.Fatal("expected an error for an empty models map")
		}
	})

	t.Run("invalid json errors", func(t *testing.T) {
		if _, err := decodeModelsDev([]byte(`{not json`)); err == nil {
			t.Fatal("expected an error for malformed json")
		}
	})

	t.Run("non-object payload errors", func(t *testing.T) {
		if _, err := decodeModelsDev([]byte(`[]`)); err == nil {
			t.Fatal("expected an error for a non-object payload")
		}
	})
}
