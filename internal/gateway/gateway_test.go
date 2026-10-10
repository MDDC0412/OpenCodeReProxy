package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestQuotaError(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"429 always matches regardless of body", 429, "", true},
		{"429 with an unrelated body", 429, `{"error":"anything"}`, true},
		{"insufficient_quota", 400, `{"error":{"code":"insufficient_quota"}}`, true},
		{"quota_exceeded", 400, "quota_exceeded", true},
		{"quota exceeded", 400, "Quota Exceeded for this key", true},
		{"quota exhausted", 400, "your quota exhausted", true},
		{"rate_limit", 400, `{"type":"rate_limit_error"}`, true},
		{"rate limit", 429, "Rate limit reached", true},
		{"rate-limit hyphenated", 400, "rate-limit", true},
		{"usage_limit", 400, "usage_limit", true},
		{"usage limit", 400, "Usage Limit reached", true},
		{"insufficient balance", 402, "insufficient balance", true},
		{"credit balance is too low", 402, "Your credit balance is too low", true},
		{"chinese insufficient quota", 400, "额度不足", true},
		{"chinese quota exhausted", 400, "配额已用尽", true},
		// Matching is substring-based and case-insensitive, and only 429 is
		// status-gated: a 2xx body that mentions a marker still counts.
		{"2xx body with a marker still matches", 200, "rate limit", true},
		{"500 body with a marker still matches", 500, "insufficient_quota", true},
		{"ordinary provider error does not match", 400, `{"error":{"message":"invalid api key"}}`, false},
		{"plain server error does not match", 500, "internal server error", false},
		{"empty body and non-429 status", 400, "", false},
		{"near miss on quota", 400, "quota available", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := quotaError(tc.status, tc.body); got != tc.want {
				t.Fatalf("quotaError(%d, %q) = %v, want %v", tc.status, tc.body, got, tc.want)
			}
		})
	}
}

func TestIsModelUnavailable(t *testing.T) {
	markers := []string{
		"model_not_found",
		"model_not_available",
		"model_retired",
		"model_deprecated",
		"unknown model",
		"model does not exist",
		"model has been retired",
		"model is no longer available",
	}
	for _, status := range []int{400, 404, 410, 422} {
		for _, marker := range markers {
			t.Run(fmt.Sprintf("%d/%s", status, marker), func(t *testing.T) {
				if !isModelUnavailable(status, `{"error":"`+marker+`"}`) {
					t.Fatalf("isModelUnavailable(%d, %q) = false, want true", status, marker)
				}
				if !isModelUnavailable(status, strings.ToUpper(marker)) {
					t.Fatalf("isModelUnavailable is expected to be case insensitive for %q", marker)
				}
			})
		}
	}
	// The same markers outside the four allowed statuses never match.
	for _, status := range []int{200, 401, 403, 429, 500, 503} {
		t.Run(fmt.Sprintf("status %d is ignored", status), func(t *testing.T) {
			if isModelUnavailable(status, "model_not_found") {
				t.Fatalf("isModelUnavailable(%d, ...) = true, want false", status)
			}
		})
	}

	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"empty body", 404, "", false},
		{"unrelated 404", 404, `{"error":"not found"}`, false},
		{"unrelated 400", 400, `{"error":"invalid request"}`, false},
		{"realistic message without an inline model id", 400, "The model does not exist for this account", true},
		// Markers must appear contiguously: an interleaved model id breaks the
		// substring match even though the meaning is the same.
		{"backticked model id breaks the marker", 400, "The model `gpt-5` does not exist for this account", false},
		{"marker inside a longer message", 422, "upstream says: model_not_found, retry later", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isModelUnavailable(tc.status, tc.body); got != tc.want {
				t.Fatalf("isModelUnavailable(%d, %q) = %v, want %v", tc.status, tc.body, got, tc.want)
			}
		})
	}
}

func TestIsEffortRejection(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"400 invalid effort", 400, `{"error":{"message":"invalid effort: max"}}`, true},
		{"400 unsupported reasoning effort", 400, "unsupported reasoning effort requested", true},
		{"400 not support", 400, "model does not support this thinking level", true},
		{"400 not allowed", 400, "reasoning effort not allowed for this model", true},
		{"400 must be", 400, "reasoning_effort must be one of low, high", true},
		{"400 allowed values", 400, "thinking: allowed values are minimal, low", true},
		{"422 also matches", 422, "invalid effort", true},
		{"case insensitive", 400, "INVALID EFFORT", true},
		// Both a reasoning keyword and a rejection marker are required.
		{"400 missing the rejection marker", 400, "invalid request: effort unavailable", true},
		{"400 effort without a rejection marker", 400, `{"error":"effort accepted"}`, false},
		{"400 thinking without a rejection marker", 400, "thinking enabled", false},
		{"400 marker without a reasoning keyword", 400, "invalid api key", false},
		{"400 unknown field", 400, `{"error":"unknown field"}`, false},
		// Only 400 and 422 are inspected.
		{"404 is ignored", 404, "invalid effort", false},
		{"500 is ignored", 500, "unsupported reasoning effort", false},
		{"200 is ignored", 200, "invalid effort", false},
		{"422 without a marker", 422, "effort", false},
		{"empty body", 400, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isEffortRejection(tc.status, tc.body); got != tc.want {
				t.Fatalf("isEffortRejection(%d, %q) = %v, want %v", tc.status, tc.body, got, tc.want)
			}
		})
	}
}

func TestIsStaleReasoningReference(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"nil body", "", false},
		{"canonical upstream message", `{"error":{"message":"Referenced reasoning item 'rs_abc' was not found or has expired"}}`, true},
		{"reasoning reference phrasing", "reasoning reference rs_1 no longer exists", true},
		{"expiry only", "reasoning item rs_1 has expirated server side", true},
		{"does not exist", "reasoning item rs_1 does not exist", true},
		{"case insensitive", "REASONING ITEM 'RS_1' WAS NOT FOUND", true},
		// The doc-comment cases: a generic validation error mentioning reasoning
		// must not match because neither phrase nor gone-marker is present.
		{"generic unknown reasoning field", "unknown reasoning field in payload", false},
		{"reasoning mentioned but nothing is gone", "reasoning item rs_1 is invalid", false},
		{"phrase without a gone marker", "reasoning item rs_1 rejected", false},
		{"gone marker without the phrase", "the referenced object was not found", false},
		{"unrelated body", `{"error":"invalid request"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isStaleReasoningReference([]byte(tc.body)); got != tc.want {
				t.Fatalf("isStaleReasoningReference(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

func TestUpstreamStatus(t *testing.T) {
	cases := []struct {
		name string
		resp *http.Response
		want int
	}{
		{"nil response", nil, 0},
		{"200", &http.Response{StatusCode: http.StatusOK}, 200},
		{"503", &http.Response{StatusCode: http.StatusServiceUnavailable}, 503},
		{"zero valued response", &http.Response{}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := upstreamStatus(tc.resp); got != tc.want {
				t.Fatalf("upstreamStatus = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestIsNonRetryableClientResponse(t *testing.T) {
	cases := []struct {
		name   string
		status int
		err    error
		resp   bool
		want   bool
	}{
		{"no response and no error", 0, nil, false, false},
		{"any error wins", 0, errors.New("transport"), true, false},
		{"error with a response", 400, errors.New("transport"), true, false},
		{"400", 400, nil, true, true},
		{"404", 404, nil, true, true},
		{"422", 422, nil, true, true},
		{"499", 499, nil, true, true},
		// 401/403/429 stay retryable so the next key or lane can be tried.
		{"401", 401, nil, true, false},
		{"403", 403, nil, true, false},
		{"429", 429, nil, true, false},
		{"200", 200, nil, true, false},
		{"500", 500, nil, true, false},
		{"502", 502, nil, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var resp *http.Response
			if tc.resp {
				resp = &http.Response{StatusCode: tc.status}
			}
			if got := isNonRetryableClientResponse(resp, tc.err); got != tc.want {
				t.Fatalf("isNonRetryableClientResponse(status=%d, err=%v) = %v, want %v", tc.status, tc.err, got, tc.want)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	future := time.Now().Add(90 * time.Second).UTC().Truncate(time.Second)
	past := time.Now().Add(-90 * time.Second).UTC().Truncate(time.Second)

	cases := []struct {
		name     string
		value    string
		want     time.Duration
		min, max time.Duration // inclusive lower bound, exclusive upper bound (0 = exact match)
	}{
		{name: "absent header", value: "", want: 0},
		{name: "whitespace only", value: "   ", want: 0},
		{name: "seconds integer", value: "120", want: 2 * time.Minute},
		{name: "seconds integer with surrounding whitespace", value: " 30 ", want: 30 * time.Second},
		{name: "single second", value: "1", want: time.Second},
		{name: "zero seconds is not a delay", value: "0", want: 0},
		{name: "negative seconds is not a delay", value: "-5", want: 0},
		{name: "fractional seconds is not an integer", value: "1.5", want: 0},
		{name: "non numeric garbage", value: "soon", want: 0},
		{name: "whitespace garbage", value: " retry later ", want: 0},
		{name: "rfc3339 is not an http date", value: future.Format(time.RFC3339), want: 0},
		{name: "past http date clamps to zero", value: past.Format(http.TimeFormat), want: 0},
		{name: "far past http date clamps to zero", value: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(http.TimeFormat), want: 0},
		{name: "future http date", value: future.Format(http.TimeFormat), min: 89 * time.Second, max: 91 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRetryAfter(tc.value)
			if tc.min == 0 {
				if got != tc.want {
					t.Fatalf("parseRetryAfter(%q) = %v, want %v", tc.value, got, tc.want)
				}
				return
			}
			if got < tc.min || got >= tc.max {
				t.Fatalf("parseRetryAfter(%q) = %v, want within [%v,%v)", tc.value, got, tc.min, tc.max)
			}
		})
	}
}

func TestParseRetryAfterAbsentHeader(t *testing.T) {
	header := http.Header{}
	if got := parseRetryAfter(header.Get("Retry-After")); got != 0 {
		t.Fatalf("parseRetryAfter(missing header) = %v, want 0", got)
	}
	header.Set("Retry-After", "45")
	if got := parseRetryAfter(header.Get("Retry-After")); got != 45*time.Second {
		t.Fatalf("parseRetryAfter(header) = %v, want 45s", got)
	}
}

type timeoutError struct{ timeout bool }

func (e timeoutError) Error() string { return "synthetic timeout error" }
func (e timeoutError) Timeout() bool { return e.timeout }

func TestIsProxyFailure(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"ordinary error", errors.New("connection reset by peer"), false},
		{"context canceled is not a proxy failure", context.Canceled, false},
		{"context deadline exceeded", context.DeadlineExceeded, true},
		{"wrapped deadline exceeded", fmt.Errorf("read body: %w", context.DeadlineExceeded), true},
		{"connection refused", syscall.ECONNREFUSED, true},
		{"wrapped connection refused", fmt.Errorf("dial: %w", syscall.ECONNREFUSED), true},
		{"timeout implementing error", timeoutError{timeout: true}, true},
		{"wrapped timeout implementing error", fmt.Errorf("proxy: %w", timeoutError{timeout: true}), true},
		{"error with Timeout() false", timeoutError{timeout: false}, false},
		// errFirstEventTimeout wraps context.DeadlineExceeded but means the
		// upstream answered slowly, so the proxy must be kept.
		{"first event timeout", errFirstEventTimeout, false},
		{"wrapped first event timeout", fmt.Errorf("stream: %w", errFirstEventTimeout), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isProxyFailure(tc.err); got != tc.want {
				t.Fatalf("isProxyFailure(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
