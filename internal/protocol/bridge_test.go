package protocol

import (
	"reflect"
	"testing"
)

func TestBudgetForEffort(t *testing.T) {
	cases := []struct {
		value any
		want  int
	}{
		{"minimal", 1024},
		{"low", 1024},
		{"medium", 4096},
		{"high", 8192},
		{"xhigh", 16384},
		{"max", 32768},
		{"  HIGH  ", 8192},
		{"XHigh", 16384},
		// Unknown levels and non-strings fall back to the medium rung.
		{"adaptive", 4096},
		{"", 4096},
		{123, 4096},
		{nil, 4096},
	}
	for _, tc := range cases {
		if got := budgetForEffort(tc.value); got != tc.want {
			t.Errorf("budgetForEffort(%#v) = %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestEffortForThinkingBudget(t *testing.T) {
	cases := []struct {
		budget int
		want   string
	}{
		{0, "high"},
		{-5, "high"},
		{1, "low"},
		{2048, "low"},
		{2049, "medium"},
		{4096, "medium"},
		{8191, "medium"},
		{8192, "high"},
		{8193, "high"},
		{16383, "high"},
		{16384, "xhigh"},
		{32000, "xhigh"},
		{32767, "xhigh"},
		{32768, "max"},
		{64000, "max"},
	}
	for _, tc := range cases {
		if got := effortForThinkingBudget(tc.budget); got != tc.want {
			t.Errorf("effortForThinkingBudget(%d) = %q, want %q", tc.budget, got, tc.want)
		}
	}
}

// TestEffortBudgetRoundTrip checks the documented inverse relationship. Only
// "high" and above round trip exactly: "minimal" synthesizes the same 1024
// budget as "low" and reads back as "low", and "max" synthesizes 32768 which
// is the "max" threshold, so it survives too.
func TestEffortBudgetRoundTrip(t *testing.T) {
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		budget := budgetForEffort(effort)
		if got := effortForThinkingBudget(budget); got != effort {
			t.Errorf("budgetForEffort(%q) = %d -> effortForThinkingBudget = %q", effort, budget, got)
		}
	}
	if got := effortForThinkingBudget(budgetForEffort("minimal")); got != "low" {
		t.Errorf("minimal round trip = %q, want low", got)
	}
}

func TestNormalizeEffortValue(t *testing.T) {
	cases := []struct {
		value   any
		want    any
		wantOK  bool
		comment string
	}{
		{"none", nil, true, "none clears the level"},
		{"DISABLED", nil, true, "case insensitive"},
		{"  None  ", nil, true, "trimmed before comparison"},
		{"", nil, false, "empty is not an explicit statement"},
		{"   ", nil, false, "whitespace only is not an explicit statement"},
		{" high ", " high ", true, "passed through verbatim, spaces included"},
		{"Minimal", "Minimal", true, "unknown levels are not rewritten"},
		{123, nil, false, "non-string"},
		{nil, nil, false, "nil"},
		{true, nil, false, "bool"},
	}
	for _, tc := range cases {
		got, ok := normalizeEffortValue(tc.value)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("normalizeEffortValue(%#v) = (%#v, %v), want (%#v, %v) // %s",
				tc.value, got, ok, tc.want, tc.wantOK, tc.comment)
		}
	}
}

func TestNumericValue(t *testing.T) {
	cases := []struct {
		value  any
		want   int
		wantOK bool
	}{
		{float64(2048), 2048, true},
		{1024.9, 1024, true}, // truncation toward zero
		{-3.7, -3, true},
		{4096, 4096, true},
		{int64(8192), 8192, true},
		{"2048", 0, false},
		{nil, 0, false},
		{true, 0, false},
	}
	for _, tc := range cases {
		got, ok := numericValue(tc.value)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("numericValue(%#v) = (%d, %v), want (%d, %v)", tc.value, got, ok, tc.want, tc.wantOK)
		}
		if zero := numericValueOrZero(tc.value); zero != tc.want {
			t.Errorf("numericValueOrZero(%#v) = %d, want %d", tc.value, zero, tc.want)
		}
	}
}

func TestThinkingFor(t *testing.T) {
	got := thinkingFor("xhigh", 0)
	want := map[string]any{"type": "enabled", "budget_tokens": float64(16384)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("thinkingFor(xhigh, 0) = %#v, want %#v", got, want)
	}
	// A positive budget is kept verbatim rather than re-derived.
	got = thinkingFor("low", 32000)
	want = map[string]any{"type": "enabled", "budget_tokens": float64(32000)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("thinkingFor(low, 32000) = %#v, want %#v", got, want)
	}
	// Non-positive budgets fall through to the rung's lower bound.
	got = thinkingFor("high", -1)
	want = map[string]any{"type": "enabled", "budget_tokens": float64(8192)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("thinkingFor(high, -1) = %#v, want %#v", got, want)
	}
	if got := thinkingFor("unknown-level", 0); got["budget_tokens"] != float64(4096) {
		t.Errorf("thinkingFor(unknown, 0) budget = %v, want 4096", got["budget_tokens"])
	}
}

func TestWithReasoningBudget(t *testing.T) {
	if got := withReasoningBudget(nil, 0); got != nil {
		t.Errorf("withReasoningBudget(nil, 0) = %#v, want nil", got)
	}
	if got := withReasoningBudget("high", 0); got != "high" {
		t.Errorf("withReasoningBudget(high, 0) = %#v, want unchanged", got)
	}
	got := withReasoningBudget("high", 2048)
	want := map[string]any{"budget_tokens": 2048, "effort": "high"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("withReasoningBudget(string, 2048) = %#v, want %#v", got, want)
	}

	original := map[string]any{"effort": "max"}
	got = withReasoningBudget(original, 1024)
	want = map[string]any{"effort": "max", "budget_tokens": 1024}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("withReasoningBudget(map, 1024) = %#v, want %#v", got, want)
	}
	if _, mutated := original["budget_tokens"]; mutated {
		t.Errorf("withReasoningBudget mutated its input: %#v", original)
	}
}

func TestReasoningBudget(t *testing.T) {
	cases := []struct {
		value any
		want  int
	}{
		{nil, 0},
		{"high", 0},
		{float64(2048), 0},
		{map[string]any{"budget_tokens": float64(4096)}, 4096},
		{map[string]any{"budget_tokens": float64(0)}, 0},
		{map[string]any{"budget_tokens": float64(-1)}, 0},
		// An effort states a level, not a budget.
		{map[string]any{"effort": "high"}, 0},
		{map[string]any{"reasoning_effort": "high"}, 0},
		{map[string]any{"effort": "high", "reasoning_budget_tokens": float64(2048)}, 2048},
		{map[string]any{"reasoning_effort": "high", "reasoning_budget_tokens": "2048"}, 0},
		// budget_tokens wins over reasoning_budget_tokens when positive.
		{map[string]any{"effort": "high", "budget_tokens": float64(8192), "reasoning_budget_tokens": float64(1)}, 8192},
	}
	for _, tc := range cases {
		if got := reasoningBudget(tc.value); got != tc.want {
			t.Errorf("reasoningBudget(%#v) = %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestExplicitEffort(t *testing.T) {
	cases := []struct {
		object map[string]any
		want   any
		wantOK bool
	}{
		{map[string]any{"effort": "max"}, "max", true},
		{map[string]any{"reasoning_effort": "HIGH"}, "HIGH", true},
		// "effort" is checked first and wins over "reasoning_effort".
		{map[string]any{"effort": "low", "reasoning_effort": "max"}, "low", true},
		// ok=true with a nil value means "none" was stated explicitly.
		{map[string]any{"effort": "none"}, nil, true},
		{map[string]any{"reasoning_effort": "disabled"}, nil, true},
		{map[string]any{"effort": 123}, nil, false},
		{map[string]any{"effort": " "}, nil, false},
		{map[string]any{"type": "high"}, nil, false},
		{map[string]any{}, nil, false},
	}
	for _, tc := range cases {
		got, ok := explicitEffort(tc.object)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("explicitEffort(%#v) = (%#v, %v), want (%#v, %v)", tc.object, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestSchemaOrDefault(t *testing.T) {
	defaultSchema := map[string]any{"type": "object", "properties": map[string]any{}}
	if got := schemaOrDefault(nil); !reflect.DeepEqual(got, defaultSchema) {
		t.Errorf("schemaOrDefault(nil) = %#v, want %#v", got, defaultSchema)
	}
	typed := map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "string"}}}
	if got := schemaOrDefault(typed); !reflect.DeepEqual(got, typed) {
		t.Errorf("schemaOrDefault(schema) = %#v, want the value unchanged", got)
	}
	// A non-nil non-map value is passed through as-is.
	if got := schemaOrDefault(false); got != false {
		t.Errorf("schemaOrDefault(false) = %#v, want false", got)
	}
}

func TestChatResponseFormat(t *testing.T) {
	cases := []struct {
		name   string
		format any
		want   any
	}{
		{"nil", nil, nil},
		{"non map", "text", nil},
		{"json_object", map[string]any{"type": "json_object", "extra": 1}, map[string]any{"type": "json_object"}},
		{
			"nested json_schema",
			map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "n"}},
			map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "n"}},
		},
		{
			// Non-map "json_schema" is ignored and the inline keys are lifted.
			"inline json_schema",
			map[string]any{"type": "json_schema", "json_schema": "oops", "name": "n", "strict": true},
			map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "n", "strict": true}},
		},
		{"empty json_schema", map[string]any{"type": "json_schema"}, nil},
		{"no Chat equivalent", map[string]any{"type": "text"}, nil},
		{"unknown type", map[string]any{"type": "weird"}, nil},
		{"missing type", map[string]any{"name": "n"}, nil},
	}
	for _, tc := range cases {
		if got := chatResponseFormat(tc.format); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: chatResponseFormat(%#v) = %#v, want %#v", tc.name, tc.format, got, tc.want)
		}
	}
}

func TestResponsesTextFormat(t *testing.T) {
	cases := []struct {
		name   string
		format any
		want   any
	}{
		{"nil", nil, nil},
		{"non map", "text", nil},
		{"text", map[string]any{"type": "text", "x": 1}, map[string]any{"type": "text", "x": 1}},
		{"json_object", map[string]any{"type": "json_object"}, map[string]any{"type": "json_object"}},
		{
			"nested flattened",
			map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "n", "strict": true}},
			map[string]any{"type": "json_schema", "name": "n", "strict": true},
		},
		{
			"inline passthrough",
			map[string]any{"type": "json_schema", "name": "n"},
			map[string]any{"type": "json_schema", "name": "n"},
		},
		{"unknown type", map[string]any{"type": "weird"}, nil},
	}
	for _, tc := range cases {
		if got := responsesTextFormat(tc.format); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: responsesTextFormat(%#v) = %#v, want %#v", tc.name, tc.format, got, tc.want)
		}
	}
}

func TestAnthropicThinking(t *testing.T) {
	thinking := func(budget int) map[string]any {
		return map[string]any{"type": "enabled", "budget_tokens": float64(budget)}
	}
	cases := []struct {
		name        string
		value       any
		wantThink   any
		wantOutputs map[string]any
	}{
		{"nil", nil, nil, nil},
		{"bool", true, nil, nil},
		{"plain string", "high", thinking(8192), map[string]any{"effort": "high"}},
		{"none string", "none", nil, nil},
		{"unknown string", "adaptive", thinking(4096), map[string]any{"effort": "adaptive"}},
		{"budget only number", float64(5000), thinking(5000), map[string]any{"effort": "medium"}},
		{"zero number", float64(0), nil, nil},
		{"negative number", float64(-10), nil, nil},
		{"disabled object", map[string]any{"type": "disabled"}, nil, nil},
		// "enabled" is passed through as the published effort verbatim.
		{"enabled with budget", map[string]any{"type": "enabled", "budget_tokens": float64(32000)},
			thinking(32000), map[string]any{"effort": "enabled"}},
		{"budget only object", map[string]any{"budget_tokens": float64(2048)},
			thinking(2048), map[string]any{"effort": "low"}},
		{"adaptive no budget", map[string]any{"type": "adaptive"},
			thinking(4096), map[string]any{"effort": "adaptive"}},
		// A stated effort outranks the budget but the budget is preserved.
		{"effort beats budget", map[string]any{"effort": "max", "budget_tokens": float64(2048)},
			thinking(2048), map[string]any{"effort": "max"}},
		{"reasoning_effort key", map[string]any{"reasoning_effort": "xhigh"},
			thinking(16384), map[string]any{"effort": "xhigh"}},
		{"explicit none clears", map[string]any{"effort": "none", "budget_tokens": float64(2048)}, nil, nil},
		{"type none clears", map[string]any{"type": "none"}, nil, nil},
	}
	for _, tc := range cases {
		gotThink, gotOutput := anthropicThinking(tc.value)
		if !reflect.DeepEqual(gotThink, tc.wantThink) {
			t.Errorf("%s: thinking = %#v, want %#v", tc.name, gotThink, tc.wantThink)
		}
		if !reflect.DeepEqual(gotOutput, tc.wantOutputs) {
			t.Errorf("%s: output_config = %#v, want %#v", tc.name, gotOutput, tc.wantOutputs)
		}
	}
}

func TestBridgeBlocksText(t *testing.T) {
	blocks := []bridgeBlock{
		{Kind: "text", Text: "a"},
		{Kind: "reasoning", Text: "hidden"},
		{Kind: "text", Text: "b"},
		{Kind: "tool_use", Text: "c"},
		{Kind: "text", Text: "d"},
	}
	if got := bridgeBlocksText(blocks); got != "abd" {
		t.Errorf("bridgeBlocksText = %q, want %q", got, "abd")
	}
	if got := bridgeBlocksText(nil); got != "" {
		t.Errorf("bridgeBlocksText(nil) = %q, want empty", got)
	}
	// An unknown kind is skipped like any non-text block.
	if got := bridgeBlocksText([]bridgeBlock{{Kind: "", Text: "x"}}); got != "" {
		t.Errorf("bridgeBlocksText(empty kind) = %q, want empty", got)
	}
}

func TestAsPrefix(t *testing.T) {
	cases := []struct {
		id     string
		prefix string
		want   string
	}{
		{"chatcmpl_abc", "chatcmpl", "chatcmpl_abc"},
		{"resp_abc", "resp", "resp_abc"},
		{"abc", "call", "call_abc"},
		{"resp_abc", "call", "call_abc"},
		{"msg_abc", "call", "call_abc"},
		// Every known prefix is trimmed in sequence, not just the first match.
		{"chatcmpl_resp_x", "call", "call_x"},
		{"msg_chatcmpl_y", "call", "call_chatcmpl_y"},
		{"", "call", "call_"},
		{"call_x", "call", "call_x"},
	}
	for _, tc := range cases {
		if got := asPrefix(tc.id, tc.prefix); got != tc.want {
			t.Errorf("asPrefix(%q, %q) = %q, want %q", tc.id, tc.prefix, got, tc.want)
		}
	}
}

func TestNormalizeResponsesRole(t *testing.T) {
	cases := []struct{ role, want string }{
		{"assistant", "assistant"},
		{"user", "user"},
		{"system", "user"},
		{"Developer", "user"},
		{"", "user"},
	}
	for _, tc := range cases {
		if got := normalizeResponsesRole(tc.role); got != tc.want {
			t.Errorf("normalizeResponsesRole(%q) = %q, want %q", tc.role, got, tc.want)
		}
	}
}
