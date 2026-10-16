package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"opencode2api/internal/config"
	"opencode2api/internal/telemetry"
)

const testKeyA = "sk-local-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func newTestConfig(keys ...string) config.Config {
	return config.Config{ServerKeys: keys}
}

// testKeyViews exercises the view builder without a full runtime manager; the
// usage counters come from a fresh monitor, matching a just-started process.
func testKeyViews(cfg config.Config) []APIKeyView {
	return (&Server{monitor: telemetry.NewMonitor()}).apiKeyViews(cfg)
}

func TestAPIName(t *testing.T) {
	cfg := config.Config{
		ServerKeys: []string{testKeyA, "sk-local-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		ServerKeyMetadata: map[string]config.ServerKeyMetadata{
			config.Fingerprint(testKeyA): {Name: "Claude Code"},
		},
	}
	if got := apiKeyName(cfg, config.Fingerprint(testKeyA), 0); got != "Claude Code" {
		t.Fatalf("named key: got %q", got)
	}
	// Keys without metadata fall back to a positional label.
	if got := apiKeyName(cfg, config.Fingerprint("sk-local-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), 1); got != "已有 Key 2" {
		t.Fatalf("unnamed key: got %q", got)
	}
}

func TestAPIKeyViews(t *testing.T) {
	cfg := newTestConfig(testKeyA)
	cfg.ServerKeyMetadata = map[string]config.ServerKeyMetadata{
		config.Fingerprint(testKeyA): {Name: "Claude Code", CreatedAt: time.Now().UTC(), Disabled: true},
	}
	views := testKeyViews(cfg)
	if len(views) != 1 {
		t.Fatalf("want 1 view, got %d", len(views))
	}
	if views[0].Name != "Claude Code" || views[0].Enabled || views[0].Display == testKeyA {
		t.Fatalf("unexpected view: %+v", views[0])
	}
	if views[0].CreatedAt == nil {
		t.Fatal("CreatedAt should be propagated when metadata carries it")
	}
	// A zero CreatedAt must stay absent rather than surface as year 1.
	cfg.ServerKeyMetadata[config.Fingerprint(testKeyA)] = config.ServerKeyMetadata{Name: "x"}
	if views = testKeyViews(cfg); views[0].CreatedAt != nil {
		t.Fatal("zero CreatedAt should not be reported")
	}
	if got := testKeyViews(config.Config{}); len(got) != 0 {
		t.Fatalf("empty config should yield no views, got %d", len(got))
	}
}

func TestValidateKeyName(t *testing.T) {
	if err := validateKeyName("x"); err != nil {
		t.Fatalf("single char should be allowed: %v", err)
	}
	if err := validateKeyName(strings.Repeat("名", 80)); err != nil {
		t.Fatalf("80 runes should be allowed: %v", err)
	}
	if err := validateKeyName(""); err == nil {
		t.Fatal("empty name should be rejected")
	}
	if err := validateKeyName(strings.Repeat("名", 81)); err == nil {
		t.Fatal("81 runes should be rejected")
	}
}

func TestAddAPIKey(t *testing.T) {
	t.Run("appends key and metadata", func(t *testing.T) {
		cfg := newTestConfig()
		if err := addAPIKey(&cfg, " Claude Code ", testKeyA); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(cfg.ServerKeys) != 1 || cfg.ServerKeys[0] != testKeyA {
			t.Fatalf("key not appended: %+v", cfg.ServerKeys)
		}
		// Names are trimmed on the way in by the handler, not by addAPIKey.
		if got := cfg.ServerKeyMetadata[config.Fingerprint(testKeyA)].Name; got != " Claude Code " {
			t.Fatalf("addAPIKey should not trim, got %q", got)
		}
	})

	invalid := []struct {
		name, key string
	}{
		{"too short", "short-key"},
		{"illegal char", "sk-local-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa!"},
		{"space inside", "sk-local-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa b"},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestConfig()
			if err := addAPIKey(&cfg, "n", tc.key); err == nil {
				t.Fatal("expected rejection")
			}
			if len(cfg.ServerKeys) != 0 {
				t.Fatal("rejected key must not be stored")
			}
		})
	}

	t.Run("rejects duplicate", func(t *testing.T) {
		cfg := newTestConfig(testKeyA)
		if err := addAPIKey(&cfg, "other", testKeyA); err == nil {
			t.Fatal("duplicate key should be rejected")
		}
		if len(cfg.ServerKeys) != 1 {
			t.Fatal("duplicate must not grow the list")
		}
	})

	t.Run("rejects empty name", func(t *testing.T) {
		cfg := newTestConfig()
		if err := addAPIKey(&cfg, "", testKeyA); err == nil {
			t.Fatal("empty name should be rejected")
		}
	})

	t.Run("enforces 200 key ceiling", func(t *testing.T) {
		cfg := newTestConfig()
		for i := range 200 {
			cfg.ServerKeys = append(cfg.ServerKeys, keyForIndex(i))
		}
		if err := addAPIKey(&cfg, "one too many", "sk-local-zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"); err == nil {
			t.Fatal("expected ceiling error")
		}
	})
}

func TestChangeAPIKey(t *testing.T) {
	id := config.Fingerprint(testKeyA)

	t.Run("rename", func(t *testing.T) {
		cfg := newTestConfig(testKeyA)
		name := "  Renamed  "
		if err := changeAPIKey(&cfg, id, &name, nil, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := cfg.ServerKeyMetadata[id].Name; got != "Renamed" {
			t.Fatalf("rename should trim, got %q", got)
		}
	})

	t.Run("disable then enable", func(t *testing.T) {
		cfg := newTestConfig(testKeyA)
		off := false
		// Disabling the only key is refused: the gateway would lock everyone out.
		if err := changeAPIKey(&cfg, id, nil, &off, false); err == nil {
			t.Fatal("disabling the last enabled key should be refused")
		}
	})

	t.Run("keep one enabled while deleting another", func(t *testing.T) {
		second := "sk-local-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		cfg := newTestConfig(testKeyA, second)
		if err := changeAPIKey(&cfg, config.Fingerprint(second), nil, nil, true); err != nil {
			t.Fatalf("deleting a non-last key should succeed: %v", err)
		}
		if len(cfg.ServerKeys) != 1 {
			t.Fatalf("want 1 remaining key, got %d", len(cfg.ServerKeys))
		}
		if _, ok := cfg.ServerKeyMetadata[config.Fingerprint(second)]; ok {
			t.Fatal("metadata for deleted key should be removed")
		}
	})

	t.Run("reject deleting the last key", func(t *testing.T) {
		cfg := newTestConfig(testKeyA)
		if err := changeAPIKey(&cfg, id, nil, nil, true); err == nil {
			t.Fatal("deleting the only key should be refused")
		}
	})

	t.Run("unknown id", func(t *testing.T) {
		cfg := newTestConfig(testKeyA)
		if err := changeAPIKey(&cfg, "deadbeef", nil, nil, false); err == nil {
			t.Fatal("unknown id should be rejected")
		}
	})

	t.Run("invalid rename", func(t *testing.T) {
		cfg := newTestConfig(testKeyA)
		empty := "   "
		if err := changeAPIKey(&cfg, id, &empty, nil, false); err == nil {
			t.Fatal("blank rename should be rejected")
		}
	})
}

func TestCCSwitchEndpoint(t *testing.T) {
	cases := []struct{ base, suffix, want string }{
		{"http://h:8080", "", "http://h:8080"},
		{"http://h:8080", "/v1", "http://h:8080/v1"},
		// Idempotent: a base that already carries /v1 must not become /v1/v1.
		{"http://h:8080/v1", "/v1", "http://h:8080/v1"},
	}
	for _, tc := range cases {
		if got := ccswitchEndpoint(tc.base, tc.suffix); got != tc.want {
			t.Errorf("ccswitchEndpoint(%q,%q) = %q, want %q", tc.base, tc.suffix, got, tc.want)
		}
	}
}

func TestCCSwitchTargets(t *testing.T) {
	// Claude writes ANTHROPIC_BASE_URL and appends /v1/messages itself.
	if suffix := ccswitchTargets["claude"].suffix; suffix != "" {
		t.Errorf("claude should not get a suffix, got %q", suffix)
	}
	// Codex appends /responses and OpenCode appends /chat/completions.
	for _, app := range []string{"codex", "opencode"} {
		target, ok := ccswitchTargets[app]
		if !ok {
			t.Fatalf("%s missing from targets", app)
		}
		if target.suffix != "/v1" {
			t.Errorf("%s suffix = %q, want /v1", app, target.suffix)
		}
	}
	// The gateway speaks no native Gemini protocol, so it is not offered.
	for _, app := range []string{"gemini", "grokbuild", "openclaw"} {
		if _, ok := ccswitchTargets[app]; ok {
			t.Errorf("%s must not be an import target", app)
		}
	}
}

func TestValidateImportBase(t *testing.T) {
	valid := []struct{ in, want string }{
		{"http://127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"https://gw.example.com", "https://gw.example.com"},
		// A trailing slash is trimmed so the /v1 suffix cannot double up.
		{"http://127.0.0.1:8080/", "http://127.0.0.1:8080"},
	}
	for _, tc := range valid {
		got, err := validateImportBase(tc.in)
		if err != nil {
			t.Errorf("validateImportBase(%q) = %v, want nil", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("validateImportBase(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	invalid := []struct{ name, in string }{
		{"empty", ""},
		{"javascript scheme", "javascript:alert(1)"},
		{"file scheme", "file:///etc/passwd"},
		{"path", "http://127.0.0.1:8080/v1"},
		{"userinfo", "http://user:pw@127.0.0.1:8080"},
		{"no host", "http://"},
		{"query", "http://127.0.0.1:8080?a=1"},
	}
	for _, tc := range invalid {
		if got, err := validateImportBase(tc.in); err == nil {
			t.Errorf("%s: validateImportBase(%q) = %q, want error", tc.name, tc.in, got)
		}
	}
}

func TestValidateImportModel(t *testing.T) {
	if got, err := validateImportModel(" glm-5.3-flash "); err != nil || got != "glm-5.3-flash" {
		t.Errorf("valid model = %q, %v", got, err)
	}
	for _, model := range []string{"", "   ", "has space", "tab\there", strings.Repeat("a", 121)} {
		if _, err := validateImportModel(model); err == nil {
			t.Errorf("validateImportModel(%q) should fail", model)
		}
	}
}

func TestDecodeAdminJSON(t *testing.T) {
	t.Run("accepts valid body", func(t *testing.T) {
		var target struct {
			Name string `json:"name"`
		}
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ok"}`))
		if err := decodeAdminJSON(httptest.NewRecorder(), r, &target); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if target.Name != "ok" {
			t.Fatalf("got %q", target.Name)
		}
	})

	rejects := []struct{ name, body string }{
		{"unknown field", `{"nope":1}`},
		{"malformed", `{`},
		{"trailing value", `{"name":"a"} {"name":"b"}`},
	}
	for _, tc := range rejects {
		t.Run(tc.name, func(t *testing.T) {
			var target struct {
				Name string `json:"name"`
			}
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			if err := decodeAdminJSON(httptest.NewRecorder(), r, &target); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestResolveSecrets(t *testing.T) {
	existing := []string{testKeyA, "socks5://127.0.0.1:1080"}

	kept, err := resolveSecrets([]SecretInput{{ID: config.Fingerprint(existing[0])}}, existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kept) != 1 || kept[0] != existing[0] {
		t.Fatalf("kept secret not resolved: %+v", kept)
	}

	// An explicit value always wins over an id reference.
	both, err := resolveSecrets([]SecretInput{{ID: config.Fingerprint(existing[0]), Value: " brand-new "}}, existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if both[0] != "brand-new" {
		t.Fatalf("value should win and be trimmed, got %q", both[0])
	}

	// Order follows the request, not the stored order.
	reordered, err := resolveSecrets([]SecretInput{
		{Value: "first"},
		{ID: config.Fingerprint(existing[1])},
	}, existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reordered) != 2 || reordered[0] != "first" || reordered[1] != existing[1] {
		t.Fatalf("unexpected order: %+v", reordered)
	}

	if _, err := resolveSecrets([]SecretInput{{ID: "stale"}}, existing); err == nil {
		t.Fatal("unknown id should be rejected")
	}
	if _, err := resolveSecrets([]SecretInput{{}}, existing); err == nil {
		t.Fatal("empty item should be rejected")
	}
}

// changeAPIKey enforces the "keep one enabled" rule only by returning an error.
// It mutates the config it is handed before checking that rule, which is safe
// because RuntimeManager.Update applies changes to a copy of the live config
// and discards it when change returns an error.
func TestChangeAPIKeyGuardReliesOnCallerCopying(t *testing.T) {
	cfg := newTestConfig(testKeyA)
	if err := changeAPIKey(&cfg, config.Fingerprint(testKeyA), nil, nil, true); err == nil {
		t.Fatal("expected the last-key guard to fire")
	}
	// The error itself is the contract; the mutated copy is the caller's problem.
	if err := changeAPIKey(&cfg, "nope", nil, nil, false); err == nil {
		t.Fatal("unknown id should still be rejected")
	}
}

func TestMaskSecretsKeepsProxiesRedacted(t *testing.T) {
	values := []string{testKeyA, "http://user:hunter2@127.0.0.1:7890"}

	plain := maskSecrets(values, false)
	if plain[0].Display == values[0] {
		t.Fatal("key display must be masked")
	}
	if strings.Contains(plain[1].Display, "hunter2") {
		t.Fatalf("proxy credential leaked in non-proxy mode: %s", plain[1].Display)
	}

	// Proxy mode routes every entry through URL redaction.
	proxied := maskSecrets(values, true)
	if strings.Contains(proxied[1].Display, "hunter2") {
		t.Fatalf("proxy credential leaked: %s", proxied[1].Display)
	}
	if proxied[0].Display != proxied[1].Display && !strings.Contains(proxied[0].Display, testKeyA) {
		t.Fatalf("unexpected proxy-mode key display: %s", proxied[0].Display)
	}
}

func TestClassifyKeyTest(t *testing.T) {
	cases := []struct {
		status  int
		outcome string
		want    string
	}{
		{200, "", "usable"},
		{299, "", "usable"},
		{401, "", "rejected"},
		{403, "", "rejected"},
		{429, "", "rate_limited"},
		{0, "", "unavailable"},
		{500, "", "upstream_error"},
		{503, "", "upstream_error"},
		// A transport failure outranks whatever status came with it.
		{401, "transport_error", "transport_error"},
		// Any other 4xx says nothing about the credential itself.
		{400, "", "request_error"},
		{404, "", "request_error"},
	}
	for _, tc := range cases {
		if got := classifyKeyTest(tc.status, tc.outcome); got != tc.want {
			t.Errorf("classifyKeyTest(%d,%q) = %q, want %q", tc.status, tc.outcome, got, tc.want)
		}
	}
}

// sensitiveDebugKey compares against lowercase hints without normalizing the
// key, so matching depends on the caller passing a lowercase field name.
func TestSensitiveDebugKey(t *testing.T) {
	for _, key := range []string{"authorization", "cookie", "password", "my_password", "x-api-key", "access_token"} {
		if !sensitiveDebugKey(key) {
			t.Errorf("%q should be treated as sensitive", key)
		}
	}
	// Case matters: these are not normalized before matching.
	for _, key := range []string{"Authorization", "MY_PASSWORD", "Set-Cookie", "model", "max_tokens"} {
		if sensitiveDebugKey(key) {
			t.Errorf("%q should not be treated as sensitive (no case folding)", key)
		}
	}
}

// clientIP reports the peer address. A forwarded header is only believed when
// the peer is loopback, i.e. a reverse proxy running on the same host.
func TestClientIP(t *testing.T) {
	t.Run("remote peer wins over a spoofed header", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = "10.0.0.9:5555"
		r.Header.Set("X-Forwarded-For", "203.0.113.7, 70.41.3.18")
		if got := clientIP(r); got != "10.0.0.9" {
			t.Fatalf("clientIP = %q, a remote client must not spoof the rate-limit key", got)
		}
	})

	t.Run("loopback proxy is trusted", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = "127.0.0.1:5555"
		r.Header.Set("X-Forwarded-For", "203.0.113.7, 70.41.3.18")
		if got := clientIP(r); got != "70.41.3.18" {
			t.Fatalf("clientIP = %q, want the first untrusted hop from the right", got)
		}
	})

	t.Run("single forwarded entry is trimmed", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = "127.0.0.1:5555"
		r.Header.Set("X-Forwarded-For", " 203.0.113.7 ")
		if got := clientIP(r); got != "203.0.113.7" {
			t.Fatalf("clientIP = %q", got)
		}
	})
	t.Run("spoofed prefix cannot replace the appended peer", func(t *testing.T) {
		for _, prefix := range []string{"203.0.113.7", "203.0.113.8", "not-an-ip"} {
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r.RemoteAddr = "127.0.0.1:5555"
			r.Header.Set("X-Forwarded-For", prefix+", 198.51.100.9, ::1")
			if got := clientIP(r); got != "198.51.100.9" {
				t.Fatalf("spoofed prefix changed identity: %q", got)
			}
		}
	})
	t.Run("multiple header lines form one forwarded chain", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = "127.0.0.1:5555"
		r.Header.Add("X-Forwarded-For", "203.0.113.7")
		r.Header.Add("X-Forwarded-For", "198.51.100.9, ::1")
		if got := clientIP(r); got != "198.51.100.9" {
			t.Fatalf("separate header lines changed identity: %q", got)
		}
	})
	t.Run("invalid trusted chain falls back to peer", func(t *testing.T) {
		for _, value := range []string{"203.0.113.7, invalid", "203.0.113.7,", ",", "127.0.0.1, ::1"} {
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r.RemoteAddr = "127.0.0.1:5555"
			r.Header.Set("X-Forwarded-For", value)
			if got := clientIP(r); got != "127.0.0.1" {
				t.Fatalf("invalid or all-trusted chain changed identity: %q", got)
			}
		}
	})

	t.Run("loopback without a header", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = "127.0.0.1:5555"
		if got := clientIP(r); got != "127.0.0.1" {
			t.Fatalf("clientIP = %q", got)
		}
	})

	t.Run("ipv6 loopback is trusted", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		r.RemoteAddr = "[::1]:5555"
		r.Header.Set("X-Forwarded-For", "203.0.113.7")
		if got := clientIP(r); got != "203.0.113.7" {
			t.Fatalf("clientIP = %q", got)
		}
	})

	t.Run("remote address without a port", func(t *testing.T) {
		bare := httptest.NewRequest(http.MethodPost, "/", nil)
		bare.RemoteAddr = "10.0.0.9"
		if got := clientIP(bare); got != "10.0.0.9" {
			t.Fatalf("clientIP without port = %q", got)
		}
	})
}

func TestRequestIsSecure(t *testing.T) {
	if requestIsSecure(nil) {
		t.Error("nil request is not secure")
	}
	if requestIsSecure(httptest.NewRequest(http.MethodGet, "http://h/", nil)) {
		t.Error("plain http is not secure")
	}
	if !requestIsSecure(httptest.NewRequest(http.MethodGet, "https://h/", nil)) {
		t.Error("https is secure")
	}
	r := httptest.NewRequest(http.MethodGet, "http://h/", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	r.Header.Set("X-Forwarded-Proto", "https")
	if !requestIsSecure(r) {
		t.Error("https forwarded over http should count as secure")
	}
	r.RemoteAddr = "203.0.113.7:5555"
	if requestIsSecure(r) {
		t.Error("remote clients must not spoof TLS through a forwarded header")
	}
}

// randomToken encodes `length` random bytes as unpadded base64url, so the
// resulting string is longer than `length`.
func TestTokenDigestAndRandomToken(t *testing.T) {
	token, err := randomToken(24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(token) != 32 {
		t.Fatalf("randomToken(24) produced %d chars, want 32", len(token))
	}
	if strings.ContainsAny(token, "+/=") {
		t.Fatalf("token is not base64url: %q", token)
	}
	other, err := randomToken(24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if other == token {
		t.Fatal("randomToken must not repeat")
	}
	if tokenDigest(token) != tokenDigest(token) {
		t.Fatal("digest must be stable")
	}
	if tokenDigest(token) == tokenDigest(token+"x") {
		t.Fatal("digest must distinguish different tokens")
	}
	if len(tokenDigest(token)) != 64 {
		t.Fatalf("digest should be hex sha256, got %d chars", len(tokenDigest(token)))
	}
}

// --- helpers ---

func keyForIndex(i int) string {
	return "sk-local-" + strings.Repeat(string(rune('a'+i%26)), 45)
}
