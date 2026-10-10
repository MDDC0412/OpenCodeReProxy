package protocol

// Characterization tests for the streaming half of the package: SSE framing
// (nextSSEBoundary / readSSE), the streaming parser state machine, the bridge
// emitter, the stream usage observer, rotation streams, and stream collapse.
//
// These lock in what the code does today. Where the behaviour looks
// surprising the test name says so; see the accompanying report.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// sxFailReader always fails, so the readSSE scanner error path can be reached.
type sxFailReader struct{ err error }

func (r *sxFailReader) Read([]byte) (int, error) { return 0, r.err }

// sxNoFlushWriter is an http.ResponseWriter that is not an http.Flusher.
type sxNoFlushWriter struct {
	header http.Header
	body   strings.Builder
}

func (w *sxNoFlushWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *sxNoFlushWriter) Write(p []byte) (int, error) { return w.body.Write(p) }
func (w *sxNoFlushWriter) WriteHeader(status int)      {}

// sxEmitter builds an emitter backed by a fresh recorder (a recorder is both
// an io.Writer and an http.Flusher).
func sxEmitter(target Protocol, model string) (*bridgeStreamEmitter, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	return newBridgeStreamEmitter(recorder, recorder, target, model), recorder
}

// sxDataLines returns every "data:" payload of an SSE body, in order.
func sxDataLines(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if payload, ok := strings.CutPrefix(line, "data:"); ok {
			out = append(out, strings.TrimSpace(payload))
		}
	}
	return out
}

// sxEventNames returns every "event:" name of an SSE body, in order.
func sxEventNames(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if name, ok := strings.CutPrefix(line, "event:"); ok {
			out = append(out, strings.TrimSpace(name))
		}
	}
	return out
}

// sxJSONAt decodes the i-th data payload of an SSE body.
func sxJSONAt(t *testing.T, body string, index int) map[string]any {
	t.Helper()
	lines := sxDataLines(body)
	if index >= len(lines) {
		t.Fatalf("data line %d out of range (have %d) in body:\n%s", index, len(lines), body)
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(lines[index]), &value); err != nil {
		t.Fatalf("data line %d is not JSON (%v): %s", index, err, lines[index])
	}
	return value
}

// sxFrame is one SSE record handed to the parser.
type sxFrame struct {
	name string
	data string
}

func sxData(payload string) sxFrame { return sxFrame{data: payload} }

// sxRender flattens a bridge stream event into a comparable one-line form.
func sxRender(event bridgeStreamEvent) string {
	parts := []string{event.Kind}
	if strings.HasPrefix(event.Kind, "tool_") {
		parts = append(parts, "key="+event.ToolKey)
	}
	if event.ResponseID != "" {
		parts = append(parts, "id="+event.ResponseID)
	}
	if event.Model != "" {
		parts = append(parts, "model="+event.Model)
	}
	if event.ToolID != "" {
		parts = append(parts, "tool="+event.ToolID)
	}
	if event.ToolName != "" {
		parts = append(parts, "name="+event.ToolName)
	}
	if event.Text != "" {
		parts = append(parts, "text="+event.Text)
	}
	if event.Signature != "" {
		parts = append(parts, "sig="+event.Signature)
	}
	if event.Encrypted != "" {
		parts = append(parts, "enc="+event.Encrypted)
	}
	if event.Stop != "" {
		parts = append(parts, "stop="+event.Stop)
	}
	if event.Error != "" {
		parts = append(parts, "err="+event.Error)
	}
	if event.ErrorType != "" {
		parts = append(parts, "type="+event.ErrorType)
	}
	if event.Usage != nil {
		parts = append(parts, fmt.Sprintf("usage=%d/%d/%d/%d/%d/%d",
			event.Usage.Input, event.Usage.Output, event.Usage.Total,
			event.Usage.Cached, event.Usage.CacheCreation, event.Usage.Reasoning))
	}
	return strings.Join(parts, " ")
}

// sxParseAll feeds frames through one shared parser and renders every event.
func sxParseAll(protocol Protocol, frames []sxFrame) ([]string, error) {
	parser := &bridgeStreamParser{
		protocol:          protocol,
		tools:             map[string]bool{},
		toolIDs:           map[string]string{},
		toolNames:         map[string]string{},
		responseArgs:      map[string]bool{},
		responseReasoning: map[string]bool{},
	}
	var out []string
	for _, frame := range frames {
		events, err := parser.Parse(frame.name, frame.data)
		if err != nil {
			return out, err
		}
		for _, event := range events {
			out = append(out, sxRender(event))
		}
	}
	return out, nil
}

func sxEqualStrings(t *testing.T, name string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d events %q, want %d events %q", name, len(got), got, len(want), want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: event %d = %q, want %q (all got %q)", name, i, got[i], want[i], got)
		}
	}
}

// ---------------------------------------------------------------------------
// nextSSEBoundary
// ---------------------------------------------------------------------------

func TestNextSSEBoundary(t *testing.T) {
	cases := []struct {
		name  string
		input string
		index int
		width int
	}{
		{"empty input", "", -1, 0},
		{"nil input", "", -1, 0},
		{"no boundary at all", "data: x", -1, 0},
		{"single trailing newline is not a boundary", "data: x\n", -1, 0},
		{"partial crlf at end of buffer", "data: x\r\n\r", -1, 0},
		{"one cr in a row is not a boundary", "\r\r", -1, 0},
		{"simple lf frame", "data: x\n\n", 7, 2},
		{"leading lf frame", "\n\nrest", 0, 2},
		{"partial frame followed by boundary", "data: a\ndata: b\n\n", 15, 2},
		{"crlf frame", "data: x\r\n\r\n", 7, 4},
		{"leading crlf frame", "\r\n\r\nrest", 0, 4},
		{"crlf boundary wins when earlier", "a\r\n\r\nb\n\n", 1, 4},
		{"earliest boundary wins", "a\n\nb\r\n\r\n", 1, 2},
		{"two consecutive crlf frames", "a\r\n\r\nb\r\n\r\n", 1, 4},
		{"mixed: lf frame then crlf frame", "a\n\nb\r\n\r\n", 1, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			index, width := nextSSEBoundary([]byte(tc.input))
			if index != tc.index || width != tc.width {
				t.Fatalf("nextSSEBoundary(%q) = (%d,%d), want (%d,%d)", tc.input, index, width, tc.index, tc.width)
			}
			if index >= 0 && index+width > len(tc.input) {
				t.Fatalf("boundary (%d,%d) runs past the end of %q", index, width, tc.input)
			}
		})
	}
}

// TestNextSSEBoundaryNilSlice asserts the nil and empty slices behave the same.
func TestNextSSEBoundaryNilSlice(t *testing.T) {
	if index, width := nextSSEBoundary(nil); index != -1 || width != 0 {
		t.Fatalf("nextSSEBoundary(nil) = (%d,%d), want (-1,0)", index, width)
	}
}

// TestNextSSEBoundaryFrameIsExactlyTheRecord documents that the returned slice
// always contains the terminating blank line.
func TestNextSSEBoundaryFrameIsExactlyTheRecord(t *testing.T) {
	input := "data: a\ndata: b\n\ndata: c\n\n"
	index, width := nextSSEBoundary([]byte(input))
	if got := input[:index+width]; got != "data: a\ndata: b\n\n" {
		t.Fatalf("frame = %q", got)
	}
	if rest := input[index+width:]; rest != "data: c\n\n" {
		t.Fatalf("remainder = %q", rest)
	}
}

// ---------------------------------------------------------------------------
// readSSE
// ---------------------------------------------------------------------------

func TestReadSSE(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantName  []string
		wantData  []string
		wantErrIs error
	}{
		{
			name:     "single data line",
			input:    "data: hello\n\n",
			wantName: []string{""}, wantData: []string{"hello"},
		},
		{
			name:     "multiple data lines join with a newline",
			input:    "data: a\ndata: b\ndata: c\n\n",
			wantName: []string{""}, wantData: []string{"a\nb\nc"},
		},
		{
			name:     "event name is captured",
			input:    "event: ping\ndata: a\n\n",
			wantName: []string{"ping"}, wantData: []string{"a"},
		},
		{
			name:     "event name is trimmed on both sides",
			input:    "event:   ping  \ndata: a\n\n",
			wantName: []string{"ping"}, wantData: []string{"a"},
		},
		{
			name:     "event name before data only",
			input:    "data: a\nevent: ping\n\n",
			wantName: []string{"ping"}, wantData: []string{"a"},
		},
		{
			name:     "comment lines are ignored",
			input:    ": keep-alive\ndata: a\n\n",
			wantName: []string{""}, wantData: []string{"a"},
		},
		{
			name:     "comment lines inside a record are ignored",
			input:    "data: a\n: note\ndata: b\n\n",
			wantName: []string{""}, wantData: []string{"a\nb"},
		},
		{
			name:     "crlf frames",
			input:    "data: a\r\n\r\n",
			wantName: []string{""}, wantData: []string{"a"},
		},
		{
			name:     "crlf multi line record",
			input:    "event: ping\r\ndata: a\r\ndata: b\r\n\r\n",
			wantName: []string{"ping"}, wantData: []string{"a\nb"},
		},
		{
			name:     "no space after the colon",
			input:    "data:a\n\n",
			wantName: []string{""}, wantData: []string{"a"},
		},
		{
			name:     "payload is trimmed",
			input:    "data:   a   \n\n",
			wantName: []string{""}, wantData: []string{"a"},
		},
		{
			name:     "unknown field names are ignored",
			input:    "id: 7\nretry: 3\ndata: a\n\n",
			wantName: []string{""}, wantData: []string{"a"},
		},
		{
			name:     "an event without data is dropped and resets the name",
			input:    "event: ping\n\n",
			wantName: nil, wantData: nil,
		},
		{
			name:     "event name does not leak into the next record",
			input:    "event: one\ndata: a\n\ndata: b\n\n",
			wantName: []string{"one", ""}, wantData: []string{"a", "b"},
		},
		{
			name:     "empty data value still counts as a record",
			input:    "data:\n\n",
			wantName: []string{""}, wantData: []string{""},
		},
		{
			name:     "only whitespace data value",
			input:    "data:    \n\n",
			wantName: []string{""}, wantData: []string{""},
		},
		{
			name:     "several frames",
			input:    "data: a\n\ndata: b\n\ndata: c\n\n",
			wantName: []string{"", "", ""}, wantData: []string{"a", "b", "c"},
		},
		{
			name:     "extra blank lines are harmless",
			input:    "data: a\n\n\n\ndata: b\n\n",
			wantName: []string{"", ""}, wantData: []string{"a", "b"},
		},
		{
			name:      "missing trailing blank line is a truncated record",
			input:     "data: a\n",
			wantName:  nil,
			wantData:  nil,
			wantErrIs: errSSEUnexpectedEOF,
		},
		{
			name:      "complete frame plus a truncated tail",
			input:     "data: a\n\ndata: b\n",
			wantName:  []string{""},
			wantData:  []string{"a"},
			wantErrIs: errSSEUnexpectedEOF,
		},
		{
			name:  "empty input",
			input: "",
		},
		{
			name:  "comments only",
			input: ": a\n: b\n\n",
		},
		{
			name:  "blank lines only",
			input: "\n\n\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var names, data []string
			err := readSSE(strings.NewReader(tc.input), func(eventName, payload string) error {
				names = append(names, eventName)
				data = append(data, payload)
				return nil
			})
			sxEqualStrings(t, "event names", names, tc.wantName)
			sxEqualStrings(t, "data", data, tc.wantData)
			if tc.wantErrIs == nil {
				if err != nil {
					t.Fatalf("readSSE error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErrIs) {
				t.Fatalf("readSSE error = %v, want %v", err, tc.wantErrIs)
			}
		})
	}
}

// TestReadSSEHandlerErrorStopsIteration documents that an error from the
// handler aborts the scan: later records are never delivered.
func TestReadSSEHandlerErrorStopsIteration(t *testing.T) {
	stop := errors.New("stop right here")
	for _, failOn := range []int{1, 2} {
		input := "data: a\n\ndata: b\n\ndata: c\n\n"
		var seen []string
		err := readSSE(strings.NewReader(input), func(_, payload string) error {
			seen = append(seen, payload)
			if len(seen) == failOn {
				return stop
			}
			return nil
		})
		if !errors.Is(err, stop) {
			t.Fatalf("failOn=%d: readSSE error = %v, want %v", failOn, err, stop)
		}
		if len(seen) != failOn {
			t.Fatalf("failOn=%d: handler saw %q, expected it to stop after %d records", failOn, seen, failOn)
		}
	}
}

// TestReadSSEPropagatesReaderError documents that a reader failure surfaces
// from readSSE instead of being reported as a truncated stream.
func TestReadSSEPropagatesReaderError(t *testing.T) {
	boom := errors.New("boom")
	calls := 0
	err := readSSE(&sxFailReader{err: boom}, func(_, _ string) error {
		calls++
		return nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("readSSE error = %v, want %v", err, boom)
	}
	if calls != 0 {
		t.Fatalf("handler called %d times, want 0", calls)
	}
}

// TestReadSSEBlankLineIsTheOnlyDelimiter records the deliberate deviation from
// the SSE spec: a final record without its blank line is treated as truncated.
func TestReadSSEBlankLineIsTheOnlyDelimiter(t *testing.T) {
	calls := 0
	err := readSSE(strings.NewReader("data: {\"a\":1}\n"), func(_, _ string) error {
		calls++
		return nil
	})
	if calls != 0 {
		t.Fatalf("handler called %d times, want 0", calls)
	}
	if !errors.Is(err, errSSEUnexpectedEOF) {
		t.Fatalf("readSSE error = %v, want errSSEUnexpectedEOF", err)
	}
}

// ---------------------------------------------------------------------------
// streamErrorMessage / streamErrorType / isStreamErrorFinish
// ---------------------------------------------------------------------------

func TestStreamErrorMessage(t *testing.T) {
	const fallback = "fallback text"
	cases := []struct {
		name  string
		value map[string]any
		want  string
	}{
		{"nested error message wins", map[string]any{"error": map[string]any{"message": "nested"}, "message": "top"}, "nested"},
		{"top level message", map[string]any{"message": "top"}, "top"},
		{"string error", map[string]any{"error": "flat"}, "flat"},
		{"a top level message beats a string error", map[string]any{"error": "flat", "message": "top"}, "top"},
		{"empty nested message falls back to top message", map[string]any{"error": map[string]any{"message": ""}, "message": "top"}, "top"},
		{"empty string error falls back", map[string]any{"error": ""}, fallback},
		{"nil error falls back", map[string]any{"error": nil}, fallback},
		{"empty object falls back", map[string]any{"error": map[string]any{}}, fallback},
		{"no error field falls back", map[string]any{}, fallback},
		{"numeric message is not a message", map[string]any{"message": 42}, fallback},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := streamErrorMessage(tc.value, fallback); got != tc.want {
				t.Fatalf("streamErrorMessage = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStreamErrorType(t *testing.T) {
	cases := []struct {
		name  string
		value map[string]any
		want  string
	}{
		{"error type", map[string]any{"error": map[string]any{"type": "rate_limit"}}, "rate_limit"},
		{"error code when type is missing", map[string]any{"error": map[string]any{"code": "bad_code"}}, "bad_code"},
		{"type wins over code", map[string]any{"error": map[string]any{"type": "t", "code": "c"}}, "t"},
		{"string error has no type", map[string]any{"error": "flat"}, "upstream_error"},
		{"nil error", map[string]any{"error": nil}, "upstream_error"},
		{"no error", map[string]any{}, "upstream_error"},
		{"blank type is skipped", map[string]any{"error": map[string]any{"type": "  ", "code": "c"}}, "c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := streamErrorType(tc.value); got != tc.want {
				t.Fatalf("streamErrorType = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsStreamErrorFinish(t *testing.T) {
	cases := []struct {
		stop string
		want bool
	}{
		{"error", true},
		{"ERROR", true},
		{"network_error", true},
		{"Network_Error", true},
		{"  server_error  ", true},
		{"stop", false},
		{"stop_sequence", false},
		{"tool_calls", false},
		{"tool_use", false},
		{"max_tokens", false},
		{"length", false},
		{"refusal", false},
		{"content_filter", false},
		{"networkerror", false},
		{"errored", false},
		{"", false},
		{"   ", false},
	}
	for _, tc := range cases {
		t.Run("stop="+tc.stop, func(t *testing.T) {
			if got := isStreamErrorFinish(tc.stop); got != tc.want {
				t.Fatalf("isStreamErrorFinish(%q) = %v, want %v", tc.stop, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// mergeToolName
// ---------------------------------------------------------------------------

func TestMergeToolName(t *testing.T) {
	cases := []struct {
		name     string
		current  string
		fragment string
		want     string
	}{
		{"first fragment", "", "get", "get"},
		{"append a continuation fragment", "get_", "weather", "get_weather"},
		{"repeated full name is idempotent", "get_weather", "get_weather", "get_weather"},
		{"growing prefix is replaced not appended", "ge", "get", "get"},
		{"empty fragment keeps the current name", "get", "", "get"},
		{"empty into empty stays empty", "", "", ""},
		{"empty fragment after an empty name", "", "", ""},
		{"fragment equal to current", "f", "f", "f"},
		{"different name is appended", "search", "query", "searchquery"},
		{"name repeated after a prefix extension", "get", "get_weather", "get_weather"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mergeToolName(tc.current, tc.fragment); got != tc.want {
				t.Fatalf("mergeToolName(%q,%q) = %q, want %q", tc.current, tc.fragment, got, tc.want)
			}
		})
	}
}

// TestMergeToolNameStreamSequence replays the fragment stream an upstream
// sends: repeated names and interleaved empty fragments.
func TestMergeToolNameStreamSequence(t *testing.T) {
	fragments := []string{"get_", "get_", "", "weather", "", "get_weather"}
	want := []string{"get_", "get_", "get_", "get_weather", "get_weather", "get_weather"}
	current := ""
	for i, fragment := range fragments {
		current = mergeToolName(current, fragment)
		if current != want[i] {
			t.Fatalf("after fragment %q (step %d) = %q, want %q", fragment, i, current, want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// mergeBridgeUsage
// ---------------------------------------------------------------------------

func TestMergeBridgeUsage(t *testing.T) {
	cases := []struct {
		name    string
		initial Usage
		steps   []Usage
		want    Usage
	}{
		{
			name: "empty into empty",
			want: Usage{},
		},
		{
			name:  "single payload derives the total",
			steps: []Usage{{Input: 3, Output: 4}},
			want:  Usage{Input: 3, Output: 4, Total: 7},
		},
		{
			// The total is only recomputed while a payload is merged, never on
			// its own, so it stays zero until the first delta arrives.
			name:    "a zero payload cannot reset an accumulated one",
			initial: Usage{Input: 7, Output: 3},
			steps:   []Usage{{}},
			want:    Usage{Input: 7, Output: 3, Total: 10},
		},
		{
			name:    "a total is not derived without a merge",
			initial: Usage{Input: 7, Output: 3},
			want:    Usage{Input: 7, Output: 3},
		},
		{
			name:    "an explicit larger total survives",
			initial: Usage{Input: 3, Output: 4, Total: 100},
			steps:   []Usage{{}},
			want:    Usage{Input: 3, Output: 4, Total: 100},
		},
		{
			name:    "a smaller total never lowers the total",
			initial: Usage{Input: 3, Output: 4, Total: 100},
			steps:   []Usage{{Total: 5}},
			want:    Usage{Input: 3, Output: 4, Total: 100},
		},
		{
			name:    "a larger total raises it",
			initial: Usage{Input: 3, Output: 4, Total: 10},
			steps:   []Usage{{Total: 99}},
			want:    Usage{Input: 3, Output: 4, Total: 99},
		},
		{
			name:  "later output deltas grow the total past the reported one",
			steps: []Usage{{Input: 10, Total: 10}, {Output: 5}},
			want:  Usage{Input: 10, Output: 5, Total: 15},
		},
		{
			name:  "input is last non zero wins",
			steps: []Usage{{Input: 10}, {Input: 25}},
			want:  Usage{Input: 25, Total: 25},
		},
		{
			name:  "cached is last non zero wins",
			steps: []Usage{{Input: 5, Cached: 5}, {Cached: 0}, {Cached: 2}},
			want:  Usage{Input: 5, Total: 5, Cached: 2},
		},
		{
			name:  "cache creation and reasoning accumulate across deltas",
			steps: []Usage{{Input: 4, Cached: 2, CacheCreation: 1}, {Output: 6, Reasoning: 3}},
			want:  Usage{Input: 4, Output: 6, Total: 10, Cached: 2, CacheCreation: 1, Reasoning: 3},
		},
		{
			name:  "reasoning output tokens accumulate",
			steps: []Usage{{Output: 2, Reasoning: 2}, {Output: 4, Reasoning: 4}},
			want:  Usage{Output: 4, Total: 4, Reasoning: 4},
		},
		{
			name:  "anthropic message_delta usage carries only the output",
			steps: []Usage{{Input: 4, Output: 0, Total: 4}, {Output: 7}},
			want:  Usage{Input: 4, Output: 7, Total: 11},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.initial
			for _, step := range tc.steps {
				mergeBridgeUsage(&got, step)
			}
			if got != tc.want {
				t.Fatalf("mergeBridgeUsage result = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// bridgeStreamParser: dispatch level
// ---------------------------------------------------------------------------

func TestParserParseErrors(t *testing.T) {
	cases := []struct {
		name     string
		protocol Protocol
		data     string
		contains string
	}{
		{"done terminator for chat", Chat, "[DONE]", ""},
		{"done terminator for responses", Responses, "[DONE]", "unexpected [DONE] terminator for responses stream"},
		{"done terminator for anthropic", Anthropic, "[DONE]", "unexpected [DONE] terminator for anthropic stream"},
		{"invalid json", Chat, "{oops", "invalid upstream SSE JSON"},
		{"empty payload", Chat, "", "invalid upstream SSE JSON"},
		{"non object json", Chat, "[1,2]", "invalid upstream SSE JSON"},
		{"unsupported protocol", SystemOne, `{"a":1}`, `unsupported stream protocol "systemone"`},
		{"unknown protocol", Protocol("bogus"), `{"a":1}`, `unsupported stream protocol "bogus"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parser := &bridgeStreamParser{
				protocol:          tc.protocol,
				tools:             map[string]bool{},
				toolIDs:           map[string]string{},
				toolNames:         map[string]string{},
				responseArgs:      map[string]bool{},
				responseReasoning: map[string]bool{},
			}
			events, err := parser.Parse("", tc.data)
			if tc.contains == "" {
				if err != nil {
					t.Fatalf("Parse error = %v, want nil", err)
				}
				sxEqualStrings(t, "events", []string{sxRender(events[0])}, []string{"done"})
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("Parse error = %v, want it to contain %q", err, tc.contains)
			}
			if events != nil {
				t.Fatalf("Parse returned events %v alongside an error", events)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// bridgeStreamParser: Chat
// ---------------------------------------------------------------------------

func TestParserChatEvents(t *testing.T) {
	cases := []struct {
		name   string
		frames []sxFrame
		want   []string
	}{
		{
			name:   "a role only chunk carries no id and produces nothing",
			frames: []sxFrame{sxData(`{"choices":[{"delta":{"role":"assistant","content":""}}]}`)},
			want:   nil,
		},
		{
			name: "the start event is emitted at most once",
			frames: []sxFrame{
				sxData(`{"id":"chat_1","model":"m1","choices":[]}`),
				sxData(`{"id":"chat_2","model":"m2","choices":[]}`),
			},
			want: []string{"start id=chat_1 model=m1"},
		},
		{
			name:   "reasoning is emitted before text",
			frames: []sxFrame{sxData(`{"choices":[{"delta":{"reasoning_content":"think","content":"hi"}}]}`)},
			want:   []string{"reasoning text=think", "text text=hi"},
		},
		{
			name:   "the reasoning alias is accepted",
			frames: []sxFrame{sxData(`{"choices":[{"delta":{"reasoning":"alias"}}]}`)},
			want:   []string{"reasoning text=alias"},
		},
		{
			name:   "empty deltas are dropped",
			frames: []sxFrame{sxData(`{"choices":[{"delta":{"content":"","reasoning_content":""}}]}`)},
			want:   nil,
		},
		{
			name:   "usage is decoded into a usage event",
			frames: []sxFrame{sxData(`{"usage":{"prompt_tokens":2,"completion_tokens":3}}`)},
			want:   []string{"usage usage=2/3/5/0/0/0"},
		},
		{
			name: "the error event name wins over a payload without an error",
			frames: []sxFrame{
				{name: "error", data: `{"message":"boom"}`},
			},
			want: []string{"error err=boom type=upstream_error"},
		},
		{
			name:   "an error object with a code becomes the error type",
			frames: []sxFrame{sxData(`{"error":{"message":"boom","code":"bad_code"}}`)},
			want:   []string{"error err=boom type=bad_code"},
		},
		{
			name:   "an empty error object uses the fallback message",
			frames: []sxFrame{sxData(`{"error":{}}`)},
			want:   []string{"error err=upstream Chat stream error type=upstream_error"},
		},
		{
			name:   "an error finish reason suppresses the finish event",
			frames: []sxFrame{sxData(`{"choices":[{"finish_reason":"error","error":{"message":"kaput"}}]}`)},
			want:   []string{"error err=kaput type=upstream_error"},
		},
		{
			name: "an error finish reason still reports the usage that arrived with it",
			frames: []sxFrame{
				sxData(`{"usage":{"prompt_tokens":5},"choices":[{"finish_reason":"server_error"}]}`),
			},
			want: []string{"usage usage=5/0/5/0/0/0", "error err=upstream Chat stream failed type=upstream_error"},
		},
		{
			name: "tool name fragments assemble before the first argument delta",
			frames: []sxFrame{
				sxData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"get_"}}]}}]}`),
				sxData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"weather"}}]}}]}`),
				sxData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`),
				sxData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"NY\"}"}}]}}]}`),
			},
			want: []string{
				"tool_start key=0 tool=call_a name=get_weather",
				"tool_delta key=0 tool=call_a name=get_weather text={\"city\":",
				"tool_delta key=0 tool=call_a name=get_weather text=\"NY\"}",
			},
		},
		{
			name: "repeated identical argument deltas are not de-duplicated",
			frames: []sxFrame{
				sxData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"f","arguments":"{}"}}]}}]}`),
				sxData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}`),
			},
			want: []string{
				"tool_start key=0 name=f",
				"tool_delta key=0 name=f text={}",
				"tool_delta key=0 name=f text={}",
			},
		},
		{
			name: "empty name fragments leave the accumulated name alone",
			frames: []sxFrame{
				sxData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":""}}]}}]}`),
				sxData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"f"}}]}}]}`),
				sxData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}`),
			},
			want: []string{"tool_start key=0 name=f", "tool_delta key=0 name=f text={}"},
		},
		{
			name: "a tool call without an index falls back to its id as the key",
			frames: []sxFrame{
				sxData(`{"choices":[{"delta":{"tool_calls":[{"id":"call_x","function":{"name":"f","arguments":"{}"}}]}}]}`),
			},
			want: []string{"tool_start key=call_x tool=call_x name=f", "tool_delta key=call_x tool=call_x name=f text={}"},
		},
		{
			name: "a tool call without index or id gets the empty key",
			frames: []sxFrame{
				sxData(`{"choices":[{"delta":{"tool_calls":[{"function":{"name":"f","arguments":"{}"}}]}}]}`),
			},
			want: []string{"tool_start key= name=f", "tool_delta key= name=f text={}"},
		},
		{
			name: "a named tool without arguments is flushed at the finish reason",
			frames: []sxFrame{
				sxData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"get_weather"}}]}}]}`),
				sxData(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`),
			},
			want: []string{"tool_start key=0 name=get_weather", "finish stop=tool_calls"},
		},
		{
			name: "an unnamed tool with arguments stays a phantom",
			frames: []sxFrame{
				sxData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_z","function":{"arguments":"{}"}}]}}]}`),
				sxData(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`),
			},
			want: []string{"tool_delta key=0 tool=call_z text={}", "finish stop=tool_calls"},
		},
		{
			name: "index 0 is a usable key",
			frames: []sxFrame{
				sxData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"f","arguments":"{}"}}]}}]}`),
			},
			want: []string{"tool_start key=0 name=f", "tool_delta key=0 name=f text={}"},
		},
		{
			name: "several choices are flattened in order",
			frames: []sxFrame{
				sxData(`{"choices":[{"delta":{"content":"a"}},{"delta":{"content":"b"}}]}`),
			},
			want: []string{"text text=a", "text text=b"},
		},
		{
			name: "a plain stop emits only the finish event",
			frames: []sxFrame{
				sxData(`{"choices":[{"delta":{"content":"hi"},"finish_reason":"stop"}]}`),
			},
			want: []string{"text text=hi", "finish stop=stop"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sxParseAll(Chat, tc.frames)
			if err != nil {
				t.Fatalf("Parse error = %v", err)
			}
			sxEqualStrings(t, "events", got, tc.want)
		})
	}
}

// TestParserChatSeparatesParallelToolCalls locks in that tool bookkeeping is
// keyed per call index.
func TestParserChatSeparatesParallelToolCalls(t *testing.T) {
	frames := []sxFrame{
		sxData(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_0","function":{"name":"alpha","arguments":"{}"}},{"index":1,"id":"call_1","function":{"name":"beta","arguments":"[]"}}]}}]}`),
		sxData(`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"[1]"}}]}}]}`),
	}
	got, err := sxParseAll(Chat, frames)
	if err != nil {
		t.Fatalf("Parse error = %v", err)
	}
	sxEqualStrings(t, "events", got, []string{
		"tool_start key=0 tool=call_0 name=alpha",
		"tool_delta key=0 tool=call_0 name=alpha text={}",
		"tool_start key=1 tool=call_1 name=beta",
		"tool_delta key=1 tool=call_1 name=beta text=[]",
		"tool_delta key=1 tool=call_1 name=beta text=[1]",
	})
}

// ---------------------------------------------------------------------------
// bridgeStreamParser: Anthropic
// ---------------------------------------------------------------------------

func TestParserAnthropicEvents(t *testing.T) {
	cases := []struct {
		name   string
		frames []sxFrame
		want   []string
	}{
		{
			name:   "message_start with usage",
			frames: []sxFrame{sxData(`{"type":"message_start","message":{"id":"msg_1","model":"m","usage":{"input_tokens":4}}}`)},
			want:   []string{"start id=msg_1 model=m", "usage usage=4/0/4/0/0/0"},
		},
		{
			name:   "message_start without a usage block",
			frames: []sxFrame{sxData(`{"type":"message_start","message":{"id":"msg_1","model":"m"}}`)},
			want:   []string{"start id=msg_1 model=m"},
		},
		{
			name:   "the event name is used when the payload has no type",
			frames: []sxFrame{{name: "message_start", data: `{"message":{}}`}},
			want:   []string{"start"},
		},
		{
			name:   "text delta",
			frames: []sxFrame{sxData(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`)},
			want:   []string{"text text=hi"},
		},
		{
			name:   "text block start carries inline text",
			frames: []sxFrame{sxData(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"inline"}}`)},
			want:   []string{"text text=inline"},
		},
		{
			name:   "an empty text block start produces nothing",
			frames: []sxFrame{sxData(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)},
			want:   nil,
		},
		{
			name:   "a thinking block emits reasoning then the signature",
			frames: []sxFrame{sxData(`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"t","signature":"sig"}}`)},
			want:   []string{"reasoning text=t", "reasoning_signature sig=sig"},
		},
		{
			name:   "a signature without thinking text",
			frames: []sxFrame{sxData(`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","signature":"sig"}}`)},
			want:   []string{"reasoning_signature sig=sig"},
		},
		{
			name:   "redacted thinking carries encrypted content",
			frames: []sxFrame{sxData(`{"type":"content_block_start","index":0,"content_block":{"type":"redacted_thinking","data":"ENC"}}`)},
			want:   []string{"reasoning enc=ENC"},
		},
		{
			name:   "thinking delta",
			frames: []sxFrame{sxData(`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"deep"}}`)},
			want:   []string{"reasoning text=deep"},
		},
		{
			name:   "signature delta",
			frames: []sxFrame{sxData(`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"abc"}}`)},
			want:   []string{"reasoning_signature sig=abc"},
		},
		{
			name:   "tool_use block start is keyed by the block index",
			frames: []sxFrame{sxData(`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"f"}}`)},
			want:   []string{"tool_start key=1 tool=toolu_1 name=f"},
		},
		{
			name:   "input json delta reuses the same key without a name",
			frames: []sxFrame{sxData(`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"a\":"}}`)},
			want:   []string{"tool_delta key=1 text={\"a\":"},
		},
		{
			name:   "message_delta reports usage before the finish",
			frames: []sxFrame{sxData(`{"type":"message_delta","usage":{"output_tokens":5},"delta":{"stop_reason":"end_turn"}}`)},
			want:   []string{"usage usage=0/5/5/0/0/0", "finish stop=end_turn"},
		},
		{
			name:   "message_delta without a stop reason still reports usage",
			frames: []sxFrame{sxData(`{"type":"message_delta","usage":{"output_tokens":5}}`)},
			want:   []string{"usage usage=0/5/5/0/0/0"},
		},
		{
			name:   "an error stop reason becomes an error event",
			frames: []sxFrame{sxData(`{"type":"message_delta","delta":{"stop_reason":"error"}}`)},
			want:   []string{"error err=upstream Anthropic stream failed type=upstream_error"},
		},
		{
			name:   "an error stop reason still reports usage first",
			frames: []sxFrame{sxData(`{"type":"message_delta","usage":{"output_tokens":2},"delta":{"stop_reason":"network_error"}}`)},
			want:   []string{"usage usage=0/2/2/0/0/0", "error err=upstream Anthropic stream failed type=upstream_error"},
		},
		{
			name:   "message_stop",
			frames: []sxFrame{sxData(`{"type":"message_stop"}`)},
			want:   []string{"done"},
		},
		{
			name:   "error event",
			frames: []sxFrame{sxData(`{"type":"error","error":{"message":"bad","type":"overloaded_error"}}`)},
			want:   []string{"error err=bad type=overloaded_error"},
		},
		{
			name:   "error event without a message falls back",
			frames: []sxFrame{sxData(`{"type":"error","error":{"type":"overloaded_error"}}`)},
			want:   []string{"error err=upstream Anthropic stream error type=overloaded_error"},
		},
		{
			name:   "an unknown delta type is ignored",
			frames: []sxFrame{sxData(`{"type":"content_block_delta","index":0,"delta":{"type":"mystery"}}`)},
			want:   nil,
		},
		{
			name:   "an unknown event type is ignored",
			frames: []sxFrame{sxData(`{"type":"ping"}`)},
			want:   nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sxParseAll(Anthropic, tc.frames)
			if err != nil {
				t.Fatalf("Parse error = %v", err)
			}
			sxEqualStrings(t, "events", got, tc.want)
		})
	}
}

// ---------------------------------------------------------------------------
// bridgeStreamParser: Responses
// ---------------------------------------------------------------------------

func TestParserResponsesEvents(t *testing.T) {
	cases := []struct {
		name   string
		frames []sxFrame
		want   []string
	}{
		{
			name:   "response.created",
			frames: []sxFrame{sxData(`{"type":"response.created","response":{"id":"resp_1","model":"m"}}`)},
			want:   []string{"start id=resp_1 model=m"},
		},
		{
			name:   "the event name is used when the payload has no type",
			frames: []sxFrame{{name: "response.created", data: `{"response":{"id":"resp_9"}}`}},
			want:   []string{"start id=resp_9"},
		},
		{
			name:   "output text delta",
			frames: []sxFrame{sxData(`{"type":"response.output_text.delta","item_id":"msg_1","delta":"hi"}`)},
			want:   []string{"text text=hi"},
		},
		{
			name:   "reasoning summary delta",
			frames: []sxFrame{sxData(`{"type":"response.reasoning_summary_text.delta","item_id":"rs_1","delta":"think"}`)},
			want:   []string{"reasoning text=think"},
		},
		{
			name: "a repeated reasoning item is dropped on output_item.done",
			frames: []sxFrame{
				sxData(`{"type":"response.reasoning_summary_text.delta","item_id":"rs_1","delta":"think"}`),
				sxData(`{"type":"response.output_item.done","output_index":0,"item":{"id":"rs_1","type":"reasoning","summary":[{"type":"summary_text","text":"think"}]}}`),
			},
			want: []string{"reasoning text=think"},
		},
		{
			name: "an unseen reasoning item is recovered from the summary",
			frames: []sxFrame{
				sxData(`{"type":"response.output_item.done","output_index":0,"item":{"id":"rs_2","type":"reasoning","summary":[{"type":"summary_text","text":"later"}]}}`),
			},
			want: []string{"reasoning text=later"},
		},
		{
			name: "an empty reasoning item produces nothing",
			frames: []sxFrame{
				sxData(`{"type":"response.output_item.done","output_index":0,"item":{"id":"rs_3","type":"reasoning"}}`),
			},
			want: nil,
		},
		{
			name:   "output_item.added starts a function call",
			frames: []sxFrame{sxData(`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"f"}}`)},
			want:   []string{"tool_start key=fc_1 tool=call_1 name=f"},
		},
		{
			name: "argument deltas reuse the remembered identity",
			frames: []sxFrame{
				sxData(`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"f"}}`),
				sxData(`{"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":0,"delta":"{\"a\":"}`),
				sxData(`{"type":"response.function_call_arguments.delta","item_id":"fc_1","output_index":0,"delta":"1}"}`),
			},
			want: []string{
				"tool_start key=fc_1 tool=call_1 name=f",
				"tool_delta key=fc_1 tool=call_1 name=f text={\"a\":",
				"tool_delta key=fc_1 tool=call_1 name=f text=1}",
			},
		},
		{
			name: "arguments.done after deltas is suppressed",
			frames: []sxFrame{
				sxData(`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"f"}}`),
				sxData(`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{}"}`),
				sxData(`{"type":"response.function_call_arguments.done","item_id":"fc_1","arguments":"{\"full\":true}"}`),
			},
			want: []string{
				"tool_start key=fc_1 tool=call_1 name=f",
				"tool_delta key=fc_1 tool=call_1 name=f text={}",
			},
		},
		{
			name: "arguments.done without deltas emits the final arguments",
			frames: []sxFrame{
				sxData(`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"f"}}`),
				sxData(`{"type":"response.function_call_arguments.done","item_id":"fc_1","arguments":"{\"full\":true}"}`),
			},
			want: []string{
				"tool_start key=fc_1 tool=call_1 name=f",
				"tool_delta key=fc_1 tool=call_1 name=f text={\"full\":true}",
			},
		},
		{
			name: "output_item.done starts a tool and replays its arguments",
			frames: []sxFrame{
				sxData(`{"type":"response.output_item.done","output_index":0,"item":{"id":"fc_9","type":"function_call","call_id":"call_9","name":"g","arguments":"{\"x\":1}"}}`),
			},
			want: []string{
				"tool_start key=fc_9 tool=call_9 name=g",
				"tool_delta key=fc_9 text={\"x\":1}",
			},
		},
		{
			name: "output_item.done after deltas only starts the tool",
			frames: []sxFrame{
				sxData(`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_9","type":"function_call","call_id":"call_9","name":"g"}}`),
				sxData(`{"type":"response.function_call_arguments.delta","item_id":"fc_9","delta":"{}"}`),
				sxData(`{"type":"response.output_item.done","output_index":0,"item":{"id":"fc_9","type":"function_call","call_id":"call_9","name":"g","arguments":"{}"}}`),
			},
			want: []string{
				"tool_start key=fc_9 tool=call_9 name=g",
				"tool_delta key=fc_9 tool=call_9 name=g text={}",
			},
		},
		{
			name: "a non function_call item is ignored",
			frames: []sxFrame{
				sxData(`{"type":"response.output_item.added","output_index":0,"item":{"id":"msg_1","type":"message"}}`),
			},
			want: nil,
		},
		{
			name:   "response.completed without tools",
			frames: []sxFrame{sxData(`{"type":"response.completed","response":{"id":"resp_1"}}`)},
			want:   []string{"finish stop=stop", "done"},
		},
		{
			name:   "response.completed reports usage first",
			frames: []sxFrame{sxData(`{"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":4}}}`)},
			want:   []string{"usage usage=3/4/7/0/0/0", "finish stop=stop", "done"},
		},
		{
			name: "response.completed with a function call output is a tool turn",
			frames: []sxFrame{
				sxData(`{"type":"response.completed","response":{"output":[{"type":"function_call","name":"f"}]}}`),
			},
			want: []string{"finish stop=tool_calls", "done"},
		},
		{
			name: "an observed tool makes the completed turn a tool turn",
			frames: []sxFrame{
				sxData(`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"f"}}`),
				sxData(`{"type":"response.completed","response":{"output":[]}}`),
			},
			want: []string{"tool_start key=fc_1 tool=call_1 name=f", "finish stop=tool_calls", "done"},
		},
		{
			name:   "response.done behaves like response.completed",
			frames: []sxFrame{sxData(`{"type":"response.done","response":{}}`)},
			want:   []string{"finish stop=stop", "done"},
		},
		{
			name:   "response.incomplete defaults to length",
			frames: []sxFrame{sxData(`{"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"}}}`)},
			want:   []string{"finish stop=length", "done"},
		},
		{
			name:   "response.incomplete keeps content_filter",
			frames: []sxFrame{sxData(`{"type":"response.incomplete","response":{"incomplete_details":{"reason":"content_filter"}}}`)},
			want:   []string{"finish stop=content_filter", "done"},
		},
		{
			name:   "response.failed is an error with its code as type",
			frames: []sxFrame{sxData(`{"type":"response.failed","response":{"error":{"message":"nope","code":"server_error"}}}`)},
			want:   []string{"error err=nope type=server_error"},
		},
		{
			name:   "response.failed without a message uses the fallback",
			frames: []sxFrame{sxData(`{"type":"response.failed","response":{}}`)},
			want:   []string{"error err=upstream Responses request failed type=upstream_error"},
		},
		{
			name:   "response.failed reports usage before the error",
			frames: []sxFrame{sxData(`{"type":"response.failed","response":{"usage":{"input_tokens":1},"error":{"message":"nope"}}}`)},
			want:   []string{"usage usage=1/0/1/0/0/0", "error err=nope type=upstream_error"},
		},
		{
			name:   "a failed status wins over the event name",
			frames: []sxFrame{sxData(`{"type":"response.completed","response":{"status":"failed","error":{"message":"dead"}}}`)},
			want:   []string{"error err=dead type=upstream_error"},
		},
		{
			name:   "an error event name is honoured",
			frames: []sxFrame{{name: "error", data: `{"message":"by name"}`}},
			want:   []string{"error err=by name type=upstream_error"},
		},
		{
			name:   "an unknown event is ignored",
			frames: []sxFrame{sxData(`{"type":"response.mystery"}`)},
			want:   nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sxParseAll(Responses, tc.frames)
			if err != nil {
				t.Fatalf("Parse error = %v", err)
			}
			sxEqualStrings(t, "events", got, tc.want)
		})
	}
}

func TestResponseToolKey(t *testing.T) {
	cases := []struct {
		name  string
		event map[string]any
		item  map[string]any
		want  string
	}{
		{"the item id wins", map[string]any{"item_id": "ev"}, map[string]any{"id": "fc_1"}, "fc_1"},
		{"the item id wins over the output index", map[string]any{"output_index": 7}, map[string]any{"id": "fc_1"}, "fc_1"},
		{"the event item id is next", map[string]any{"item_id": "ev", "output_index": 7}, nil, "ev"},
		{"the output index is the last resort", map[string]any{"output_index": 7}, nil, "7"},
		{"no output index renders as nil", map[string]any{}, nil, "<nil>"},
		{"an item without an id falls through", map[string]any{"item_id": "ev"}, map[string]any{"type": "reasoning"}, "ev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := responseToolKey(tc.event, tc.item); got != tc.want {
				t.Fatalf("responseToolKey = %q, want %q", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// streamUsageObserver
// ---------------------------------------------------------------------------

func TestStreamUsageObserverCollectsFrames(t *testing.T) {
	observer := newStreamUsageObserver(Chat)
	chunks := []string{
		"data: {\"usage\":{\"prompt_tokens\":5}}\n",
		"\n",
		"data: {\"usage\":{\"completion_tokens\":2,\"completion_tokens_details\":{\"reasoning_tokens\":1}}}\n\n",
		"data: [DONE]\n\n",
	}
	for _, chunk := range chunks {
		n, err := observer.Write([]byte(chunk))
		if err != nil {
			t.Fatalf("Write(%q) error = %v", chunk, err)
		}
		if n != len(chunk) {
			t.Fatalf("Write(%q) = %d, want %d", chunk, n, len(chunk))
		}
	}
	if !observer.Reported() {
		t.Fatal("Reported() = false, want true after a usage frame")
	}
	if !observer.NormalTermination() {
		t.Fatal("NormalTermination() = false, want true after [DONE]")
	}
	if observer.ErrorTermination() {
		t.Fatal("ErrorTermination() = true, want false")
	}
	if observer.ParseError() != nil {
		t.Fatalf("ParseError() = %v, want nil", observer.ParseError())
	}
	if got := observer.Finish(); got != (Usage{Input: 5, Output: 2, Total: 7, Reasoning: 1}) {
		t.Fatalf("Finish() = %+v", got)
	}
}

// TestStreamUsageObserverDropsTruncatedTail locks in that a record without its
// blank line is never counted, even when it carries usage.
func TestStreamUsageObserverDropsTruncatedTail(t *testing.T) {
	observer := newStreamUsageObserver(Chat)
	if _, err := observer.Write([]byte("data: {\"usage\":{\"prompt_tokens\":9}}")); err != nil {
		t.Fatalf("Write error = %v", err)
	}
	if observer.Reported() {
		t.Fatal("Reported() = true before the frame is complete")
	}
	if got := observer.Finish(); got != (Usage{}) {
		t.Fatalf("Finish() = %+v, want the zero usage", got)
	}
	if observer.Reported() {
		t.Fatal("Reported() = true, want false")
	}
}

func TestStreamUsageObserverTerminationFlags(t *testing.T) {
	t.Run("error frame sets error termination", func(t *testing.T) {
		observer := newStreamUsageObserver(Chat)
		if _, err := observer.Write([]byte("data: {\"error\":{\"message\":\"boom\",\"type\":\"upstream_error\"}}\n\n")); err != nil {
			t.Fatalf("Write error = %v", err)
		}
		if !observer.ErrorTermination() {
			t.Fatal("ErrorTermination() = false, want true")
		}
		if observer.NormalTermination() {
			t.Fatal("NormalTermination() = true, want false")
		}
	})
	t.Run("an error finish reason sets error termination", func(t *testing.T) {
		observer := newStreamUsageObserver(Chat)
		if _, err := observer.Write([]byte("data: {\"choices\":[{\"finish_reason\":\"error\"}]}\n\n")); err != nil {
			t.Fatalf("Write error = %v", err)
		}
		if !observer.ErrorTermination() {
			t.Fatal("ErrorTermination() = false, want true")
		}
	})
	t.Run("a parse error is recorded and later frames are still consumed", func(t *testing.T) {
		observer := newStreamUsageObserver(Chat)
		input := "data: {broken\n\ndata: {\"usage\":{\"prompt_tokens\":4}}\n\n"
		if _, err := observer.Write([]byte(input)); err != nil {
			t.Fatalf("Write error = %v", err)
		}
		if observer.ParseError() == nil {
			t.Fatal("ParseError() = nil, want a JSON error")
		}
		if !observer.Reported() {
			t.Fatal("Reported() = false, want the frame after the bad one to count")
		}
		if got := observer.Finish(); got.Input != 4 {
			t.Fatalf("Finish() = %+v, want Input 4", got)
		}
	})
	t.Run("only the first parse error is kept", func(t *testing.T) {
		observer := newStreamUsageObserver(Chat)
		if _, err := observer.Write([]byte("data: {aaa\n\ndata: {zzz\n\n")); err != nil {
			t.Fatalf("Write error = %v", err)
		}
		// The JSON error only names the offending character, so the two
		// failures are told apart by that.
		if observer.ParseError() == nil || !strings.Contains(observer.ParseError().Error(), "'a'") {
			t.Fatalf("ParseError() = %v, want the first failure", observer.ParseError())
		}
	})
	t.Run("an empty stream reports nothing", func(t *testing.T) {
		observer := newStreamUsageObserver(Chat)
		if got := observer.Finish(); got != (Usage{}) {
			t.Fatalf("Finish() = %+v", got)
		}
		if observer.Reported() || observer.NormalTermination() || observer.ErrorTermination() {
			t.Fatal("an empty stream reported termination or usage")
		}
	})
}

// TestStreamUsageObserverFrameBoundaries documents that several frames in one
// Write are all consumed and the leftover bytes stay buffered.
func TestStreamUsageObserverFrameBoundaries(t *testing.T) {
	observer := newStreamUsageObserver(Chat)
	input := "data: {\"usage\":{\"prompt_tokens\":1}}\n\ndata: {\"usage\":{\"prompt_tokens\":2}}\n\ndata: {\"usage\":{\"prompt_"
	n, err := observer.Write([]byte(input))
	if err != nil || n != len(input) {
		t.Fatalf("Write = (%d,%v)", n, err)
	}
	if got := observer.Finish(); got.Input != 2 {
		t.Fatalf("Finish() = %+v, want Input 2", got)
	}
}

// ---------------------------------------------------------------------------
// emitUnexpectedStreamError
// ---------------------------------------------------------------------------

func TestEmitUnexpectedStreamError(t *testing.T) {
	cases := []struct {
		name    string
		cause   error
		message string
	}{
		{"no cause", nil, "upstream SSE stream ended before a terminal event"},
		{"the sentinel cause", errSSEUnexpectedEOF, "upstream SSE stream ended before a terminal event"},
		{"wrapped sentinel cause", fmt.Errorf("read: %w", errSSEUnexpectedEOF), "upstream SSE stream ended before a terminal event"},
		{"a real cause", errors.New("boom"), "upstream SSE stream failed: boom"},
		{"a normalized termination is a real cause", errStreamNormalTermination, "upstream SSE stream failed: upstream stream terminated normally"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			emitter, recorder := sxEmitter(Chat, "m")
			if err := emitUnexpectedStreamError(emitter, tc.cause); err != nil {
				t.Fatalf("emitUnexpectedStreamError error = %v, want nil (the upstream failure is swallowed)", err)
			}
			if !strings.Contains(recorder.Body.String(), tc.message) {
				t.Fatalf("body = %q, want it to contain %q", recorder.Body.String(), tc.message)
			}
			if !emitter.done {
				t.Fatal("emitter.done = false, want the error to mark the stream finished")
			}
			if !strings.Contains(recorder.Body.String(), "upstream_error") {
				t.Fatalf("body = %q, want the upstream_error type", recorder.Body.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// bridgeStreamEmitter
// ---------------------------------------------------------------------------

func TestNewBridgeStreamEmitter(t *testing.T) {
	emitter, recorder := sxEmitter(Chat, "alias-model")
	if emitter.target != Chat {
		t.Fatalf("target = %q, want chat", emitter.target)
	}
	if emitter.model != "alias-model" {
		t.Fatalf("model = %q, want alias-model", emitter.model)
	}
	if emitter.created <= 0 {
		t.Fatalf("created = %d, want a unix timestamp", emitter.created)
	}
	if emitter.tools == nil {
		t.Fatal("tools map is nil")
	}
	if emitter.started || emitter.done || emitter.usageReported {
		t.Fatal("a fresh emitter must be unstarted and unreported")
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("body = %q, want nothing written at construction", recorder.Body.String())
	}
}

// TestBridgeStreamEmitterSyntheticToolKeys documents the fallback key for tool
// events that arrive without one.
func TestBridgeStreamEmitterSyntheticToolKeys(t *testing.T) {
	emitter, _ := sxEmitter(Chat, "m")
	first := emitter.tool("")
	second := emitter.tool("")
	if first.Key != "tool-0" || second.Key != "tool-1" {
		t.Fatalf("synthetic keys = %q,%q, want tool-0,tool-1", first.Key, second.Key)
	}
	if first.ID == "" || first.ItemID == "" {
		t.Fatal("synthesised tool must receive generated ids")
	}
	if emitter.tool("named").Key != "named" {
		t.Fatalf("explicit key was not preserved: %q", emitter.tool("named").Key)
	}
}

// TestBridgeStreamEmitterEmptyTextStillOpensTheStream is a characterization of
// the current ordering: the start handshake runs before the empty-text check,
// so an empty text event still opens the downstream stream.
func TestBridgeStreamEmitterEmptyTextStillOpensTheStream(t *testing.T) {
	emitter, recorder := sxEmitter(Chat, "m")
	if err := emitter.Emit(bridgeStreamEvent{Kind: "text", Text: ""}); err != nil {
		t.Fatalf("Emit error = %v", err)
	}
	if !emitter.started {
		t.Fatal("emitter.started = false, want the empty text event to open the stream")
	}
	if got := recorder.Body.String(); !strings.Contains(got, `"role":"assistant"`) {
		t.Fatalf("body = %q, want the role chunk", got)
	}
	if lines := sxDataLines(recorder.Body.String()); len(lines) != 1 {
		t.Fatalf("wrote %d data lines, want only the start chunk", len(lines))
	}
	if emitter.text.String() != "" {
		t.Fatalf("text = %q, want empty", emitter.text.String())
	}
}

func TestBridgeStreamEmitterChatText(t *testing.T) {
	emitter, recorder := sxEmitter(Chat, "m")
	if err := emitter.Emit(bridgeStreamEvent{Kind: "start", ResponseID: "resp_1", Model: "real-model"}); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := emitter.Emit(bridgeStreamEvent{Kind: "text", Text: "Hello"}); err != nil {
		t.Fatalf("text: %v", err)
	}
	if err := emitter.Emit(bridgeStreamEvent{Kind: "usage", Usage: &Usage{Input: 2, Output: 3}}); err != nil {
		t.Fatalf("usage: %v", err)
	}
	if err := emitter.Emit(bridgeStreamEvent{Kind: "finish", Stop: "length"}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if err := emitter.Emit(bridgeStreamEvent{Kind: "done"}); err != nil {
		t.Fatalf("done: %v", err)
	}
	body := recorder.Body.String()
	for _, want := range []string{`"role":"assistant"`, `"content":"Hello"`, `"finish_reason":"length"`, `"prompt_tokens":2`, "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want it to contain %q", body, want)
		}
	}
	if emitter.model != "real-model" {
		t.Fatalf("model = %q, want the event model to win", emitter.model)
	}
	if !emitter.usageReported || emitter.usage != (Usage{Input: 2, Output: 3, Total: 5}) {
		t.Fatalf("usage = %+v reported=%v", emitter.usage, emitter.usageReported)
	}
	if got := len(sxDataLines(body)); got == 0 {
		t.Fatal("no data lines were written")
	}
}

// TestBridgeStreamEmitterDoneWithoutContentStartsAndFinishes documents that a
// lone done event still produces a complete downstream response.
func TestBridgeStreamEmitterDoneWithoutContentStartsAndFinishes(t *testing.T) {
	emitter, recorder := sxEmitter(Chat, "m")
	if err := emitter.Emit(bridgeStreamEvent{Kind: "done"}); err != nil {
		t.Fatalf("Emit error = %v", err)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"role":"assistant"`) || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("body = %q, want a complete Chat stream", body)
	}
	if !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Fatalf("body = %q, want the default stop reason", body)
	}
}

// TestBridgeStreamEmitterStopsWithoutStartedTools is the streaming twin of the
// phantom tool-call guard: a tool stop with no started tool becomes a stop.
func TestBridgeStreamEmitterStopsWithoutStartedTools(t *testing.T) {
	emitter, recorder := sxEmitter(Chat, "m")
	if err := emitter.Emit(bridgeStreamEvent{Kind: "tool_delta", ToolKey: "k", Text: "{}"}); err != nil {
		t.Fatalf("tool_delta: %v", err)
	}
	if err := emitter.Emit(bridgeStreamEvent{Kind: "finish", Stop: "tool_calls"}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if err := emitter.Emit(bridgeStreamEvent{Kind: "done"}); err != nil {
		t.Fatalf("done: %v", err)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Fatalf("body = %q, want the tool stop demoted to stop", body)
	}
	if strings.Contains(body, `"tool_calls"`) {
		t.Fatalf("body = %q, want no tool call chunk at all", body)
	}
}

// TestBridgeStreamEmitterToolStopWithStartedTool keeps the tool stop.
func TestBridgeStreamEmitterToolStopWithStartedTool(t *testing.T) {
	emitter, recorder := sxEmitter(Chat, "m")
	if err := emitter.Emit(bridgeStreamEvent{Kind: "tool_start", ToolKey: "k", ToolID: "call_1", ToolName: "f"}); err != nil {
		t.Fatalf("tool_start: %v", err)
	}
	if err := emitter.Emit(bridgeStreamEvent{Kind: "finish", Stop: "tool_calls"}); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if err := emitter.Emit(bridgeStreamEvent{Kind: "done"}); err != nil {
		t.Fatalf("done: %v", err)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"finish_reason":"tool_calls"`) {
		t.Fatalf("body = %q, want the tool stop preserved", body)
	}
	if !strings.Contains(body, `"name":"f"`) || !strings.Contains(body, `"arguments":""`) {
		t.Fatalf("body = %q, want the tool call chunk", body)
	}
}

// TestBridgeStreamEmitterQueuesArgumentsUntilTheNameArrives documents that
// arguments seen before a tool name are flushed once the tool can start.
func TestBridgeStreamEmitterQueuesArgumentsUntilTheNameArrives(t *testing.T) {
	emitter, recorder := sxEmitter(Chat, "m")
	if err := emitter.Emit(bridgeStreamEvent{Kind: "tool_delta", ToolKey: "k", Text: "{\"a\":"}); err != nil {
		t.Fatalf("tool_delta: %v", err)
	}
	if lines := sxDataLines(recorder.Body.String()); len(lines) != 1 {
		t.Fatalf("body = %q, want only the start chunk so far", recorder.Body.String())
	}
	if emitter.tools["k"].Started {
		t.Fatal("an unnamed tool must not be marked as started")
	}
	if err := emitter.Emit(bridgeStreamEvent{Kind: "tool_start", ToolKey: "k", ToolID: "call_1", ToolName: "f"}); err != nil {
		t.Fatalf("tool_start: %v", err)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"arguments":"{\"a\":"`) {
		t.Fatalf("body = %q, want the queued arguments to be flushed", body)
	}
}

// TestBridgeStreamEmitterArgumentDeltaAccumulation locks in how partial tool
// arguments accumulate: empty deltas emit nothing, repeated deltas are emitted
// again, and the concatenation is what lands in the final response.
func TestBridgeStreamEmitterArgumentDeltaAccumulation(t *testing.T) {
	emitter, recorder := sxEmitter(Responses, "m")
	deltas := []string{"{\"a\":", "", "1}", "{\"a\":"}
	for _, delta := range deltas {
		if err := emitter.Emit(bridgeStreamEvent{Kind: "tool_delta", ToolKey: "k", ToolID: "call_1", ToolName: "f", Text: delta}); err != nil {
			t.Fatalf("tool_delta(%q): %v", delta, err)
		}
	}
	if err := emitter.Emit(bridgeStreamEvent{Kind: "done"}); err != nil {
		t.Fatalf("done: %v", err)
	}
	tool := emitter.tools["k"]
	if got := tool.Arguments.String(); got != "{\"a\":1}{\"a\":" {
		t.Fatalf("accumulated arguments = %q", got)
	}
	body := recorder.Body.String()
	// Each record names the event twice on the wire (event: line and the type
	// field), so both spellings are counted per record.
	if got := strings.Count(body, "event: response.function_call_arguments.delta"); got != 3 {
		t.Fatalf("wrote %d argument delta records, want 3 (the empty one is skipped)", got)
	}
	if got := strings.Count(body, `"type":"response.function_call_arguments.delta"`); got != 3 {
		t.Fatalf("wrote %d argument delta payloads, want 3", got)
	}
	completed := sxJSONAt(t, body, len(sxDataLines(body))-1)
	response, _ := completed["response"].(map[string]any)
	output, _ := response["output"].([]any)
	found := false
	for _, raw := range output {
		item, _ := raw.(map[string]any)
		if item["type"] == "function_call" {
			found = true
			if item["arguments"] != "{\"a\":1}{\"a\":" {
				t.Fatalf("completed arguments = %v", item["arguments"])
			}
			if item["name"] != "f" || item["call_id"] != "call_1" {
				t.Fatalf("completed function_call = %v", item)
			}
		}
	}
	if !found {
		t.Fatalf("no function_call item in %v", output)
	}
	if got := sxEventNames(body); !containsAll(got, "response.output_item.added", "response.function_call_arguments.done", "response.output_item.done", "response.completed") {
		t.Fatalf("event names = %q", got)
	}
}

// TestBridgeStreamEmitterEmptyToolArgumentsBecomeObject documents that a tool
// call without arguments is closed with "{}" on the Responses target.
func TestBridgeStreamEmitterEmptyToolArgumentsBecomeObject(t *testing.T) {
	emitter, recorder := sxEmitter(Responses, "m")
	if err := emitter.Emit(bridgeStreamEvent{Kind: "tool_start", ToolKey: "k", ToolID: "call_1", ToolName: "f"}); err != nil {
		t.Fatalf("tool_start: %v", err)
	}
	if err := emitter.Emit(bridgeStreamEvent{Kind: "done"}); err != nil {
		t.Fatalf("done: %v", err)
	}
	if !strings.Contains(recorder.Body.String(), `"arguments":"{}"`) {
		t.Fatalf("body = %q, want the {} placeholder", recorder.Body.String())
	}
}

func TestBridgeStreamEmitterErrorEvent(t *testing.T) {
	emitter, recorder := sxEmitter(Chat, "m")
	err := emitter.Emit(bridgeStreamEvent{Kind: "error", Error: "boom", ErrorType: "rate_limit"})
	if !errors.Is(err, errStreamUpstreamFailure) {
		t.Fatalf("Emit error = %v, want errStreamUpstreamFailure", err)
	}
	if !emitter.done {
		t.Fatal("emitter.done = false after an error event")
	}
	body := recorder.Body.String()
	for _, want := range []string{`"message":"boom"`, `"type":"rate_limit"`, "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want it to contain %q", body, want)
		}
	}
	// An error event never opens the Chat stream, so no role or finish chunk
	// is written before the error frame.
	if strings.Contains(body, `"finish_reason"`) || strings.Contains(body, `"role":"assistant"`) {
		t.Fatalf("body = %q, want no synthetic chunks before an error frame", body)
	}
}

// TestBridgeStreamEmitterErrorFallbacks documents the defaults for an error
// event without a message or type.
func TestBridgeStreamEmitterErrorFallbacks(t *testing.T) {
	emitter, recorder := sxEmitter(Chat, "m")
	if err := emitter.Emit(bridgeStreamEvent{Kind: "error"}); !errors.Is(err, errStreamUpstreamFailure) {
		t.Fatalf("Emit error = %v", err)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"message":"upstream stream failed"`) || !strings.Contains(body, `"type":"upstream_error"`) {
		t.Fatalf("body = %q, want the fallback message and type", body)
	}
}

// TestBridgeStreamEmitterFinishIsIdempotent documents that the first done wins
// and later done events emit nothing.
func TestBridgeStreamEmitterFinishIsIdempotent(t *testing.T) {
	emitter, recorder := sxEmitter(Chat, "m")
	if err := emitter.Emit(bridgeStreamEvent{Kind: "text", Text: "hi"}); err != nil {
		t.Fatalf("text: %v", err)
	}
	if err := emitter.Emit(bridgeStreamEvent{Kind: "done"}); err != nil {
		t.Fatalf("done: %v", err)
	}
	after := recorder.Body.Len()
	if err := emitter.Emit(bridgeStreamEvent{Kind: "done"}); err != nil {
		t.Fatalf("second done: %v", err)
	}
	if recorder.Body.Len() != after {
		t.Fatalf("a second done wrote %d more bytes", recorder.Body.Len()-after)
	}
}

func TestBridgeStreamEmitterErrorTargets(t *testing.T) {
	cases := []struct {
		name   string
		target Protocol
		want   []string
	}{
		{"anthropic", Anthropic, []string{"event: error", `"type":"overloaded_error"`, `"message":"boom"`}},
		{"responses", Responses, []string{"event: response.failed", `"status":"failed"`, `"code":"overloaded_error"`}},
		{"chat", Chat, []string{`"message":"boom"`, "data: [DONE]"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			emitter, recorder := sxEmitter(tc.target, "m")
			if err := emitter.Emit(bridgeStreamEvent{Kind: "error", Error: "boom", ErrorType: "overloaded_error"}); !errors.Is(err, errStreamUpstreamFailure) {
				t.Fatalf("Emit error = %v", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(recorder.Body.String(), want) {
					t.Fatalf("body = %q, want it to contain %q", recorder.Body.String(), want)
				}
			}
		})
	}
}

// TestBridgeStreamEmitterRedactedThinking: an encrypted reasoning event with no
// text is replayed as the placeholder on Chat/Anthropic, but only opens the
// reasoning item on Responses.
func TestBridgeStreamEmitterRedactedThinking(t *testing.T) {
	for _, target := range []Protocol{Chat, Anthropic} {
		emitter, recorder := sxEmitter(target, "m")
		if err := emitter.Emit(bridgeStreamEvent{Kind: "reasoning", Encrypted: "ENC"}); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if !strings.Contains(recorder.Body.String(), anthropicRedactedThinkingPlaceholder) {
			t.Fatalf("%s: body = %q, want the placeholder", target, recorder.Body.String())
		}
		if emitter.reasoningEncrypted != "ENC" {
			t.Fatalf("%s: reasoningEncrypted = %q", target, emitter.reasoningEncrypted)
		}
	}
	emitter, recorder := sxEmitter(Responses, "m")
	if err := emitter.Emit(bridgeStreamEvent{Kind: "reasoning", Encrypted: "ENC"}); err != nil {
		t.Fatalf("responses: %v", err)
	}
	if strings.Contains(recorder.Body.String(), anthropicRedactedThinkingPlaceholder) {
		t.Fatalf("responses: body = %q, want no placeholder", recorder.Body.String())
	}
	if !emitter.reasoningOpen {
		t.Fatal("responses: reasoning item was not opened")
	}
	if !strings.Contains(recorder.Body.String(), "response.reasoning_summary_part.added") {
		t.Fatalf("responses: body = %q", recorder.Body.String())
	}
}

// TestBridgeStreamEmitterUnsupportedTarget documents that only the known
// targets can be started.
func TestBridgeStreamEmitterUnsupportedTarget(t *testing.T) {
	emitter, _ := sxEmitter(SystemOne, "m")
	err := emitter.Emit(bridgeStreamEvent{Kind: "text", Text: "hi"})
	if err == nil || !strings.Contains(err.Error(), "unsupported stream target") {
		t.Fatalf("Emit error = %v, want an unsupported target error", err)
	}
}

func TestBridgeStreamEmitterSequenceNumbersIncrease(t *testing.T) {
	emitter, recorder := sxEmitter(Responses, "m")
	if err := emitter.Emit(bridgeStreamEvent{Kind: "text", Text: "hi"}); err != nil {
		t.Fatalf("text: %v", err)
	}
	if err := emitter.Emit(bridgeStreamEvent{Kind: "done"}); err != nil {
		t.Fatalf("done: %v", err)
	}
	body := recorder.Body.String()
	first := sxJSONAt(t, body, 0)["sequence_number"]
	last := sxJSONAt(t, body, len(sxDataLines(body))-1)["sequence_number"]
	// response.created, response.in_progress, output_item.added,
	// content_part.added, output_text.delta, then the four closing records.
	if first != float64(0) || last != float64(8) {
		t.Fatalf("sequence numbers: first=%v last=%v, want 0 and 8", first, last)
	}
	if emitter.sequence != 9 {
		t.Fatalf("sequence counter = %d, want 9", emitter.sequence)
	}
}

func containsAll(values []string, want ...string) bool {
	set := map[string]bool{}
	for _, value := range values {
		set[value] = true
	}
	for _, want := range want {
		if !set[want] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// TranscodeStream
// ---------------------------------------------------------------------------

func sxFrames(payloads ...string) string {
	var out strings.Builder
	for _, payload := range payloads {
		out.WriteString("data: ")
		out.WriteString(payload)
		out.WriteString("\n\n")
	}
	return out.String()
}

func TestTranscodeStreamHappyPath(t *testing.T) {
	recorder := httptest.NewRecorder()
	input := sxFrames(
		`{"id":"chat_1","model":"upstream-model","choices":[{"delta":{"content":"Hello"}}]}`,
		`{"choices":[{"delta":{"content":" world"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`{"usage":{"prompt_tokens":3,"completion_tokens":1}}`,
		"[DONE]",
	)
	usage, reported, err := TranscodeStream(context.Background(), recorder, strings.NewReader(input), Chat, Chat, "requested")
	if err != nil {
		t.Fatalf("TranscodeStream error = %v", err)
	}
	if !reported {
		t.Fatal("reported = false, want true")
	}
	if usage != (Usage{Input: 3, Output: 1, Total: 4}) {
		t.Fatalf("usage = %+v", usage)
	}
	body := recorder.Body.String()
	for _, want := range []string{`"content":"Hello"`, `"content":" world"`, `"finish_reason":"stop"`, "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want it to contain %q", body, want)
		}
	}
	if strings.Contains(body, "upstream SSE stream ended") {
		t.Fatalf("body = %q, want no synthetic truncation error", body)
	}
}

func TestTranscodeStreamUpstreamError(t *testing.T) {
	recorder := httptest.NewRecorder()
	input := sxFrames(`{"choices":[{"delta":{"content":"partial"}}]}`, `{"error":{"message":"boom","type":"rate_limit"}}`)
	usage, reported, err := TranscodeStream(context.Background(), recorder, strings.NewReader(input), Chat, Chat, "m")
	if !errors.Is(err, errStreamUpstreamFailure) {
		t.Fatalf("error = %v, want errStreamUpstreamFailure", err)
	}
	if reported || usage != (Usage{}) {
		t.Fatalf("usage = %+v reported = %v", usage, reported)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"message":"boom"`) || !strings.Contains(body, `"type":"rate_limit"`) {
		t.Fatalf("body = %q, want the in band upstream error", body)
	}
	if strings.Contains(body, "upstream SSE stream ended") {
		t.Fatalf("body = %q, want no second synthetic error after an upstream error", body)
	}
}

func TestTranscodeStreamTruncated(t *testing.T) {
	t.Run("stream simply ends", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		input := "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n"
		_, _, err := TranscodeStream(context.Background(), recorder, strings.NewReader(input), Chat, Chat, "m")
		if !errors.Is(err, errSSEUnexpectedEOF) {
			t.Fatalf("error = %v, want errSSEUnexpectedEOF", err)
		}
		if !strings.Contains(recorder.Body.String(), "upstream SSE stream ended before a terminal event") {
			t.Fatalf("body = %q, want the synthetic truncation error", recorder.Body.String())
		}
	})
	t.Run("record without its blank line", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		input := "data: {\"usage\":{\"prompt_tokens\":5}}\n"
		usage, reported, err := TranscodeStream(context.Background(), recorder, strings.NewReader(input), Chat, Chat, "m")
		if !errors.Is(err, errSSEUnexpectedEOF) {
			t.Fatalf("error = %v, want errSSEUnexpectedEOF", err)
		}
		if usage != (Usage{}) || reported {
			t.Fatalf("usage = %+v reported = %v, want the truncated record ignored", usage, reported)
		}
	})
	t.Run("client canceled mid stream", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		recorder := httptest.NewRecorder()
		input := "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n"
		_, _, err := TranscodeStream(ctx, recorder, strings.NewReader(input), Chat, Chat, "m")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled from the post read cancellation check", err)
		}
		if strings.Contains(recorder.Body.String(), "upstream SSE stream ended") {
			t.Fatalf("body = %q, want no synthetic error for a canceled client", recorder.Body.String())
		}
	})
	t.Run("client canceled with a truncated record", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		recorder := httptest.NewRecorder()
		_, _, err := TranscodeStream(ctx, recorder, strings.NewReader("data: {\"usage\":{}}\n"), Chat, Chat, "m")
		if !errors.Is(err, errSSEUnexpectedEOF) {
			t.Fatalf("error = %v, want the read error passed through for a canceled client", err)
		}
		if strings.Contains(recorder.Body.String(), "upstream SSE stream ended") {
			t.Fatalf("body = %q, want no synthetic error for a canceled client", recorder.Body.String())
		}
	})
	t.Run("reader failure", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		boom := errors.New("dial tcp: refused")
		_, _, err := TranscodeStream(context.Background(), recorder, &sxFailReader{err: boom}, Chat, Chat, "m")
		if !errors.Is(err, boom) {
			t.Fatalf("error = %v, want the reader failure", err)
		}
		if !strings.Contains(recorder.Body.String(), "upstream SSE stream failed: dial tcp: refused") {
			t.Fatalf("body = %q, want the reader failure message", recorder.Body.String())
		}
	})
}

func TestTranscodeStreamRequiresFlusher(t *testing.T) {
	writer := &sxNoFlushWriter{}
	usage, reported, err := TranscodeStream(context.Background(), writer, strings.NewReader(""), Chat, Chat, "m")
	if err == nil || !strings.Contains(err.Error(), "does not support streaming") {
		t.Fatalf("error = %v, want a streaming unsupported error", err)
	}
	if reported || usage != (Usage{}) {
		t.Fatalf("usage = %+v reported = %v", usage, reported)
	}
	if writer.body.String() != "" {
		t.Fatalf("body = %q, want nothing written", writer.body.String())
	}
}

// TestTranscodeStreamStopReasonMapping checks how upstream stop reasons are
// translated into the downstream Chat and Anthropic vocabularies.
func TestTranscodeStreamStopReasonMapping(t *testing.T) {
	cases := []struct {
		stop      string
		chatWant  string
		anthropic string
	}{
		{"end_turn", `"finish_reason":"stop"`, `"stop_reason":"end_turn"`},
		{"stop_sequence", `"finish_reason":"stop"`, `"stop_reason":"stop_sequence"`},
		{"max_tokens", `"finish_reason":"length"`, `"stop_reason":"max_tokens"`},
		{"pause_turn", `"finish_reason":"stop"`, `"stop_reason":"pause_turn"`},
		{"refusal", `"finish_reason":"stop"`, `"stop_reason":"refusal"`},
	}
	for _, tc := range cases {
		t.Run(tc.stop, func(t *testing.T) {
			input := sxFrames(
				`{"type":"message_start","message":{"id":"msg_1","model":"m","usage":{"input_tokens":1}}}`,
				`{"type":"message_delta","usage":{"output_tokens":1},"delta":{"stop_reason":"`+tc.stop+`"}}`,
				`{"type":"message_stop"}`,
			)
			recorder := httptest.NewRecorder()
			usage, reported, err := TranscodeStream(context.Background(), recorder, strings.NewReader(input), Anthropic, Chat, "m")
			if err != nil {
				t.Fatalf("chat transcode error = %v", err)
			}
			if !reported || usage != (Usage{Input: 1, Output: 1, Total: 2}) {
				t.Fatalf("chat usage = %+v reported = %v", usage, reported)
			}
			if !strings.Contains(recorder.Body.String(), tc.chatWant) {
				t.Fatalf("chat body = %q, want %q", recorder.Body.String(), tc.chatWant)
			}

			recorder = httptest.NewRecorder()
			if _, _, err := TranscodeStream(context.Background(), recorder, strings.NewReader(input), Anthropic, Anthropic, "m"); err != nil {
				t.Fatalf("anthropic transcode error = %v", err)
			}
			if !strings.Contains(recorder.Body.String(), tc.anthropic) {
				t.Fatalf("anthropic body = %q, want %q", recorder.Body.String(), tc.anthropic)
			}
		})
	}
}

// TestTranscodeStreamResponsesToolTurn checks the tool turn survives the live
// stream path as well as the collapse path.
func TestTranscodeStreamResponsesToolTurn(t *testing.T) {
	recorder := httptest.NewRecorder()
	input := sxFrames(
		`{"type":"response.created","response":{"id":"resp_1","model":"m"}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"f"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"a\":1}"}`,
		`{"type":"response.completed","response":{"output":[]}}`,
	)
	_, _, err := TranscodeStream(context.Background(), recorder, strings.NewReader(input), Responses, Chat, "m")
	if err != nil {
		t.Fatalf("TranscodeStream error = %v", err)
	}
	body := recorder.Body.String()
	for _, want := range []string{`"name":"f"`, `"arguments":"{\"a\":1}"`, `"finish_reason":"tool_calls"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, want it to contain %q", body, want)
		}
	}
}

func TestClientCanceled(t *testing.T) {
	live := context.Background()
	dead, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name   string
		ctx    context.Context
		stream error
		want   bool
	}{
		{"live context and no error", live, nil, false},
		{"nil context and no error", nil, nil, false},
		{"canceled error with a live context", live, context.Canceled, true},
		{"wrapped canceled error", live, fmt.Errorf("copy: %w", context.Canceled), true},
		{"canceled context with no error", dead, nil, true},
		{"unrelated error", live, errors.New("boom"), false},
		{"nil context with an unrelated error", nil, errors.New("boom"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClientCanceled(tc.ctx, tc.stream); got != tc.want {
				t.Fatalf("ClientCanceled = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ForwardStream
// ---------------------------------------------------------------------------

func TestForwardStreamPassthrough(t *testing.T) {
	recorder := httptest.NewRecorder()
	input := sxFrames(`{"choices":[{"delta":{"content":"Hi"}}]}`, "[DONE]")
	usage, reported, err := ForwardStream(context.Background(), recorder, strings.NewReader(input), Chat, "m")
	if err != nil {
		t.Fatalf("ForwardStream error = %v", err)
	}
	if reported || usage != (Usage{}) {
		t.Fatalf("usage = %+v reported = %v, want no usage in this stream", usage, reported)
	}
	if recorder.Body.String() != input {
		t.Fatalf("body = %q, want the upstream bytes unchanged", recorder.Body.String())
	}
}

func TestForwardStreamCollectsUsage(t *testing.T) {
	recorder := httptest.NewRecorder()
	input := sxFrames(`{"usage":{"prompt_tokens":4}}`, "[DONE]")
	usage, reported, err := ForwardStream(context.Background(), recorder, strings.NewReader(input), Chat, "m")
	if err != nil {
		t.Fatalf("ForwardStream error = %v", err)
	}
	if !reported || usage != (Usage{Input: 4, Total: 4}) {
		t.Fatalf("usage = %+v reported = %v", usage, reported)
	}
	if recorder.Body.String() != input {
		t.Fatalf("body = %q, want the upstream bytes unchanged", recorder.Body.String())
	}
}

func TestForwardStreamErrorTermination(t *testing.T) {
	recorder := httptest.NewRecorder()
	input := sxFrames(`{"error":{"message":"boom","type":"rate_limit"}}`)
	usage, _, err := ForwardStream(context.Background(), recorder, strings.NewReader(input), Chat, "m")
	if !errors.Is(err, errStreamUpstreamFailure) {
		t.Fatalf("error = %v, want errStreamUpstreamFailure", err)
	}
	if usage != (Usage{}) {
		t.Fatalf("usage = %+v", usage)
	}
	if recorder.Body.String() != input {
		t.Fatalf("body = %q, want only the forwarded bytes", recorder.Body.String())
	}
}

func TestForwardStreamTruncated(t *testing.T) {
	t.Run("clean end without a terminal event", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		input := "data: {\"usage\":{\"prompt_tokens\":2}}\n\n"
		usage, reported, err := ForwardStream(context.Background(), recorder, strings.NewReader(input), Chat, "m")
		if !errors.Is(err, errSSEUnexpectedEOF) {
			t.Fatalf("error = %v, want errSSEUnexpectedEOF", err)
		}
		if !reported || usage != (Usage{Input: 2, Total: 2}) {
			t.Fatalf("usage = %+v reported = %v", usage, reported)
		}
		if !strings.Contains(recorder.Body.String(), "upstream SSE stream ended before a terminal event") {
			t.Fatalf("body = %q, want the synthetic truncation error appended", recorder.Body.String())
		}
		if !strings.HasPrefix(recorder.Body.String(), input) {
			t.Fatalf("body = %q, want the upstream bytes first", recorder.Body.String())
		}
	})
	t.Run("unparseable record", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		input := "data: {broken\n\n"
		_, _, err := ForwardStream(context.Background(), recorder, strings.NewReader(input), Chat, "m")
		if err == nil || !strings.Contains(err.Error(), "invalid upstream SSE JSON") {
			t.Fatalf("error = %v, want the parse error", err)
		}
		if !strings.Contains(recorder.Body.String(), "upstream SSE stream failed:") {
			t.Fatalf("body = %q, want the failure message", recorder.Body.String())
		}
	})
	t.Run("reader failure wins over the parse state", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		boom := errors.New("connection reset")
		_, _, err := ForwardStream(context.Background(), recorder, &sxFailReader{err: boom}, Chat, "m")
		if !errors.Is(err, boom) {
			t.Fatalf("error = %v, want the reader failure", err)
		}
	})
	t.Run("canceled client", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		recorder := httptest.NewRecorder()
		input := "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n"
		_, _, err := ForwardStream(ctx, recorder, strings.NewReader(input), Chat, "m")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
		if recorder.Body.String() != input {
			t.Fatalf("body = %q, want no synthetic error for a canceled client", recorder.Body.String())
		}
	})
}

func TestForwardStreamRequiresFlusher(t *testing.T) {
	writer := &sxNoFlushWriter{}
	_, _, err := ForwardStream(context.Background(), writer, strings.NewReader(""), Chat, "m")
	if err == nil || !strings.Contains(err.Error(), "does not support streaming") {
		t.Fatalf("error = %v, want a streaming unsupported error", err)
	}
}

// ---------------------------------------------------------------------------
// RotationStream
// ---------------------------------------------------------------------------

func sxRotationReady(t *testing.T, calls *int) func() error {
	return func() error {
		*calls++
		return nil
	}
}

func TestRotationStreamCommitsOnContent(t *testing.T) {
	recorder := httptest.NewRecorder()
	input := sxFrames(`{"id":"chat_1","model":"m","choices":[{"delta":{"content":"Hi"}}]}`, "[DONE]")
	calls := 0
	usage, reported, committed, err := RotationStream(context.Background(), recorder, strings.NewReader(input), Chat, Chat, "alias", sxRotationReady(t, &calls))
	if err != nil {
		t.Fatalf("RotationStream error = %v", err)
	}
	if !committed {
		t.Fatal("committed = false, want true")
	}
	if calls != 1 {
		t.Fatalf("ready called %d times, want 1", calls)
	}
	if reported || usage != (Usage{}) {
		t.Fatalf("usage = %+v reported = %v", usage, reported)
	}
	if !strings.Contains(recorder.Body.String(), `"content":"Hi"`) {
		t.Fatalf("body = %q, want the text committed", recorder.Body.String())
	}
}

// TestRotationStreamDefersTheStartEvent documents that non content events are
// buffered until the first effective event commits the stream.
func TestRotationStreamDefersTheStartEvent(t *testing.T) {
	recorder := httptest.NewRecorder()
	input := sxFrames(`{"id":"chat_1","model":"m","choices":[]}`, `{"usage":{"prompt_tokens":2}}`, `{"choices":[{"delta":{"content":"Hi"}}]}`, "[DONE]")
	calls := 0
	if _, _, committed, err := RotationStream(context.Background(), recorder, strings.NewReader(input), Chat, Chat, "alias", sxRotationReady(t, &calls)); err != nil || !committed {
		t.Fatalf("committed=%v err=%v", committed, err)
	}
	body := recorder.Body.String()
	start := strings.Index(body, `"role":"assistant"`)
	content := strings.Index(body, `"content":"Hi"`)
	if start < 0 || content < 0 || start > content {
		t.Fatalf("body = %q, want the buffered start flushed before the text", body)
	}
	if !strings.Contains(body, `"prompt_tokens":2`) {
		t.Fatalf("body = %q, want the deferred usage event", body)
	}
}

func TestRotationStreamCommitsOnDoneOnly(t *testing.T) {
	recorder := httptest.NewRecorder()
	input := sxFrames("[DONE]")
	calls := 0
	if _, _, committed, err := RotationStream(context.Background(), recorder, strings.NewReader(input), Chat, Chat, "alias", sxRotationReady(t, &calls)); err != nil || !committed {
		t.Fatalf("committed=%v err=%v", committed, err)
	}
	if calls != 1 {
		t.Fatalf("ready called %d times, want 1", calls)
	}
}

func TestRotationStreamErrorsBeforeCommit(t *testing.T) {
	t.Run("upstream error event", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		input := sxFrames(`{"error":{"message":"bad","type":"upstream_error"}}`)
		calls := 0
		usage, reported, committed, err := RotationStream(context.Background(), recorder, strings.NewReader(input), Chat, Chat, "alias", sxRotationReady(t, &calls))
		var failure *StreamFailure
		if !errors.As(err, &failure) {
			t.Fatalf("error = %v, want a *StreamFailure", err)
		}
		if failure.Message != "bad" || failure.Type != "upstream_error" {
			t.Fatalf("failure = %+v", failure)
		}
		if committed {
			t.Fatal("committed = true, want false")
		}
		if calls != 0 {
			t.Fatalf("ready called %d times, want 0", calls)
		}
		if reported || usage != (Usage{}) {
			t.Fatalf("usage = %+v reported = %v", usage, reported)
		}
		if recorder.Body.Len() != 0 {
			t.Fatalf("body = %q, want nothing committed", recorder.Body.String())
		}
	})
	t.Run("parse error", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		calls := 0
		_, _, committed, err := RotationStream(context.Background(), recorder, strings.NewReader("data: {broken\n\n"), Chat, Chat, "alias", sxRotationReady(t, &calls))
		var failure *StreamFailure
		if !errors.As(err, &failure) || failure.Type != "parse_error" {
			t.Fatalf("error = %v, want a parse_error StreamFailure", err)
		}
		if committed || calls != 0 {
			t.Fatalf("committed = %v ready calls = %d", committed, calls)
		}
	})
	t.Run("unexpected end of stream", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		calls := 0
		_, _, committed, err := RotationStream(context.Background(), recorder, strings.NewReader(""), Chat, Chat, "alias", sxRotationReady(t, &calls))
		var failure *StreamFailure
		if !errors.As(err, &failure) || failure.Type != "unexpected_eof" {
			t.Fatalf("error = %v, want an unexpected_eof StreamFailure", err)
		}
		if committed {
			t.Fatal("committed = true, want false")
		}
		if recorder.Body.Len() != 0 {
			t.Fatalf("body = %q, want nothing committed", recorder.Body.String())
		}
	})
}

// TestRotationStreamInvalidPrelude: more than 256 buffered non content events
// without a commit aborts the turn before the model is chosen.
func TestRotationStreamInvalidPrelude(t *testing.T) {
	var builder strings.Builder
	frame := "data: {\"usage\":{\"prompt_tokens\":1}}\n\n"
	for i := 0; i < 257; i++ {
		builder.WriteString(frame)
	}
	recorder := httptest.NewRecorder()
	calls := 0
	_, _, committed, err := RotationStream(context.Background(), recorder, strings.NewReader(builder.String()), Chat, Chat, "alias", sxRotationReady(t, &calls))
	var failure *StreamFailure
	if !errors.As(err, &failure) || failure.Type != "invalid_prelude" {
		t.Fatalf("error = %v, want an invalid_prelude StreamFailure", err)
	}
	if committed || calls != 0 {
		t.Fatalf("committed = %v ready calls = %d", committed, calls)
	}
}

func TestRotationStreamPreludeAtTheLimit(t *testing.T) {
	var builder strings.Builder
	frame := "data: {\"usage\":{\"prompt_tokens\":1}}\n\n"
	for i := 0; i < 256; i++ {
		builder.WriteString(frame)
	}
	builder.WriteString(sxFrames(`{"choices":[{"delta":{"content":"Hi"}}]}`))
	builder.WriteString(sxFrames("[DONE]"))
	recorder := httptest.NewRecorder()
	calls := 0
	if _, _, committed, err := RotationStream(context.Background(), recorder, strings.NewReader(builder.String()), Chat, Chat, "alias", sxRotationReady(t, &calls)); err != nil || !committed {
		t.Fatalf("committed=%v err=%v", committed, err)
	}
	if calls != 1 {
		t.Fatalf("ready called %d times, want 1", calls)
	}
}

// TestRotationStreamTruncatedAfterCommit emits the in band interruption notice
// instead of replaying the failure.
func TestRotationStreamTruncatedAfterCommit(t *testing.T) {
	recorder := httptest.NewRecorder()
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n"
	_, _, committed, err := RotationStream(context.Background(), recorder, strings.NewReader(input), Chat, Chat, "alias", func() error { return nil })
	if !committed {
		t.Fatal("committed = false, want true")
	}
	var failure *StreamFailure
	if !errors.As(err, &failure) || failure.Type != "unexpected_eof" {
		t.Fatalf("error = %v, want an unexpected_eof StreamFailure", err)
	}
	if !strings.Contains(recorder.Body.String(), "本次响应已结束") {
		t.Fatalf("body = %q, want the in band interruption notice", recorder.Body.String())
	}
}

func TestRotationStreamRequiresFlusher(t *testing.T) {
	writer := &sxNoFlushWriter{}
	_, _, _, err := RotationStream(context.Background(), writer, strings.NewReader(""), Chat, Chat, "alias", func() error { return nil })
	if err == nil || !strings.Contains(err.Error(), "streaming is unsupported") {
		t.Fatalf("error = %v, want a streaming unsupported error", err)
	}
}

func TestRotationStreamReadyFailureAborts(t *testing.T) {
	recorder := httptest.NewRecorder()
	input := sxFrames(`{"choices":[{"delta":{"content":"Hi"}}]}`)
	refused := errors.New("model not allowed")
	_, _, committed, err := RotationStream(context.Background(), recorder, strings.NewReader(input), Chat, Chat, "alias", func() error { return refused })
	if !errors.Is(err, refused) {
		t.Fatalf("error = %v, want the ready failure", err)
	}
	if committed {
		t.Fatal("committed = true, want false")
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("body = %q, want nothing written", recorder.Body.String())
	}
}

func TestRotationStreamModelIsTheAlias(t *testing.T) {
	recorder := httptest.NewRecorder()
	input := sxFrames(`{"id":"chat_1","model":"upstream-model","choices":[{"delta":{"content":"Hi"}}]}`, "[DONE]")
	if _, _, _, err := RotationStream(context.Background(), recorder, strings.NewReader(input), Chat, Chat, "alias-model", func() error { return nil }); err != nil {
		t.Fatalf("RotationStream error = %v", err)
	}
	if !strings.Contains(recorder.Body.String(), `"model":"alias-model"`) {
		t.Fatalf("body = %q, want the alias as the model everywhere", recorder.Body.String())
	}
}

func TestStreamFailureAndClientWriteError(t *testing.T) {
	failure := &StreamFailure{Message: "secret upstream text", Type: "rate_limit"}
	if got := failure.Error(); got != "upstream stream failed" {
		t.Fatalf("StreamFailure.Error() = %q, want the raw text hidden", got)
	}
	inner := errors.New("broken pipe")
	write := &ClientWriteError{inner}
	if got := write.Error(); got != "downstream write failed" {
		t.Fatalf("ClientWriteError.Error() = %q", got)
	}
	if !errors.Is(write, inner) {
		t.Fatal("ClientWriteError must unwrap to the underlying write error")
	}
}

func TestRotationStreamClientWriteErrorIsNotReplayed(t *testing.T) {
	recorder := &sxBrokenWriter{failAfter: 0}
	input := sxFrames(`{"choices":[{"delta":{"content":"Hi"}}]}`)
	_, _, committed, err := RotationStream(context.Background(), recorder, strings.NewReader(input), Chat, Chat, "alias", func() error { return nil })
	var writeErr *ClientWriteError
	if !errors.As(err, &writeErr) {
		t.Fatalf("error = %v, want a *ClientWriteError", err)
	}
	if !committed {
		t.Fatal("committed = false, want true")
	}
	if recorder.attempts > 1 {
		t.Fatalf("write attempted %d times, want the failure not replayed as an upstream error", recorder.attempts)
	}
}

// sxBrokenWriter is a Flusher whose writes fail after a budget is exhausted.
type sxBrokenWriter struct {
	attempts   int
	failAfter  int
	lastGood   string
	flushCount int
}

func (w *sxBrokenWriter) Header() http.Header { return http.Header{} }
func (w *sxBrokenWriter) WriteHeader(int)     {}
func (w *sxBrokenWriter) Flush()              { w.flushCount++ }
func (w *sxBrokenWriter) Write(p []byte) (int, error) {
	w.attempts++
	if w.attempts > w.failAfter {
		return 0, errors.New("write failed")
	}
	w.lastGood += string(p)
	return len(p), nil
}

// ---------------------------------------------------------------------------
// collapse
// ---------------------------------------------------------------------------

func TestFirstNonEmpty(t *testing.T) {
	cases := []struct {
		name   string
		values []string
		want   string
	}{
		{"no values", nil, ""},
		{"all empty", []string{"", ""}, ""},
		{"single value", []string{"a"}, "a"},
		{"first non empty wins", []string{"", "b", "c"}, "b"},
		{"whitespace is not empty", []string{" ", "b"}, " "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstNonEmpty(tc.values...); got != tc.want {
				t.Fatalf("firstNonEmpty(%q) = %q, want %q", tc.values, got, tc.want)
			}
		})
	}
}

func TestCollapseAccumulator(t *testing.T) {
	t.Run("text and start", func(t *testing.T) {
		acc := &collapseAccumulator{tools: map[string]int{}}
		acc.apply(bridgeStreamEvent{Kind: "start", ResponseID: "resp_1", Model: "m"})
		acc.apply(bridgeStreamEvent{Kind: "text", Text: "Hello"})
		acc.apply(bridgeStreamEvent{Kind: "text", Text: " world"})
		acc.apply(bridgeStreamEvent{Kind: "finish", Stop: "stop"})
		if acc.response.Text != "Hello world" {
			t.Fatalf("text = %q", acc.response.Text)
		}
		if acc.response.ID != "resp_1" || acc.response.Model != "m" || acc.response.Stop != "stop" {
			t.Fatalf("response = %+v", acc.response)
		}
	})
	t.Run("an empty start does not clear what is known", func(t *testing.T) {
		acc := &collapseAccumulator{tools: map[string]int{}}
		acc.apply(bridgeStreamEvent{Kind: "start", ResponseID: "resp_1", Model: "m"})
		acc.apply(bridgeStreamEvent{Kind: "start"})
		if acc.response.ID != "resp_1" || acc.response.Model != "m" {
			t.Fatalf("response = %+v", acc.response)
		}
	})
	t.Run("an empty reasoning event creates no block", func(t *testing.T) {
		acc := &collapseAccumulator{tools: map[string]int{}}
		acc.apply(bridgeStreamEvent{Kind: "reasoning"})
		if len(acc.response.Reasoning) != 0 {
			t.Fatalf("reasoning = %+v, want none", acc.response.Reasoning)
		}
	})
	t.Run("reasoning deltas accumulate in one block", func(t *testing.T) {
		acc := &collapseAccumulator{tools: map[string]int{}}
		acc.apply(bridgeStreamEvent{Kind: "reasoning", Text: "a"})
		acc.apply(bridgeStreamEvent{Kind: "reasoning", Text: "b"})
		acc.apply(bridgeStreamEvent{Kind: "reasoning_signature", Signature: "sig"})
		if len(acc.response.Reasoning) != 1 {
			t.Fatalf("reasoning blocks = %d, want 1", len(acc.response.Reasoning))
		}
		if acc.response.Reasoning[0].Text != "ab" || acc.response.Reasoning[0].Signature != "sig" {
			t.Fatalf("block = %+v", acc.response.Reasoning[0])
		}
	})
	t.Run("encrypted reasoning opens a second block", func(t *testing.T) {
		acc := &collapseAccumulator{tools: map[string]int{}}
		acc.apply(bridgeStreamEvent{Kind: "reasoning", Text: "visible"})
		acc.apply(bridgeStreamEvent{Kind: "reasoning", Encrypted: "ENC"})
		if len(acc.response.Reasoning) != 2 {
			t.Fatalf("reasoning blocks = %d, want 2", len(acc.response.Reasoning))
		}
		if acc.response.Reasoning[0].Text != "visible" || acc.response.Reasoning[0].Encrypted != "" {
			t.Fatalf("first block = %+v", acc.response.Reasoning[0])
		}
		if acc.response.Reasoning[1].Encrypted != "ENC" {
			t.Fatalf("second block = %+v", acc.response.Reasoning[1])
		}
	})
	t.Run("reasoning after an encrypted item starts yet another block", func(t *testing.T) {
		// Each encrypted event first touches the current (possibly empty)
		// block, then splits, so an encrypted event with no text leaves an
		// extra empty reasoning block behind.
		acc := &collapseAccumulator{tools: map[string]int{}}
		acc.apply(bridgeStreamEvent{Kind: "reasoning", Encrypted: "ENC"})
		acc.apply(bridgeStreamEvent{Kind: "reasoning", Text: "after"})
		if len(acc.response.Reasoning) != 3 {
			t.Fatalf("reasoning blocks = %d, want 3 (one empty block per encrypted event)", len(acc.response.Reasoning))
		}
		if acc.response.Reasoning[1].Encrypted != "ENC" {
			t.Fatalf("second block = %+v", acc.response.Reasoning[1])
		}
		if acc.response.Reasoning[2].Text != "after" {
			t.Fatalf("third block = %+v", acc.response.Reasoning[2])
		}
	})
	t.Run("an empty signature still opens a block", func(t *testing.T) {
		acc := &collapseAccumulator{tools: map[string]int{}}
		acc.apply(bridgeStreamEvent{Kind: "reasoning_signature"})
		if len(acc.response.Reasoning) != 1 {
			t.Fatalf("reasoning blocks = %d, want 1", len(acc.response.Reasoning))
		}
	})
	t.Run("tool blocks are keyed and back-filled", func(t *testing.T) {
		acc := &collapseAccumulator{tools: map[string]int{}}
		acc.apply(bridgeStreamEvent{Kind: "tool_delta", ToolKey: "k", Text: "{\"a\":"})
		acc.apply(bridgeStreamEvent{Kind: "tool_start", ToolKey: "k", ToolID: "call_1", ToolName: "f"})
		acc.apply(bridgeStreamEvent{Kind: "tool_delta", ToolKey: "k", Text: "1}"})
		if len(acc.response.Tools) != 1 {
			t.Fatalf("tool blocks = %d, want 1", len(acc.response.Tools))
		}
		tool := acc.response.Tools[0]
		if tool.ID != "call_1" || tool.Name != "f" || tool.ArgumentsJSON != "{\"a\":1}" || tool.Kind != "tool_call" {
			t.Fatalf("tool = %+v", tool)
		}
	})
	t.Run("a tool id or name is never overwritten", func(t *testing.T) {
		acc := &collapseAccumulator{tools: map[string]int{}}
		acc.apply(bridgeStreamEvent{Kind: "tool_start", ToolKey: "k", ToolID: "call_1", ToolName: "f"})
		acc.apply(bridgeStreamEvent{Kind: "tool_start", ToolKey: "k", ToolID: "call_2", ToolName: "g"})
		if acc.response.Tools[0].ID != "call_1" || acc.response.Tools[0].Name != "f" {
			t.Fatalf("tool = %+v", acc.response.Tools[0])
		}
	})
	t.Run("distinct tool keys become distinct blocks", func(t *testing.T) {
		acc := &collapseAccumulator{tools: map[string]int{}}
		acc.apply(bridgeStreamEvent{Kind: "tool_start", ToolKey: "a", ToolID: "c1", ToolName: "one"})
		acc.apply(bridgeStreamEvent{Kind: "tool_start", ToolKey: "b", ToolID: "c2", ToolName: "two"})
		if len(acc.response.Tools) != 2 || acc.response.Tools[1].Name != "two" {
			t.Fatalf("tools = %+v", acc.response.Tools)
		}
	})
	t.Run("usage merges with the stream rules", func(t *testing.T) {
		acc := &collapseAccumulator{tools: map[string]int{}}
		acc.apply(bridgeStreamEvent{Kind: "usage", Usage: &Usage{Input: 4, Output: 0, Total: 4}})
		acc.apply(bridgeStreamEvent{Kind: "usage", Usage: &Usage{Output: 3}})
		acc.apply(bridgeStreamEvent{Kind: "usage"})
		if acc.response.Usage != (Usage{Input: 4, Output: 3, Total: 7}) {
			t.Fatalf("usage = %+v", acc.response.Usage)
		}
	})
}

// TestCollapseAccumulatorSplitReasoning covers the helper directly.
func TestCollapseAccumulatorSplitReasoning(t *testing.T) {
	acc := &collapseAccumulator{tools: map[string]int{}}
	first := acc.reasoningBlock()
	acc.splitReasoning()
	second := acc.reasoningBlock()
	if first == second {
		t.Fatal("splitReasoning must start a new block")
	}
	if len(acc.response.Reasoning) != 2 {
		t.Fatalf("reasoning blocks = %d, want 2", len(acc.response.Reasoning))
	}
	if acc.reasoningBlock() != second {
		t.Fatal("reasoningBlock must be sticky until the next split")
	}
}

func TestCollapseStreamChat(t *testing.T) {
	input := sxFrames(
		`{"id":"chat_1","model":"m","choices":[{"delta":{"reasoning_content":"why"}}]}`,
		`{"choices":[{"delta":{"content":"Hello"}}]}`,
		`{"choices":[{"delta":{"content":" world"}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"get_weather","arguments":"{\"city\":"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"NY\"}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`{"usage":{"prompt_tokens":5,"completion_tokens":7}}`,
		"[DONE]",
	)
	document, err := CollapseStream(strings.NewReader(input), Chat, "requested-model")
	if err != nil {
		t.Fatalf("CollapseStream error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("collapsed document is not JSON: %v", err)
	}
	if decoded["model"] != "m" {
		t.Fatalf("model = %v, want the upstream model", decoded["model"])
	}
	choices, _ := decoded["choices"].([]any)
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	if message["content"] != "Hello world" {
		t.Fatalf("content = %v", message["content"])
	}
	if message["reasoning_content"] != "why" {
		t.Fatalf("reasoning_content = %v", message["reasoning_content"])
	}
	if choice["finish_reason"] != "tool_calls" {
		t.Fatalf("finish_reason = %v", choice["finish_reason"])
	}
	calls, _ := message["tool_calls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("tool_calls = %v", calls)
	}
	call, _ := calls[0].(map[string]any)
	function, _ := call["function"].(map[string]any)
	if function["name"] != "get_weather" || function["arguments"] != `{"city":"NY"}` {
		t.Fatalf("function = %v", function)
	}
	usage, _ := decoded["usage"].(map[string]any)
	if usage["prompt_tokens"] != float64(5) || usage["completion_tokens"] != float64(7) {
		t.Fatalf("usage = %v", usage)
	}
}

// TestCollapseStreamRepeatedArgumentDeltas documents that repeated identical
// argument deltas are concatenated rather than de-duplicated.
func TestCollapseStreamRepeatedArgumentDeltas(t *testing.T) {
	input := sxFrames(
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"f","arguments":"{}"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		"[DONE]",
	)
	document, err := CollapseStream(strings.NewReader(input), Chat, "m")
	if err != nil {
		t.Fatalf("CollapseStream error = %v", err)
	}
	if !strings.Contains(string(document), `"arguments":"{}{}"`) {
		t.Fatalf("document = %s, want the duplicated delta to be concatenated", document)
	}
}

func TestCollapseStreamAnthropic(t *testing.T) {
	input := sxFrames(
		`{"type":"message_start","message":{"id":"msg_1","model":"claude","usage":{"input_tokens":3}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"deep","signature":"sig"}}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Hello"}}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_1","name":"f"}}`,
		`{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"a\":1}"}}`,
		`{"type":"message_delta","usage":{"output_tokens":4},"delta":{"stop_reason":"tool_use"}}`,
		`{"type":"message_stop"}`,
	)
	document, err := CollapseStream(strings.NewReader(input), Anthropic, "m")
	if err != nil {
		t.Fatalf("CollapseStream error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("collapsed document is not JSON: %v", err)
	}
	if decoded["id"] != "msg_1" || decoded["model"] != "claude" {
		t.Fatalf("document = %v", decoded)
	}
	content, _ := decoded["content"].([]any)
	if len(content) != 3 {
		t.Fatalf("content blocks = %v", content)
	}
	thinking, _ := content[0].(map[string]any)
	text, _ := content[1].(map[string]any)
	tool, _ := content[2].(map[string]any)
	if thinking["thinking"] != "deep" || thinking["signature"] != "sig" {
		t.Fatalf("thinking block = %v", thinking)
	}
	if text["text"] != "Hello" {
		t.Fatalf("text block = %v", text)
	}
	if tool["id"] != "toolu_1" || tool["name"] != "f" {
		t.Fatalf("tool block = %v", tool)
	}
	if decoded["stop_reason"] != "tool_use" {
		t.Fatalf("stop_reason = %v", decoded["stop_reason"])
	}
	usage, _ := decoded["usage"].(map[string]any)
	if usage["input_tokens"] != float64(3) || usage["output_tokens"] != float64(4) {
		t.Fatalf("usage = %v", usage)
	}
}

func TestCollapseStreamResponses(t *testing.T) {
	input := sxFrames(
		`{"type":"response.created","response":{"id":"resp_1","model":"gpt"}}`,
		`{"type":"response.output_text.delta","item_id":"msg_1","delta":"Hello"}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"f"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{}"}`,
		`{"type":"response.completed","response":{"output":[],"usage":{"input_tokens":2,"output_tokens":3}}}`,
	)
	document, err := CollapseStream(strings.NewReader(input), Responses, "m")
	if err != nil {
		t.Fatalf("CollapseStream error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("collapsed document is not JSON: %v", err)
	}
	if decoded["status"] != "completed" {
		t.Fatalf("status = %v", decoded["status"])
	}
	output, _ := decoded["output"].([]any)
	if len(output) != 2 {
		t.Fatalf("output = %v", output)
	}
	message, _ := output[0].(map[string]any)
	call, _ := output[1].(map[string]any)
	if message["type"] != "message" || call["type"] != "function_call" || call["name"] != "f" || call["arguments"] != "{}" {
		t.Fatalf("output = %v", output)
	}
	usage, _ := decoded["usage"].(map[string]any)
	if usage["input_tokens"] != float64(2) || usage["output_tokens"] != float64(3) {
		t.Fatalf("usage = %v", usage)
	}
}

func TestCollapseStreamErrors(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		protocol Protocol
		contains string
		is       error
	}{
		{
			name:     "an upstream error frame becomes the returned error",
			input:    sxFrames(`{"choices":[{"delta":{"content":"partial"}}]}`, `{"error":{"message":"boom","type":"rate_limit"}}`),
			protocol: Chat,
			contains: "boom",
		},
		{
			name:     "an error event with no message falls back",
			input:    sxFrames(`{"error":{}}`),
			protocol: Chat,
			contains: "upstream Chat stream error",
		},
		{
			name:     "an unparseable record",
			input:    sxFrames(`{broken`),
			protocol: Chat,
			contains: "invalid upstream SSE JSON",
		},
		{
			name:     "a truncated stream",
			input:    "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n",
			protocol: Chat,
			is:       errSSEUnexpectedEOF,
		},
		{
			name:     "an empty stream",
			input:    "",
			protocol: Chat,
			is:       errSSEUnexpectedEOF,
		},
		{
			name:     "a chat terminator on a responses stream",
			input:    sxFrames("[DONE]"),
			protocol: Responses,
			contains: "unexpected [DONE] terminator",
		},
		{
			name:     "an anthropic error event",
			input:    sxFrames(`{"type":"error","error":{"message":"overloaded","type":"overloaded_error"}}`),
			protocol: Anthropic,
			contains: "overloaded",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			document, err := CollapseStream(strings.NewReader(tc.input), tc.protocol, "m")
			if err == nil {
				t.Fatalf("CollapseStream succeeded with %s, want an error", document)
			}
			if document != nil {
				t.Fatalf("document = %s, want nil alongside an error", document)
			}
			if tc.is != nil && !errors.Is(err, tc.is) {
				t.Fatalf("error = %v, want %v", err, tc.is)
			}
			if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.contains)
			}
		})
	}
}

// TestCollapseStreamPhantomToolIsDemoted documents the collapse side of the
// phantom tool-call guard.
func TestCollapseStreamPhantomToolIsDemoted(t *testing.T) {
	input := sxFrames(
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"arguments":"{}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		"[DONE]",
	)
	document, err := CollapseStream(strings.NewReader(input), Chat, "m")
	if err != nil {
		t.Fatalf("CollapseStream error = %v", err)
	}
	if !strings.Contains(string(document), `"finish_reason":"stop"`) {
		t.Fatalf("document = %s, want the tool stop demoted", document)
	}
	if strings.Contains(string(document), `"tool_calls"`) {
		t.Fatalf("document = %s, want no tool call block", document)
	}
}

// TestCollapseStreamUsesTheRequestedModelByDefault documents that the model
// argument is used until an event overrides it.
func TestCollapseStreamUsesTheRequestedModelByDefault(t *testing.T) {
	document, err := CollapseStream(strings.NewReader(sxFrames("[DONE]")), Chat, "requested-model")
	if err != nil {
		t.Fatalf("CollapseStream error = %v", err)
	}
	if !strings.Contains(string(document), `"model":"requested-model"`) {
		t.Fatalf("document = %s", document)
	}
	if !strings.Contains(string(document), `"created_at"`) && !strings.Contains(string(document), `"created"`) {
		t.Fatalf("document = %s, want a creation timestamp", document)
	}
}
