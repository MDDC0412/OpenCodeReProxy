package telemetry

import (
	"testing"

	"opencode2api/internal/protocol"
)

func TestRequestSucceeded(t *testing.T) {
	cases := []struct {
		name   string
		status int
		meta   *RequestMeta
		want   bool
	}{
		{"200 without meta", 200, nil, true},
		{"200 with empty meta", 200, &RequestMeta{}, true},
		{"204 no content", 204, nil, true},
		{"399 is still success", 399, nil, true},
		{"199 is not success", 199, nil, false},
		{"200 is the lower success bound", 200, nil, true},
		{"400 is not success", 400, nil, false},
		{"500 is not success", 500, nil, false},
		{"0 is not success", 0, nil, false},
		// Any non-empty terminal outcome overrides the HTTP status, even when
		// the status says 2xx.
		{"stream_error demotes a 200", 200, &RequestMeta{Outcome: "stream_error"}, false},
		{"client_canceled demotes a 200", 200, &RequestMeta{Outcome: "client_canceled"}, false},
		{"any other outcome also demotes a 200", 200, &RequestMeta{Outcome: "success"}, false},
		{"attempt outcome alone does not demote", 200, &RequestMeta{AttemptOutcome: "stream_error"}, true},
		{"outcome on a 500 stays failed", 500, &RequestMeta{Outcome: "stream_error"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := requestSucceeded(tc.status, tc.meta); got != tc.want {
				t.Fatalf("requestSucceeded(%d, %+v) = %v, want %v", tc.status, tc.meta, got, tc.want)
			}
		})
	}
}

func TestRequestOutcome(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		channel string
		want    string
	}{
		{"empty channel is not routed regardless of status", 200, "", "not_routed"},
		{"not_routed channel wins over a success status", 200, "not_routed", "not_routed"},
		{"not_routed channel wins over a server error", 500, "not_routed", "not_routed"},
		{"200 on a real channel", 200, "zen", "success"},
		{"204 on a real channel", 204, "anonymous", "success"},
		{"302 on a real channel counts as success", 302, "zen", "success"},
		{"399 is still success", 399, "zen", "success"},
		{"400 is a client error", 400, "zen", "client_error"},
		{"404 is a client error", 404, "zen", "client_error"},
		{"499 is a client error", 499, "zen", "client_error"},
		{"500 is a server error", 500, "zen", "server_error"},
		{"503 is a server error", 503, "go", "server_error"},
		{"199 is unknown", 199, "zen", "unknown"},
		{"100 is unknown", 100, "zen", "unknown"},
		{"0 is unknown", 0, "zen", "unknown"},
		// Anything >= 500 is a server error, including impossible statuses.
		{"600 is a server error", 600, "zen", "server_error"},
		{"599 is a server error", 599, "zen", "server_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := requestOutcome(tc.status, tc.channel); got != tc.want {
				t.Fatalf("requestOutcome(%d, %q) = %q, want %q", tc.status, tc.channel, got, tc.want)
			}
		})
	}
}

func TestHistogramPercentile(t *testing.T) {
	cases := []struct {
		name      string
		histogram [11]uint64
		total     uint64
		p         float64
		want      uint64
	}{
		{
			name:      "single fast bucket",
			histogram: [11]uint64{1: 10},
			total:     10,
			p:         0.50,
			want:      100,
		},
		{
			name:      "p95 lands in a later bucket",
			histogram: [11]uint64{0: 90, 5: 10},
			total:     100,
			p:         0.95,
			want:      2500,
		},
		{
			name:      "p99 with everything in the last bucket overflows",
			histogram: [11]uint64{10: 5},
			total:     5,
			p:         0.99,
			want:      60001,
		},
		{
			name:      "histogram shorter than total returns zero",
			histogram: [11]uint64{0: 3},
			total:     10,
			p:         0.50,
			want:      0,
		},
		{
			name:      "percentile zero still returns the first bound, ignoring the histogram",
			histogram: [11]uint64{9: 7},
			total:     7,
			p:         0,
			want:      50,
		},
		{
			name:      "empty histogram with total zero returns the first bound, not zero",
			histogram: [11]uint64{},
			total:     0,
			p:         0.95,
			want:      50,
		},
		{
			name:      "histogram index maps to the matching latency bound",
			histogram: [11]uint64{1: 2},
			total:     2,
			p:         0.50,
			want:      100,
		},
		{
			name:      "exactly the overflow boundary is not overflow",
			histogram: [11]uint64{9: 1},
			total:     1,
			p:         0.99,
			want:      60000,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := histogramPercentile(tc.histogram, tc.total, tc.p); got != tc.want {
				t.Fatalf("histogramPercentile(%v, %d, %v) = %d, want %d", tc.histogram, tc.total, tc.p, got, tc.want)
			}
		})
	}
}

func TestTokenCounts(t *testing.T) {
	cases := []struct {
		name  string
		usage protocol.Usage
		want  TokenCounts
	}{
		{
			name:  "zero usage",
			usage: protocol.Usage{},
			want:  TokenCounts{},
		},
		{
			name:  "all fields",
			usage: protocol.Usage{Input: 10, Output: 20, Total: 30, Cached: 5, CacheCreation: 7, Reasoning: 3},
			want:  TokenCounts{Input: 10, Output: 20, Total: 30, Cached: 5, Reasoning: 3},
		},
		{
			name:  "cache creation is not represented in TokenCounts",
			usage: protocol.Usage{Input: 1, CacheCreation: 99},
			want:  TokenCounts{Input: 1},
		},
		{
			name:  "negative fields clamp to zero",
			usage: protocol.Usage{Input: -5, Output: -1, Total: -100, Cached: -2, Reasoning: -3},
			want:  TokenCounts{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tokenCounts(tc.usage); got != tc.want {
				t.Fatalf("tokenCounts(%+v) = %+v, want %+v", tc.usage, got, tc.want)
			}
		})
	}
}

func TestAddTokenCounts(t *testing.T) {
	target := TokenCounts{Input: 1, Output: 2, Cached: 3, Reasoning: 4, Total: 5}
	addTokenCounts(&target, TokenCounts{Input: 10, Output: 20, Cached: 30, Reasoning: 40, Total: 50})
	want := TokenCounts{Input: 11, Output: 22, Cached: 33, Reasoning: 44, Total: 55}
	if target != want {
		t.Fatalf("addTokenCounts = %+v, want %+v", target, want)
	}
	addTokenCounts(&target, TokenCounts{})
	if target != want {
		t.Fatalf("adding a zero value changed the target: %+v", target)
	}
}

func TestMergeCounts(t *testing.T) {
	cases := []struct {
		name   string
		target map[string]uint64
		source map[string]uint64
		want   map[string]uint64
	}{
		{
			name:   "disjoint keys",
			target: map[string]uint64{"a": 1},
			source: map[string]uint64{"b": 2},
			want:   map[string]uint64{"a": 1, "b": 2},
		},
		{
			name:   "same key accumulates",
			target: map[string]uint64{"a": 3},
			source: map[string]uint64{"a": 4},
			want:   map[string]uint64{"a": 7},
		},
		{
			name:   "nil source is a no-op",
			target: map[string]uint64{"a": 1},
			source: nil,
			want:   map[string]uint64{"a": 1},
		},
		{
			name:   "nil source leaves an empty target empty",
			target: map[string]uint64{},
			source: nil,
			want:   map[string]uint64{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mergeCounts(tc.target, tc.source)
			if len(tc.target) != len(tc.want) {
				t.Fatalf("mergeCounts = %v, want %v", tc.target, tc.want)
			}
			for key, value := range tc.want {
				if tc.target[key] != value {
					t.Fatalf("mergeCounts = %v, want %v", tc.target, tc.want)
				}
			}
		})
	}
}

func TestAddTokenMap(t *testing.T) {
	target := map[string]TokenCounts{}
	addTokenMap(target, "zen", TokenCounts{Input: 1, Total: 1})
	addTokenMap(target, "zen", TokenCounts{Output: 2})
	// addTokenMap skips empty keys entirely, so no "" entry is ever created.
	addTokenMap(target, "", TokenCounts{Input: 9})
	if len(target) != 1 {
		t.Fatalf("target = %v, want a single zen entry", target)
	}
	// Every field is summed independently; a partial update does not recompute Total.
	want := TokenCounts{Input: 1, Output: 2, Total: 1}
	if target["zen"] != want {
		t.Fatalf("target[zen] = %+v, want %+v", target["zen"], want)
	}
}

func TestMergeTokenMaps(t *testing.T) {
	cases := []struct {
		name   string
		target map[string]TokenCounts
		source map[string]TokenCounts
		want   map[string]TokenCounts
	}{
		{
			name:   "disjoint keys",
			target: map[string]TokenCounts{"a": {Input: 1, Total: 1}},
			source: map[string]TokenCounts{"b": {Output: 2, Total: 2}},
			want:   map[string]TokenCounts{"a": {Input: 1, Total: 1}, "b": {Output: 2, Total: 2}},
		},
		{
			name:   "same key accumulates every field",
			target: map[string]TokenCounts{"a": {Input: 1, Output: 2, Cached: 3, Reasoning: 4, Total: 10}},
			source: map[string]TokenCounts{"a": {Input: 5, Output: 6, Cached: 7, Reasoning: 8, Total: 26}},
			want:   map[string]TokenCounts{"a": {Input: 6, Output: 8, Cached: 10, Reasoning: 12, Total: 36}},
		},
		{
			name:   "empty keys are dropped",
			target: map[string]TokenCounts{},
			source: map[string]TokenCounts{"": {Input: 5}},
			want:   map[string]TokenCounts{},
		},
		{
			name:   "nil source is a no-op",
			target: map[string]TokenCounts{"a": {Input: 1}},
			source: nil,
			want:   map[string]TokenCounts{"a": {Input: 1}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mergeTokenMaps(tc.target, tc.source)
			if len(tc.target) != len(tc.want) {
				t.Fatalf("mergeTokenMaps = %+v, want %+v", tc.target, tc.want)
			}
			for key, value := range tc.want {
				if tc.target[key] != value {
					t.Fatalf("mergeTokenMaps[%q] = %+v, want %+v", key, tc.target[key], value)
				}
			}
		})
	}
}

func TestCloneUsagePeriodIsDeep(t *testing.T) {
	source := newUsagePeriod()
	source.Requests, source.Reported, source.Coverage = 2, 1, 0.5
	source.Tokens = TokenCounts{Input: 7}
	addTokenMap(source.Models, "m", TokenCounts{Input: 3})
	addTokenMap(source.Tiers, "zen", TokenCounts{Input: 3})

	clone := cloneUsagePeriod(source)
	if clone.Requests != 2 || clone.Reported != 1 || clone.Tokens != (TokenCounts{Input: 7}) {
		t.Fatalf("scalar fields not copied: %+v", clone)
	}
	// Coverage is deliberately not copied: it is recomputed by
	// finalizeUsagePeriod after the aggregate is assembled.
	if clone.Coverage != 0 {
		t.Fatalf("coverage = %v, want 0 before finalization", clone.Coverage)
	}
	finalizeUsagePeriod(&clone)
	if clone.Coverage != 0.5 {
		t.Fatalf("coverage = %v, want 0.5", clone.Coverage)
	}
	if clone.Models["m"] != (TokenCounts{Input: 3}) || clone.Tiers["zen"] != (TokenCounts{Input: 3}) {
		t.Fatalf("maps not copied: %+v", clone)
	}
	addTokenMap(clone.Models, "m", TokenCounts{Input: 100})
	addTokenMap(clone.Tiers, "zen", TokenCounts{Input: 100})
	if source.Models["m"].Input != 3 || source.Tiers["zen"].Input != 3 {
		t.Fatalf("clone shares maps with the source: %+v", source)
	}
}

func TestFinalizeUsagePeriod(t *testing.T) {
	cases := []struct {
		name   string
		period UsagePeriod
		want   float64
	}{
		{"no requests keeps zero coverage", UsagePeriod{}, 0},
		{"all reported", UsagePeriod{Requests: 4, Reported: 4}, 1},
		{"half reported", UsagePeriod{Requests: 4, Reported: 2}, 0.5},
		{"none reported", UsagePeriod{Requests: 3, Reported: 0}, 0},
		{"reported without requests is not divided", UsagePeriod{Reported: 5}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			finalizeUsagePeriod(&tc.period)
			if tc.period.Coverage != tc.want {
				t.Fatalf("coverage = %v, want %v", tc.period.Coverage, tc.want)
			}
		})
	}
}

func TestRecordAttemptCounts(t *testing.T) {
	var counts AttemptCounts
	recordAttemptCounts(&counts, true)
	recordAttemptCounts(&counts, false)
	recordAttemptCounts(&counts, false)
	want := AttemptCounts{Total: 3, Success: 1, Failed: 2}
	if counts != want {
		t.Fatalf("counts = %+v, want %+v", counts, want)
	}
}

func TestAddAttemptMap(t *testing.T) {
	target := map[string]AttemptCounts{}
	addAttemptMap(target, "zen", true)
	addAttemptMap(target, "zen", false)
	addAttemptMap(target, "", true)
	if len(target) != 1 {
		t.Fatalf("target = %v, want a single zen entry", target)
	}
	want := AttemptCounts{Total: 2, Success: 1, Failed: 1}
	if target["zen"] != want {
		t.Fatalf("target[zen] = %+v, want %+v", target["zen"], want)
	}
}

func TestRecordAttemptAggregate(t *testing.T) {
	aggregate := newAttemptAggregate()
	recordAttemptAggregate(&aggregate, UpstreamAttempt{Tier: "zen", Channel: "anonymous", KeyID: "public", Success: true})
	recordAttemptAggregate(&aggregate, UpstreamAttempt{Tier: "zen", Channel: "anonymous", KeyID: "public", Success: false})
	// Attempts without a tier/channel/key still count in the total.
	recordAttemptAggregate(&aggregate, UpstreamAttempt{Success: false})

	wantTotals := AttemptCounts{Total: 3, Success: 1, Failed: 2}
	if aggregate.AttemptCounts != wantTotals {
		t.Fatalf("totals = %+v, want %+v", aggregate.AttemptCounts, wantTotals)
	}
	// The per-key maps skip the empty key, so they only carry the two keyed attempts.
	wantKeyed := AttemptCounts{Total: 2, Success: 1, Failed: 1}
	if aggregate.Tiers["zen"] != wantKeyed {
		t.Fatalf("tiers[zen] = %+v, want %+v", aggregate.Tiers["zen"], wantKeyed)
	}
	if aggregate.Channels["anonymous"] != wantKeyed {
		t.Fatalf("channels[anonymous] = %+v, want %+v", aggregate.Channels["anonymous"], wantKeyed)
	}
	if aggregate.Keys["public"] != wantKeyed {
		t.Fatalf("keys[public] = %+v, want %+v", aggregate.Keys["public"], wantKeyed)
	}
	if len(aggregate.Tiers) != 1 || len(aggregate.Channels) != 1 || len(aggregate.Keys) != 1 {
		t.Fatalf("empty keys must not be recorded: %+v", aggregate)
	}
}

func TestMergeAttemptAggregate(t *testing.T) {
	source := newAttemptAggregate()
	recordAttemptAggregate(&source, UpstreamAttempt{Tier: "zen", Channel: "anonymous", KeyID: "k1", Success: true})
	recordAttemptAggregate(&source, UpstreamAttempt{Tier: "go", Channel: "key", KeyID: "k2", Success: false})

	target := newAttemptAggregate()
	recordAttemptAggregate(&target, UpstreamAttempt{Tier: "zen", Channel: "anonymous", KeyID: "k1", Success: true})
	mergeAttemptAggregate(&target, source)

	want := AttemptCounts{Total: 3, Success: 2, Failed: 1}
	if target.AttemptCounts != want {
		t.Fatalf("totals = %+v, want %+v", target.AttemptCounts, want)
	}
	// The pre-existing zen attempt plus the merged zen attempt.
	if target.Tiers["zen"] != (AttemptCounts{Total: 2, Success: 2, Failed: 0}) {
		t.Fatalf("tiers[zen] = %+v, want %+v", target.Tiers["zen"], AttemptCounts{Total: 2, Success: 2})
	}
	if target.Tiers["go"] != (AttemptCounts{Total: 1, Failed: 1}) {
		t.Fatalf("tiers[go] = %+v", target.Tiers["go"])
	}
	if target.Channels["key"] != (AttemptCounts{Total: 1, Failed: 1}) {
		t.Fatalf("channels[key] = %+v", target.Channels["key"])
	}
	if target.Keys["k2"] != (AttemptCounts{Total: 1, Failed: 1}) {
		t.Fatalf("keys[k2] = %+v", target.Keys["k2"])
	}
}

func TestCloneAttemptAggregateIsDeep(t *testing.T) {
	source := newAttemptAggregate()
	recordAttemptAggregate(&source, UpstreamAttempt{Tier: "zen", Channel: "anonymous", KeyID: "k1", Success: true})

	clone := cloneAttemptAggregate(source)
	if clone.AttemptCounts != source.AttemptCounts || clone.Tiers["zen"] != source.Tiers["zen"] {
		t.Fatalf("clone = %+v, want %+v", clone, source)
	}
	recordAttemptAggregate(&clone, UpstreamAttempt{Tier: "zen", Channel: "anonymous", KeyID: "k1", Success: true})
	if source.Tiers["zen"].Total != 1 {
		t.Fatalf("clone shares maps with the source: %+v", source)
	}
}

func TestFinalizeAttemptAggregate(t *testing.T) {
	cases := []struct {
		name      string
		aggregate AttemptAggregate
		want      float64
	}{
		{"empty stays zero", AttemptAggregate{}, 0},
		{"all successful", AttemptAggregate{AttemptCounts: AttemptCounts{Total: 4, Success: 4}}, 1},
		{"half successful", AttemptAggregate{AttemptCounts: AttemptCounts{Total: 4, Success: 2}}, 0.5},
		{"none successful", AttemptAggregate{AttemptCounts: AttemptCounts{Total: 3, Failed: 3}}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			finalizeAttemptAggregate(&tc.aggregate)
			if tc.aggregate.SuccessRate != tc.want {
				t.Fatalf("success rate = %v, want %v", tc.aggregate.SuccessRate, tc.want)
			}
		})
	}
}

func TestMetricBucketReset(t *testing.T) {
	bucket := metricBucket{
		minute: 1, total: 9, success: 5, errors: 4, duration: 1000,
		histogram: [11]uint64{0: 9}, endpoints: map[string]uint64{"/v1/x": 9},
		models: map[string]uint64{"m": 9}, tiers: map[string]uint64{"zen": 9}, statuses: map[string]uint64{"200": 9},
		usageRequests: 9, usageReported: 9, tokens: TokenCounts{Input: 9},
		usageModels: map[string]TokenCounts{"m": {Input: 9}}, usageTiers: map[string]TokenCounts{"zen": {Input: 9}},
	}
	bucket.reset(42)
	if bucket.minute != 42 {
		t.Fatalf("minute = %d, want 42", bucket.minute)
	}
	if bucket.total != 0 || bucket.success != 0 || bucket.errors != 0 || bucket.duration != 0 {
		t.Fatalf("counters not cleared: %+v", bucket)
	}
	if bucket.histogram != ([11]uint64{}) || bucket.tokens != (TokenCounts{}) {
		t.Fatalf("histogram/tokens not cleared: %+v", bucket)
	}
	if len(bucket.endpoints) != 0 || len(bucket.models) != 0 || len(bucket.tiers) != 0 || len(bucket.statuses) != 0 ||
		len(bucket.usageModels) != 0 || len(bucket.usageTiers) != 0 {
		t.Fatalf("maps not cleared: %+v", bucket)
	}
	// The maps must be usable immediately after the reset.
	bucket.endpoints["/v1/y"] = 1
	addTokenMap(bucket.usageModels, "m", TokenCounts{Output: 1})
	if bucket.endpoints["/v1/y"] != 1 || bucket.usageModels["m"].Output != 1 {
		t.Fatalf("maps are not usable after reset: %+v", bucket)
	}
}
