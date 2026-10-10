package config

// Characterization tests for package config. Every expectation below records
// what the production code does today, including behaviour that looks
// accidental, so that a refactor that changes it is caught here.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------- rotation --

func TestIsSOTA(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"deepseek-v4.1-flash", true},
		{"glm-5.3", true},
		{"grok-4.6", true},
		{"kimi-k3", true},
		{"qwen3.8-max", true},
		{"GLM-5.3", false},  // exact match, case sensitive
		{" glm-5.3", false}, // no trimming
		{"glm-5.3 ", false},
		{"glm-5.3-dev", false},
		{"glm-4.6", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsSOTA(tt.model); got != tt.want {
			t.Errorf("IsSOTA(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
	if len(SOTAModels) != 5 {
		t.Fatalf("SOTAModels has %d entries, want 5", len(SOTAModels))
	}
}

func TestRotationDefaults(t *testing.T) {
	t.Run("zero value", func(t *testing.T) {
		var c RotationConfig
		c.Defaults()
		if c.SOTA.Alias != "sota" || c.Sweet.Alias != "sweet" {
			t.Errorf("aliases = %q/%q, want sota/sweet", c.SOTA.Alias, c.Sweet.Alias)
		}
		if !reflect.DeepEqual(c.SOTA.Order, SOTAModels) {
			t.Errorf("SOTA.Order = %v, want %v", c.SOTA.Order, SOTAModels)
		}
		if c.FirstOutputSeconds != 30 {
			t.Errorf("FirstOutputSeconds = %d, want 30", c.FirstOutputSeconds)
		}
		// Defaults must clone the package-level slice, not alias it.
		if len(c.SOTA.Order) > 0 {
			c.SOTA.Order[0] = "mutated"
		}
		if SOTAModels[0] != "deepseek-v4.1-flash" {
			t.Fatalf("Defaults aliased SOTAModels: %v", SOTAModels)
		}
	})

	t.Run("keeps existing values", func(t *testing.T) {
		order := []string{"glm-5.3"}
		c := RotationConfig{
			SOTA:               RotationGroup{Alias: "top", Order: order},
			Sweet:              RotationGroup{Alias: "balanced"},
			FirstOutputSeconds: 45,
		}
		c.Defaults()
		if c.SOTA.Alias != "top" || c.Sweet.Alias != "balanced" {
			t.Errorf("aliases = %q/%q, want top/balanced", c.SOTA.Alias, c.Sweet.Alias)
		}
		if !reflect.DeepEqual(c.SOTA.Order, order) {
			t.Errorf("SOTA.Order = %v, want %v", c.SOTA.Order, order)
		}
		if c.FirstOutputSeconds != 45 {
			t.Errorf("FirstOutputSeconds = %d, want 45", c.FirstOutputSeconds)
		}
		// Sweet.Order stays empty: Defaults never fills it.
		if len(c.Sweet.Order) != 0 {
			t.Errorf("Sweet.Order = %v, want empty", c.Sweet.Order)
		}
	})
}

func validRotation() RotationConfig {
	return RotationConfig{
		SOTA:               RotationGroup{Alias: "sota", Order: append([]string(nil), SOTAModels...)},
		Sweet:              RotationGroup{Alias: "sweet", Order: []string{"glm-4.6", "kimi-k2"}},
		FirstOutputSeconds: 30,
	}
}

func TestRotationValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RotationConfig)
		models []string
		want   string // "" means "no error expected"
	}{
		{name: "valid", mutate: func(*RotationConfig) {}},
		{
			name:   "first output seconds lower bound",
			mutate: func(c *RotationConfig) { c.FirstOutputSeconds = 1 },
		},
		{
			name:   "first output seconds upper bound",
			mutate: func(c *RotationConfig) { c.FirstOutputSeconds = 300 },
		},
		{
			name:   "first output seconds zero",
			mutate: func(c *RotationConfig) { c.FirstOutputSeconds = 0 },
			want:   "首个有效输出超时",
		},
		{
			name:   "first output seconds too large",
			mutate: func(c *RotationConfig) { c.FirstOutputSeconds = 301 },
			want:   "首个有效输出超时",
		},
		{
			name:   "identical aliases",
			mutate: func(c *RotationConfig) { c.SOTA.Alias = "same"; c.Sweet.Alias = "same" },
			want:   "两个分组的别名不能相同",
		},
		{
			name:   "alias starting with a dot",
			mutate: func(c *RotationConfig) { c.SOTA.Alias = ".sota" },
			want:   "别名需为 1–64 位",
		},
		{
			name:   "alias starting with a dash",
			mutate: func(c *RotationConfig) { c.SOTA.Alias = "-sota" },
			want:   "别名需为 1–64 位",
		},
		{
			name:   "alias containing a space",
			mutate: func(c *RotationConfig) { c.SOTA.Alias = "so ta" },
			want:   "别名需为 1–64 位",
		},
		{
			name:   "alias 64 characters",
			mutate: func(c *RotationConfig) { c.SOTA.Alias = strings.Repeat("a", 64) },
		},
		{
			name:   "alias 65 characters",
			mutate: func(c *RotationConfig) { c.SOTA.Alias = strings.Repeat("a", 65) },
			want:   "别名需为 1–64 位",
		},
		{
			name:   "alias collides with a real model id",
			mutate: func(c *RotationConfig) { c.SOTA.Alias = "kimi-k2" },
			models: []string{"kimi-k2", "glm-4.6"},
			want:   "别名不能与真实模型 ID 相同",
		},
		{
			name:   "alias collides with a SOTA model id",
			mutate: func(c *RotationConfig) { c.SOTA.Alias = "glm-5.3" },
			want:   "别名不能与真实模型 ID 相同",
		},
		{
			name:   "alias collides with the sweet order",
			mutate: func(c *RotationConfig) { c.SOTA.Alias = "glm-4.6" },
			want:   "别名不能与真实模型 ID 相同",
		},
		{
			name:   "sweet alias inside its own order",
			mutate: func(c *RotationConfig) { c.Sweet.Alias = "glm-4.6" },
			want:   "别名不能与真实模型 ID 相同",
		},
		{
			name:   "sota order with a duplicate",
			mutate: func(c *RotationConfig) { c.SOTA.Order = append(c.SOTA.Order, "glm-5.3") },
			want:   "模型顺序包含重复、空值或不属于该分组的模型",
		},
		{
			name:   "sota order with an empty entry",
			mutate: func(c *RotationConfig) { c.SOTA.Order[2] = "" },
			want:   "模型顺序包含重复、空值或不属于该分组的模型",
		},
		{
			name:   "sota order with a non-SOTA model",
			mutate: func(c *RotationConfig) { c.SOTA.Order[4] = "glm-4.6" },
			want:   "模型顺序包含重复、空值或不属于该分组的模型",
		},
		{
			name:   "sota order missing one model",
			mutate: func(c *RotationConfig) { c.SOTA.Order = c.SOTA.Order[:4] },
			want:   "SOTA 组必须包含指定的五个模型",
		},
		{
			name:   "sweet order containing a SOTA model",
			mutate: func(c *RotationConfig) { c.Sweet.Order = []string{"glm-5.3"} },
			want:   "模型顺序包含重复、空值或不属于该分组的模型",
		},
		{
			name:   "sweet order with a duplicate",
			mutate: func(c *RotationConfig) { c.Sweet.Order = []string{"glm-4.6", "glm-4.6"} },
			want:   "模型顺序包含重复、空值或不属于该分组的模型",
		},
		{
			name:   "empty sweet order is accepted",
			mutate: func(c *RotationConfig) { c.Sweet.Order = nil },
		},
		{
			name:   "empty sota order",
			mutate: func(c *RotationConfig) { c.SOTA.Order = nil },
			want:   "SOTA 组必须包含指定的五个模型",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validRotation()
			tt.mutate(&cfg)
			err := cfg.Validate(tt.models)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() = %q, want it to contain %q", err.Error(), tt.want)
			}
		})
	}
}

// Alias checking only consults the realModels argument: passing nil (which is
// what Normalize does) means a server-reported model id can never collide.
func TestRotationValidateRealModelsAreOnlyCheckedWhenSupplied(t *testing.T) {
	cfg := validRotation()
	cfg.SOTA.Alias = "kimi-k2"
	cfg.Sweet.Order = []string{"glm-4.6"}
	if err := cfg.Validate(nil); err != nil {
		t.Fatalf("Validate(nil) = %v, want nil", err)
	}
	if err := cfg.Validate([]string{"kimi-k2"}); err == nil {
		t.Fatal("Validate with kimi-k2 in realModels = nil, want error")
	}
}

// ------------------------------------------------------------------- proxy --

func TestStripProxyLineComment(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"hash at line start", "# comment", ""},
		{"semicolon at line start", "; comment", ""},
		{"slash at line start", "// comment", ""},
		// The marker and everything after it are dropped, but the whitespace in
		// front of it stays; readProxyFile trims the value afterwards.
		{"hash after whitespace", "socks5://a:1 # note", "socks5://a:1 "},
		{"semicolon after whitespace", "socks5://a:1 ; note", "socks5://a:1 "},
		{"slash slash after whitespace", "socks5://a:1 // note", "socks5://a:1 "},
		{"tab separated comment", "socks5://a:1\t# note", "socks5://a:1\t"},
		{"scheme slashes are kept", "socks5h://user:pass@host:1080", "socks5h://user:pass@host:1080"},
		{"inner slashes are kept", "http://host/a//b", "http://host/a//b"},
		{"marker glued to value is kept", "socks5://a:1;note", "socks5://a:1;note"},
		{"hash glued to value is kept", "socks5://a:1#note", "socks5://a:1#note"},
		{"only a hash", "#", ""},
		{"only a semicolon", ";", ""},
		{"lone slash is kept", "/", "/"},
		{"comment ends at the first marker", "proxy # one # two", "proxy "},
		{"spaces before marker are kept", "proxy # note", "proxy "},
		{"double slash after value and space", "proxy // note", "proxy "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripProxyLineComment(tt.in); got != tt.want {
				t.Errorf("stripProxyLineComment(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestUniqueStrings(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", []string{}, []string{}},
		{"nil", nil, nil},
		{"no duplicates", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"keeps first occurrence order", []string{"b", "a", "b", "c", "a"}, []string{"b", "a", "c"}},
		{"empty strings are values too", []string{"", "a", ""}, []string{"", "a"}},
		{"case sensitive", []string{"a", "A"}, []string{"a", "A"}},
		{"not trimmed", []string{"a", " a"}, []string{"a", " a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uniqueStrings(append([]string(nil), tt.in...))
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("uniqueStrings(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// uniqueStrings reuses the caller's backing array: items[:0] then append.
func TestUniqueStringsWritesIntoTheInputArray(t *testing.T) {
	original := []string{"b", "a", "b", "c"}
	got := uniqueStrings(original)
	if !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Fatalf("uniqueStrings = %v", got)
	}
	if original[0] != "b" || original[1] != "a" || original[2] != "c" {
		t.Fatalf("input slice was not reused in place: %v", original)
	}
}

func TestTrimList(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty", []string{}, nil},
		{"drops blank and whitespace-only entries", []string{"a", "", "  ", "\t", "b"}, []string{"a", "b"}},
		{"trims surrounding whitespace", []string{" a ", "b\t", "  c"}, []string{"a", "b", "c"}},
		{"order preserved", []string{"z", " a ", "y"}, []string{"z", "a", "y"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := append([]string(nil), tt.in...)
			trimList(&items)
			if len(tt.want) == 0 {
				if len(items) != 0 {
					t.Fatalf("trimList(%v) = %v, want empty", tt.in, items)
				}
				return
			}
			if !reflect.DeepEqual(items, tt.want) {
				t.Errorf("trimList(%v) = %v, want %v", tt.in, items, tt.want)
			}
		})
	}
}

// trimList compacts into the same backing array, so the caller's original
// slice observes the trimmed values.
func TestTrimListReusesTheInputArray(t *testing.T) {
	original := []string{" a ", "", " b"}
	items := original
	trimList(&items)
	if !reflect.DeepEqual(items, []string{"a", "b"}) {
		t.Fatalf("trimList = %v, want [a b]", items)
	}
	if original[0] != "a" || original[1] != "b" {
		t.Fatalf("input slice was not reused in place: %q", original)
	}
}

func TestReadProxyFile(t *testing.T) {
	t.Run("comments, blanks and BOM", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "proxies.txt")
		content := strings.Join([]string{
			"\ufeffsocks5://a:1",
			"",
			"   ",
			"# hash comment",
			"; semicolon comment",
			"// slash comment",
			"  socks5://b:2   # trailing note",
			"socks5h://c:3",
			"\t",
		}, "\n")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := readProxyFile(path)
		if err != nil {
			t.Fatalf("readProxyFile: %v", err)
		}
		want := []string{"socks5://a:1", "socks5://b:2", "socks5h://c:3"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("readProxyFile = %v, want %v", got, want)
		}
	})

	t.Run("carriage returns are trimmed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "crlf.txt")
		if err := os.WriteFile(path, []byte("socks5://a:1\r\nsocks5://b:2\r\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := readProxyFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"socks5://a:1", "socks5://b:2"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("readProxyFile = %v, want %v", got, want)
		}
	})

	t.Run("file of comments only yields nothing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.txt")
		if err := os.WriteFile(path, []byte("# only\n; comments\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := readProxyFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Fatalf("readProxyFile = %v, want empty", got)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if _, err := readProxyFile(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
			t.Fatal("readProxyFile(missing) = nil error, want failure")
		}
	})
}

func TestResolveProxyFiles(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	writeProxies := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("no proxies configured falls back to direct", func(t *testing.T) {
		cfg := Config{}
		if err := resolveProxyFiles(configPath, &cfg); err != nil {
			t.Fatal(err)
		}
		if want := []string{"direct"}; !reflect.DeepEqual(cfg.effectiveProxies, want) {
			t.Fatalf("effectiveProxies = %v, want %v", cfg.effectiveProxies, want)
		}
		// The stored list is untouched: "direct" is never written back.
		if len(cfg.Proxies) != 0 {
			t.Fatalf("Proxies = %v, want empty", cfg.Proxies)
		}
	})

	t.Run("inline proxies are trimmed and blank entries dropped", func(t *testing.T) {
		cfg := Config{Proxies: []string{" socks5://a:1 ", "", "direct"}}
		if err := resolveProxyFiles(configPath, &cfg); err != nil {
			t.Fatal(err)
		}
		if want := []string{"socks5://a:1", "direct"}; !reflect.DeepEqual(cfg.effectiveProxies, want) {
			t.Fatalf("effectiveProxies = %v, want %v", cfg.effectiveProxies, want)
		}
		if want := []string{"socks5://a:1", "direct"}; !reflect.DeepEqual(cfg.Proxies, want) {
			t.Fatalf("Proxies = %v, want %v", cfg.Proxies, want)
		}
	})

	t.Run("relative proxyfile is resolved next to the config", func(t *testing.T) {
		writeProxies("list.txt", "socks5://b:2\nsocks5://c:3\n")
		cfg := Config{Proxies: []string{"socks5://a:1"}, ProxyFile: "list.txt"}
		if err := resolveProxyFiles(configPath, &cfg); err != nil {
			t.Fatal(err)
		}
		// Config list first, file entries appended, duplicates removed.
		if want := []string{"socks5://a:1", "socks5://b:2", "socks5://c:3"}; !reflect.DeepEqual(cfg.effectiveProxies, want) {
			t.Fatalf("effectiveProxies = %v, want %v", cfg.effectiveProxies, want)
		}
	})

	t.Run("absolute proxyfile is used as is", func(t *testing.T) {
		abs := writeProxies("abs.txt", "socks5://abs:1\n")
		cfg := Config{ProxyFile: abs}
		if err := resolveProxyFiles(configPath, &cfg); err != nil {
			t.Fatal(err)
		}
		if want := []string{"socks5://abs:1"}; !reflect.DeepEqual(cfg.effectiveProxies, want) {
			t.Fatalf("effectiveProxies = %v, want %v", cfg.effectiveProxies, want)
		}
	})

	t.Run("duplicates across list and file collapse to the first position", func(t *testing.T) {
		writeProxies("dupes.txt", "socks5://b:2\nsocks5://a:1\n")
		cfg := Config{Proxies: []string{"socks5://a:1"}, ProxyFile: "dupes.txt"}
		if err := resolveProxyFiles(configPath, &cfg); err != nil {
			t.Fatal(err)
		}
		if want := []string{"socks5://a:1", "socks5://b:2"}; !reflect.DeepEqual(cfg.effectiveProxies, want) {
			t.Fatalf("effectiveProxies = %v, want %v", cfg.effectiveProxies, want)
		}
	})

	t.Run("missing proxyfile reports the resolved path", func(t *testing.T) {
		cfg := Config{ProxyFile: "nope.txt"}
		err := resolveProxyFiles(configPath, &cfg)
		if err == nil {
			t.Fatal("resolveProxyFiles = nil, want error")
		}
		if !strings.Contains(err.Error(), "load proxy file") {
			t.Fatalf("error = %q, want it to mention 'load proxy file'", err.Error())
		}
		if !strings.Contains(err.Error(), filepath.Join(dir, "nope.txt")) {
			t.Fatalf("error = %q, want it to contain the resolved path", err.Error())
		}
		if !os.IsNotExist(errUnwrapAll(err)) && !strings.Contains(err.Error(), "no such file") {
			t.Logf("error chain: %v", err)
		}
	})
}

// errUnwrapAll is a tiny helper so the missing-file test can inspect the
// wrapped *os.PathError without importing errors just for one call.
func errUnwrapAll(err error) error {
	for {
		next, ok := err.(interface{ Unwrap() error })
		if !ok {
			return err
		}
		unwrapped := next.Unwrap()
		if unwrapped == nil {
			return err
		}
		err = unwrapped
	}
}

// --------------------------------------------------------------------- json --

func TestStripJSONComments(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no comments", "{\n  \"a\": 1\n}", "{\n  \"a\": 1\n}"},
		{"line comment at end of file", "{\"a\":1} // note", "{\"a\":1}        "},
		{"line comment keeps the newline", "{\"a\":1}// note\n{\"b\":2}", "{\"a\":1}       \n{\"b\":2}"},
		{"whole-line comments", "// a\n{\"a\":1}\n// b\n", "    \n{\"a\":1}\n    \n"},
		{"block comment on one line", "{/* c */\"a\":1}", "{       \"a\":1}"},
		{"block comment keeps newlines", "{/*\n*/\"a\":1}", "{  \n  \"a\":1}"},
		{"unterminated line comment is fine", "{\"a\":1}//", "{\"a\":1}  "},
		{"url inside a string is preserved", "{\"u\":\"https://a/b//c\"}", "{\"u\":\"https://a/b//c\"}"},
		{"block markers inside a string are preserved", "{\"u\":\"/*x*/\"}", "{\"u\":\"/*x*/\"}"},
		{"escaped quote does not end the string", "{\"a\":\"he said \\\"//\\\"\"}", "{\"a\":\"he said \\\"//\\\"\"}"},
		{"escaped backslash before a quote", `{"a":"back\\"} // x`, `{"a":"back\\"}     `},
		{"BOM is stripped", "\ufeff{\"a\":1}", "{\"a\":1}"},
		{"comment markers inside a comment", "// a // b\n{}", "         \n{}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stripJSONComments([]byte(tt.in))
			if err != nil {
				t.Fatalf("stripJSONComments(%q) error = %v", tt.in, err)
			}
			if string(got) != tt.want {
				t.Fatalf("stripJSONComments(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStripJSONCommentsUnterminatedBlockComment(t *testing.T) {
	out, err := stripJSONComments([]byte("{\"a\":1} /* never closed"))
	if err == nil {
		t.Fatalf("stripJSONComments = %q, want an error", out)
	}
	if err.Error() != "unterminated block comment" {
		t.Fatalf("error = %q, want %q", err.Error(), "unterminated block comment")
	}
	if out != nil {
		t.Fatalf("output = %q, want nil on error", out)
	}
}

func TestStripJSONCommentsKeepsLineNumbers(t *testing.T) {
	in := "{\n  // a comment\n  \"a\": 1\n}"
	out, err := stripJSONComments([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(out), "\n") != strings.Count(in, "\n") {
		t.Fatalf("newlines not preserved: %q", out)
	}
}

// ------------------------------------------------------------------ secrets --

func TestFingerprint(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		// First 10 hex characters of sha256(value).
		{"sk-abc123", "0a2f6ba7b3"},
		{"sk-def456", "f301ac7118"},
		{"", "e3b0c44298"},
		{"glm-5.3", "9691a990ed"},
	}
	for _, tt := range tests {
		if got := Fingerprint(tt.value); got != tt.want {
			t.Errorf("Fingerprint(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
	if got := Fingerprint("sk-abc123"); got != Fingerprint("sk-abc123") {
		t.Errorf("Fingerprint is not stable: %q vs %q", got, Fingerprint("sk-abc123"))
	}
	if Fingerprint("sk-abc123") == Fingerprint("sk-abc124") {
		t.Error("Fingerprint collides for near-identical keys")
	}
	if len(Fingerprint("x")) != 10 {
		t.Errorf("Fingerprint length = %d, want 10", len(Fingerprint("x")))
	}
}

func TestMaskValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"empty", "", "••••"},
		{"short", "abc", "••••"},
		{"exactly five runes reveals nothing", "abcde", "••••"},
		{"six runes", "abcdef", "••••bcdef"},
		{"long secret", "sk-abcdefghij", "••••fghij"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskValue(tt.value); got != tt.want {
				t.Errorf("MaskValue(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}

	t.Run("never leaks the middle of a secret", func(t *testing.T) {
		const secret = "sk-live-2f9C4d-MIDDLE-PART-7788"
		masked := MaskValue(secret)
		if strings.Contains(masked, "MIDDLE") {
			t.Fatalf("MaskValue leaked the middle: %q", masked)
		}
		if strings.Contains(masked, "sk-live") {
			t.Fatalf("MaskValue leaked the prefix: %q", masked)
		}
		if want := "••••-7788"; masked != want {
			t.Fatalf("MaskValue = %q, want %q", masked, want)
		}
	})

	t.Run("counts runes, not bytes", func(t *testing.T) {
		// 8 runes, 24 bytes: a byte-based implementation would keep only one
		// character of the tail.
		if got := MaskValue("密码密码密码密码"); got != "••••码密码密码" {
			t.Errorf("MaskValue(密码密码密码密码) = %q, want %q", got, "••••码密码密码")
		}
	})
}

func TestKeyDisplayID(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{"", ""},
		{"abc", "abc"},
		{"abcde", "abcde"},
		{"abcdef", "bcdef"},
		{"sk-abcdefghij", "fghij"},
	}
	for _, tt := range tests {
		if got := KeyDisplayID(tt.value); got != tt.want {
			t.Errorf("KeyDisplayID(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// The mask must render literally. Going through url.User would escape
		// the asterisks and surface %2A%2A%2A in the UI instead.
		{"user and password", "https://user:pass@zen.example:8443/v1", "https://***:***@zen.example:8443/v1"},
		{"user only", "https://user@zen.example/v1", "https://***@zen.example/v1"},
		{"no credentials", "https://zen.example/v1", "https://zen.example/v1"},
		{"empty", "", ""},
		{"unparsable", "://nope", "<invalid>"},
		{"missing host with credentials", "socks5://user:pass@", "<invalid>"},
		{"relative authority", "//user:pass@proxy.example:1080/path", "//***:***@proxy.example:1080/path"},
		{"ipv6 and suffix", "socks5://user:pass@[::1]:1080/path?q=1#part", "socks5://***:***@[::1]:1080/path?q=1#part"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactURL(tt.in); got != tt.want {
				t.Errorf("RedactURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSecretRedactorWithoutReplaceIsInert(t *testing.T) {
	r := NewSecretRedactor()
	if got := r.String("sk-abc123 and sk-def456"); got != "sk-abc123 and sk-def456" {
		t.Fatalf("String on a fresh redactor = %q, want the input unchanged", got)
	}
}

func TestSecretRedactorReplacesConfiguredSecrets(t *testing.T) {
	cfg := Config{
		ServerKeys: []string{"sk-server-1"},
		ZenKeys:    []string{"zen-key-1"},
		GoKeys:     []string{"go-key-1"},
		WebUI:      WebUIConfig{Password: "webui-password-1"},
		Upstream:   UpstreamConfig{Zen: "https://zenuser:zenpass@zen.example", Go: "https://gouser@gopass@go.example"},
	}
	cfg.effectiveProxies = []string{"direct", "socks5://puser:ppass@proxy.example:1080"}

	r := NewSecretRedactor()
	r.Replace(cfg)

	tests := []struct {
		in   string
		want string
	}{
		{"server key sk-server-1 used", "server key *** used"},
		{"zen key zen-key-1 used", "zen key *** used"},
		{"go key go-key-1 used", "go key *** used"},
		{"password webui-password-1", "password ***"},
		// The whole credentialed URL is registered as a secret and is the
		// longest match, so the entire URL collapses to "***" rather than
		// only its userinfo part.
		{"proxy via socks5://puser:ppass@proxy.example:1080", "proxy via ***"},
		{"user puser and ppass", "user *** and ***"},
		{"upstream https://zenuser:zenpass@zen.example", "upstream ***"},
		{"upstream go https://gouser@gopass@go.example", "upstream go ***"},
		{"direct stays visible", "direct stays visible"},
		{"unrelated text sk-server", "unrelated text sk-server"},
	}
	for _, tt := range tests {
		if got := r.String(tt.in); got != tt.want {
			t.Errorf("String(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// Secrets shorter than 4 characters are skipped, so a 1-3 character value is
// never rewritten.
func TestSecretRedactorSkipsVeryShortValues(t *testing.T) {
	cfg := Config{ZenKeys: []string{"ab", "abc", "abcd"}, Anonymous: true}
	r := NewSecretRedactor()
	r.Replace(cfg)
	if got := r.String("ab abc abcd"); got != "ab abc ***" {
		t.Fatalf("String = %q, want %q", got, "ab abc ***")
	}
}

// Longer secrets are replaced before shorter ones that are a prefix of them,
// so a short key cannot leave its own tail exposed.
func TestSecretRedactorReplacesLongestFirst(t *testing.T) {
	cfg := Config{ServerKeys: []string{"abcd"}, ZenKeys: []string{"abcdefgh"}}
	r := NewSecretRedactor()
	r.Replace(cfg)
	if got := r.String("abcdefgh"); got != "***" {
		t.Fatalf("String(abcdefgh) = %q, want %q", got, "***")
	}
}

func TestSecretRedactorHandlesUpstreamWithoutCredentials(t *testing.T) {
	cfg := Config{Anonymous: true, Upstream: UpstreamConfig{Zen: "https://zen.example", Go: "https://go.example"}}
	r := NewSecretRedactor()
	r.Replace(cfg)
	if got := r.String("https://zen.example"); got != "https://zen.example" {
		t.Fatalf("String = %q, want it unchanged", got)
	}
}

// -------------------------------------------------------------- server keys --

func TestServerKeyEnabled(t *testing.T) {
	const key = "sk-abc123"
	cfg := Config{
		ServerKeys: []string{key},
		ServerKeyMetadata: map[string]ServerKeyMetadata{
			Fingerprint(key): {Name: "primary", Disabled: true},
			Fingerprint("x"): {Name: "other"},
		},
	}
	if cfg.ServerKeyEnabled(key) {
		t.Error("ServerKeyEnabled = true for a key disabled in metadata")
	}
	if !cfg.ServerKeyEnabled("sk-unknown") {
		t.Error("ServerKeyEnabled = false for a key with no metadata entry")
	}
	if cfg.ServerKeyMetadata[Fingerprint(key)].Name != "primary" {
		t.Error("metadata is not keyed by Fingerprint")
	}
}

func TestFirstEnabledServerKey(t *testing.T) {
	const (
		first  = "sk-first"
		second = "sk-second"
	)
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "no keys",
			cfg:  Config{},
			want: "",
		},
		{
			name: "no metadata means every key is enabled",
			cfg:  Config{ServerKeys: []string{first, second}},
			want: first,
		},
		{
			name: "nil metadata map is treated as enabled",
			cfg:  Config{ServerKeys: []string{first}, ServerKeyMetadata: nil},
			want: first,
		},
		{
			name: "skips disabled keys in order",
			cfg: Config{
				ServerKeys: []string{first, second},
				ServerKeyMetadata: map[string]ServerKeyMetadata{
					Fingerprint(first): {Disabled: true},
				},
			},
			want: second,
		},
		{
			name: "returns empty when everything is disabled",
			cfg: Config{
				ServerKeys: []string{first, second},
				ServerKeyMetadata: map[string]ServerKeyMetadata{
					Fingerprint(first):  {Disabled: true},
					Fingerprint(second): {Disabled: true},
				},
			},
			want: "",
		},
		{
			name: "disabled metadata for another key does not matter",
			cfg: Config{
				ServerKeys: []string{first},
				ServerKeyMetadata: map[string]ServerKeyMetadata{
					Fingerprint(second): {Disabled: true},
				},
			},
			want: first,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.FirstEnabledServerKey(); got != tt.want {
				t.Errorf("FirstEnabledServerKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --------------------------------------------------------------------- tier --

func TestTierConstants(t *testing.T) {
	if TierZen != "zen" || TierGo != "go" {
		t.Fatalf("tier constants = %q/%q, want zen/go", TierZen, TierGo)
	}
	// Tier is a defined string type: it can be built from an untyped literal
	// and compared against raw strings.
	if string(TierZen) != "zen" {
		t.Fatalf("string(TierZen) = %q, want zen", string(TierZen))
	}
	var fromLiteral Tier = "custom"
	if fromLiteral == TierZen || fromLiteral == TierGo {
		t.Fatal("an unknown tier must not equal a known one")
	}
}

// ------------------------------------------------------------------ config --

func TestValidateEffort(t *testing.T) {
	valid := []string{"", "auto", "minimal", "low", "medium", "high", "xhigh", "max", "none"}
	for _, value := range valid {
		if err := validateEffort("reasoning.effort", value); err != nil {
			t.Errorf("validateEffort(%q) = %v, want nil", value, err)
		}
	}
	invalid := []string{"LOW", "High", " ultra", "auto ", "off", "0", "min", "maximum", "none ", "none\t"}
	for _, value := range invalid {
		err := validateEffort("reasoning.effort", value)
		if err == nil {
			t.Errorf("validateEffort(%q) = nil, want an error", value)
			continue
		}
		if !strings.HasPrefix(err.Error(), "reasoning.effort must be one of") {
			t.Errorf("validateEffort(%q) error = %q, want it to name the field", value, err.Error())
		}
	}
}

func TestNormalizeReasoning(t *testing.T) {
	tests := []struct {
		name    string
		input   ReasoningConfig
		want    ReasoningConfig
		wantErr string
	}{
		{
			name:  "empty stays empty",
			input: ReasoningConfig{},
			want:  ReasoningConfig{},
		},
		{
			name:  "effort is lowercased and trimmed",
			input: ReasoningConfig{Effort: "  HIGH  "},
			want:  ReasoningConfig{Effort: "high"},
		},
		{
			name:    "unknown effort is rejected",
			input:   ReasoningConfig{Effort: "Ultra"},
			wantErr: "reasoning.effort must be one of",
		},
		{
			name:  "nil map stays nil",
			input: ReasoningConfig{Effort: "low"},
			want:  ReasoningConfig{Effort: "low"},
		},
		{
			name:  "empty map is folded to nil",
			input: ReasoningConfig{EffortByModel: map[string]string{}},
			want:  ReasoningConfig{},
		},
		{
			name:  "per-model entries are trimmed and lowercased",
			input: ReasoningConfig{EffortByModel: map[string]string{" glm-5.3 ": "  XHIGH "}},
			want:  ReasoningConfig{EffortByModel: map[string]string{"glm-5.3": "xhigh"}},
		},
		{
			name:    "empty model id is rejected",
			input:   ReasoningConfig{EffortByModel: map[string]string{"   ": "low"}},
			wantErr: "reasoning.effort_by_model must not contain an empty model ID",
		},
		{
			name:    "invalid per-model effort names the model",
			input:   ReasoningConfig{EffortByModel: map[string]string{"glm-5.3": "maxish"}},
			wantErr: `reasoning.effort_by_model["glm-5.3"] must be one of`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Reasoning: tt.input}
			err := normalizeReasoning(&cfg)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("normalizeReasoning = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeReasoning: %v", err)
			}
			if !reflect.DeepEqual(cfg.Reasoning, tt.want) {
				t.Fatalf("Reasoning = %+v, want %+v", cfg.Reasoning, tt.want)
			}
		})
	}
}

func TestForcedEffort(t *testing.T) {
	cfg := Config{Reasoning: ReasoningConfig{
		Effort:        "medium",
		EffortByModel: map[string]string{"glm-5.3": "high"},
	}}
	if got := cfg.ForcedEffort("glm-5.3"); got != "high" {
		t.Errorf("ForcedEffort(overridden) = %q, want high", got)
	}
	if got := cfg.ForcedEffort("other-model"); got != "medium" {
		t.Errorf("ForcedEffort(default) = %q, want medium", got)
	}
	if got := (Config{}).ForcedEffort("m"); got != "" {
		t.Errorf("ForcedEffort(zero config) = %q, want empty", got)
	}
}

func TestRuntimeProxies(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want []string
	}{
		{
			name: "nothing configured",
			cfg:  Config{},
			want: []string{"direct"},
		},
		{
			name: "stored proxies are returned when nothing is resolved",
			cfg:  Config{Proxies: []string{"socks5://a:1"}},
			want: []string{"socks5://a:1"},
		},
		{
			name: "resolved proxies win",
			cfg:  Config{Proxies: []string{"socks5://a:1"}, effectiveProxies: []string{"direct", "socks5://b:2"}},
			want: []string{"direct", "socks5://b:2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.RuntimeProxies(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RuntimeProxies() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPasswordForSaveClearsOnlyResolvedProxies(t *testing.T) {
	cfg := Config{Proxies: []string{"socks5://a:1"}, effectiveProxies: []string{"direct", "socks5://a:1"}}
	cfg.PasswordForSave()
	if cfg.effectiveProxies != nil {
		t.Errorf("effectiveProxies = %v, want nil before save", cfg.effectiveProxies)
	}
	if want := []string{"socks5://a:1"}; !reflect.DeepEqual(cfg.Proxies, want) {
		t.Errorf("Proxies = %v, want %v", cfg.Proxies, want)
	}
}

func TestAttemptTimeout(t *testing.T) {
	tests := []struct {
		name    string
		seconds int
		request time.Duration
		want    time.Duration
	}{
		{"zero keeps the request timeout", 0, 30 * time.Second, 30 * time.Second},
		{"negative keeps the request timeout", -5, 30 * time.Second, 30 * time.Second},
		{"configured below the request timeout", 5, 30 * time.Second, 5 * time.Second},
		{"configured above the request timeout is capped", 60, 30 * time.Second, 30 * time.Second},
		{"configured with no request timeout", 60, 0, 60 * time.Second},
		{"both zero", 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := PerformanceConfig{AttemptTimeoutSeconds: tt.seconds}
			if got := cfg.AttemptTimeout(tt.request); got != tt.want {
				t.Errorf("AttemptTimeout(%v) with %ds = %v, want %v", tt.request, tt.seconds, got, tt.want)
			}
		})
	}
}

func TestCloneIsDeep(t *testing.T) {
	cfg := Config{
		Rotation:          RotationConfig{SOTA: RotationGroup{Order: []string{"glm-5.3"}}, Sweet: RotationGroup{Order: []string{"a"}}},
		ServerKeys:        []string{"sk-1"},
		ServerKeyMetadata: map[string]ServerKeyMetadata{Fingerprint("sk-1"): {Name: "one"}},
		ZenKeys:           []string{"z"},
		GoKeys:            []string{"g"},
		Proxies:           []string{"direct"},
		effectiveProxies:  []string{"direct"},
		Models:            ModelsConfig{Protocols: map[string]string{"m": "chat"}},
		Reasoning:         ReasoningConfig{EffortByModel: map[string]string{"m": "low"}},
	}
	clone := Clone(cfg)
	clone.ServerKeys[0] = "sk-2"
	clone.ServerKeyMetadata[Fingerprint("sk-1")] = ServerKeyMetadata{Name: "changed"}
	clone.Models.Protocols["m"] = "anthropic"
	clone.Reasoning.EffortByModel["m"] = "high"
	clone.Rotation.SOTA.Order[0] = "changed"
	clone.Rotation.Sweet.Order[0] = "changed"
	clone.effectiveProxies[0] = "changed"

	if cfg.ServerKeys[0] != "sk-1" {
		t.Error("ServerKeys is shared with the clone")
	}
	if cfg.ServerKeyMetadata[Fingerprint("sk-1")].Name != "one" {
		t.Error("ServerKeyMetadata is shared with the clone")
	}
	if cfg.Models.Protocols["m"] != "chat" {
		t.Error("Models.Protocols is shared with the clone")
	}
	if cfg.Reasoning.EffortByModel["m"] != "low" {
		t.Error("Reasoning.EffortByModel is shared with the clone")
	}
	if cfg.Rotation.SOTA.Order[0] != "glm-5.3" || cfg.Rotation.Sweet.Order[0] != "a" {
		t.Error("Rotation orders are shared with the clone")
	}
	if cfg.effectiveProxies[0] != "direct" {
		t.Error("effectiveProxies is shared with the clone")
	}
}

func TestCloneKeepsNilCollections(t *testing.T) {
	clone := Clone(Config{})
	if clone.ServerKeyMetadata == nil || len(clone.ServerKeyMetadata) != 0 {
		t.Errorf("ServerKeyMetadata = %v, want an empty non-nil map", clone.ServerKeyMetadata)
	}
	if clone.Models.Protocols != nil {
		t.Errorf("Models.Protocols = %v, want nil", clone.Models.Protocols)
	}
	if clone.Reasoning.EffortByModel != nil {
		t.Errorf("Reasoning.EffortByModel = %v, want nil", clone.Reasoning.EffortByModel)
	}
	if clone.ServerKeys != nil {
		t.Errorf("ServerKeys = %v, want nil", clone.ServerKeys)
	}
}

// baseConfig returns a configuration that Normalize accepts unchanged.
func baseConfig() Config {
	return Config{
		Listen:     "127.0.0.1:8080",
		ServerKeys: []string{"sk-local-1"},
		ZenKeys:    []string{"zen-1"},
		Prefer:     TierGo,
		Upstream: UpstreamConfig{
			Zen: "https://zen.example",
			Go:  "https://go.example",
		},
		Retry: RetryConfig{MaxAttempts: 3, TimeoutSeconds: 300},
		Models: ModelsConfig{
			RefreshSeconds: 300,
			Protocols:      map[string]string{},
		},
		Performance: PerformanceConfig{
			MaxIdleConns:           2048,
			MaxIdleConnsPerHost:    256,
			MaxConnsPerHost:        0,
			IdleConnTimeoutSeconds: 120,
			ConnectTimeoutSeconds:  5,
			FailureCooldownSeconds: 15,
		},
		Logging: LoggingConfig{Level: "info", RingSize: 2000},
	}
}

func TestNormalizeHappyPath(t *testing.T) {
	cfg := baseConfig()
	got, err := Normalize("config.json", cfg)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	// Defaults are applied for the rotation block.
	if got.Rotation.SOTA.Alias != "sota" || got.Rotation.Sweet.Alias != "sweet" {
		t.Errorf("rotation aliases = %q/%q", got.Rotation.SOTA.Alias, got.Rotation.Sweet.Alias)
	}
	if got.Rotation.FirstOutputSeconds != 30 {
		t.Errorf("FirstOutputSeconds = %d, want 30", got.Rotation.FirstOutputSeconds)
	}
	if !reflect.DeepEqual(got.Rotation.SOTA.Order, SOTAModels) {
		t.Errorf("SOTA.Order = %v", got.Rotation.SOTA.Order)
	}
	if want := []string{"direct"}; !reflect.DeepEqual(got.effectiveProxies, want) {
		t.Errorf("effectiveProxies = %v, want %v", got.effectiveProxies, want)
	}
	if got.Listen != "127.0.0.1:8080" || got.Prefer != TierGo {
		t.Errorf("unexpected normalised values: %+v", got)
	}
}

func TestNormalizeTrimsKeyLists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "proxies.txt"), []byte("socks5://file:1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := baseConfig()
	cfg.ServerKeys = []string{"  sk-local-1  ", ""}
	cfg.ZenKeys = []string{" zen-1 "}
	cfg.GoKeys = []string{"  "}
	cfg.Upstream.Zen = "  https://zen.example  "
	cfg.ProxyFile = "  proxies.txt  "
	got, err := Normalize(filepath.Join(dir, "config.json"), cfg)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if want := []string{"sk-local-1"}; !reflect.DeepEqual(got.ServerKeys, want) {
		t.Errorf("ServerKeys = %v, want %v", got.ServerKeys, want)
	}
	if want := []string{"zen-1"}; !reflect.DeepEqual(got.ZenKeys, want) {
		t.Errorf("ZenKeys = %v, want %v", got.ZenKeys, want)
	}
	if len(got.GoKeys) != 0 {
		t.Errorf("GoKeys = %v, want empty", got.GoKeys)
	}
	if got.Upstream.Zen != "https://zen.example" {
		t.Errorf("Upstream.Zen = %q", got.Upstream.Zen)
	}
	if got.ProxyFile != "proxies.txt" {
		t.Errorf("ProxyFile = %q", got.ProxyFile)
	}
	// "  proxies.txt  " was trimmed before it was resolved relative to the
	// config file, so the file entry made it into the effective list. Because
	// that list is non-empty, no "direct" fallback is added.
	if want := []string{"socks5://file:1"}; !reflect.DeepEqual(got.effectiveProxies, want) {
		t.Errorf("effectiveProxies = %v, want %v", got.effectiveProxies, want)
	}
}

func TestNormalizeRejections(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{
			name:   "prefer",
			mutate: func(c *Config) { c.Prefer = "both" },
			want:   `prefer must be "zen" or "go"`,
		},
		{
			name:   "empty listen",
			mutate: func(c *Config) { c.Listen = "" },
			want:   "listen must not be empty",
		},
		{
			name:   "upstream zen without scheme",
			mutate: func(c *Config) { c.Upstream.Zen = "zen.example" },
			want:   "upstream.zen must be an http or https URL",
		},
		{
			name:   "upstream go with ftp scheme",
			mutate: func(c *Config) { c.Upstream.Go = "ftp://go.example" },
			want:   "upstream.go must be an http or https URL",
		},
		{
			name:   "upstream zen that is only whitespace",
			mutate: func(c *Config) { c.Upstream.Zen = "   " },
			want:   "upstream.zen must be an http or https URL",
		},
		{
			name:   "no server keys",
			mutate: func(c *Config) { c.ServerKeys = nil },
			want:   "server_keys must contain at least one local key",
		},
		{
			name: "every server key disabled",
			mutate: func(c *Config) {
				c.ServerKeyMetadata = map[string]ServerKeyMetadata{
					Fingerprint("sk-local-1"): {Disabled: true},
				}
			},
			want: "至少保留一个启用的 API Key",
		},
		{
			name:   "no upstream keys while not anonymous",
			mutate: func(c *Config) { c.ZenKeys = nil },
			want:   "zen_keys or go_keys must contain at least one upstream key unless anonymous is enabled",
		},
		{
			name:   "anonymous needs no upstream keys",
			mutate: func(c *Config) { c.Anonymous = true; c.ZenKeys = nil },
			want:   "",
		},
		{
			name:   "retry attempts",
			mutate: func(c *Config) { c.Retry.MaxAttempts = 0 },
			want:   "retry.max_attempts must be at least 1",
		},
		{
			name:   "retry timeout",
			mutate: func(c *Config) { c.Retry.TimeoutSeconds = 0 },
			want:   "retry.timeout_seconds must be at least 1",
		},
		{
			name:   "model refresh",
			mutate: func(c *Config) { c.Models.RefreshSeconds = 0 },
			want:   "models.refresh_seconds must be at least 1",
		},
		{
			name:   "performance idle conns",
			mutate: func(c *Config) { c.Performance.MaxIdleConns = 0 },
			want:   "performance values must be positive",
		},
		{
			name:   "performance idle conns per host",
			mutate: func(c *Config) { c.Performance.MaxIdleConnsPerHost = 0 },
			want:   "performance values must be positive",
		},
		{
			name:   "performance negative max conns per host",
			mutate: func(c *Config) { c.Performance.MaxConnsPerHost = -1 },
			want:   "performance values must be positive",
		},
		{
			name:   "performance zero max conns per host is allowed",
			mutate: func(c *Config) { c.Performance.MaxConnsPerHost = 0 },
			want:   "",
		},
		{
			name:   "performance idle timeout",
			mutate: func(c *Config) { c.Performance.IdleConnTimeoutSeconds = 0 },
			want:   "performance values must be positive",
		},
		{
			name:   "performance connect timeout",
			mutate: func(c *Config) { c.Performance.ConnectTimeoutSeconds = 0 },
			want:   "performance values must be positive",
		},
		{
			name:   "performance failure cooldown",
			mutate: func(c *Config) { c.Performance.FailureCooldownSeconds = 0 },
			want:   "performance values must be positive",
		},
		{
			name:   "negative attempt timeout",
			mutate: func(c *Config) { c.Performance.AttemptTimeoutSeconds = -1 },
			want:   "performance.attempt_timeout_seconds must not be negative",
		},
		{
			name:   "negative first event timeout",
			mutate: func(c *Config) { c.Performance.FirstEventTimeoutSeconds = -1 },
			want:   "performance.first_event_timeout_seconds must be between 0 and 86400",
		},
		{
			name:   "first event timeout above a day",
			mutate: func(c *Config) { c.Performance.FirstEventTimeoutSeconds = 86401 },
			want:   "performance.first_event_timeout_seconds must be between 0 and 86400",
		},
		{
			name:   "logging level",
			mutate: func(c *Config) { c.Logging.Level = "verbose" },
			want:   "logging.level must be debug, info, warn, or error",
		},
		{
			name:   "ring size too small",
			mutate: func(c *Config) { c.Logging.RingSize = 99 },
			want:   "logging.ring_size must be between 100 and 50000",
		},
		{
			name:   "ring size too large",
			mutate: func(c *Config) { c.Logging.RingSize = 50001 },
			want:   "logging.ring_size must be between 100 and 50000",
		},
		{
			// The password length is checked even when the WebUI is disabled.
			name:   "short webui password is rejected while disabled",
			mutate: func(c *Config) { c.WebUI.Password = "short" },
			want:   "webui.password must contain at least 10 characters",
		},
		{
			name: "enabled webui without listen",
			mutate: func(c *Config) {
				c.WebUI = WebUIConfig{Enabled: true, Username: "admin", Password: "longenoughpw", SessionTTLMinutes: 720}
			},
			want: "webui.listen must not be empty when webui is enabled",
		},
		{
			name: "enabled webui without username",
			mutate: func(c *Config) {
				c.WebUI = WebUIConfig{Enabled: true, Listen: "0.0.0.0:8081", Password: "longenoughpw", SessionTTLMinutes: 720}
			},
			want: "webui.username must not be empty when webui is enabled",
		},
		{
			name: "enabled webui without any credential",
			mutate: func(c *Config) {
				c.WebUI = WebUIConfig{Enabled: true, Listen: "0.0.0.0:8081", Username: "admin", SessionTTLMinutes: 720}
			},
			want: "webui.password is required for first-time setup",
		},
		{
			name: "enabled webui accepts a stored hash instead of a password",
			mutate: func(c *Config) {
				c.WebUI = WebUIConfig{Enabled: true, Listen: "0.0.0.0:8081", Username: "admin", PasswordHash: "$argon2id$hash", SessionTTLMinutes: 720}
			},
			want: "",
		},
		{
			name: "webui ttl too small",
			mutate: func(c *Config) {
				c.WebUI = WebUIConfig{Enabled: true, Listen: "0.0.0.0:8081", Username: "admin", Password: "longenoughpw", SessionTTLMinutes: 4}
			},
			want: "webui.session_ttl_minutes must be between 5 and 10080",
		},
		{
			name: "webui ttl too large",
			mutate: func(c *Config) {
				c.WebUI = WebUIConfig{Enabled: true, Listen: "0.0.0.0:8081", Username: "admin", Password: "longenoughpw", SessionTTLMinutes: 10081}
			},
			want: "webui.session_ttl_minutes must be between 5 and 10080",
		},
		{
			name:   "proxy with unsupported scheme",
			mutate: func(c *Config) { c.Proxies = []string{"ftp://proxy.example:21"} },
			want:   `unsupported proxy scheme "ftp"`,
		},
		{
			name:   "proxy without a host",
			mutate: func(c *Config) { c.Proxies = []string{"socks5://"} },
			want:   "invalid proxy URL",
		},
		{
			name:   "direct is exempt from proxy URL validation",
			mutate: func(c *Config) { c.Proxies = []string{"direct"} },
			want:   "",
		},
		{
			name: "all supported proxy schemes are accepted",
			mutate: func(c *Config) {
				c.Proxies = []string{"http://p.example:8080", "https://p.example:8443", "socks5://p.example:1080", "socks5h://p.example:1080"}
			},
			want: "",
		},
		{
			name:   "invalid proxy url is reported redacted",
			mutate: func(c *Config) { c.Proxies = []string{"socks5://user:pass@"} },
			want:   "invalid proxy URL \"<invalid>\"",
		},
		{
			name:   "protocol mapping with an unknown protocol",
			mutate: func(c *Config) { c.Models.Protocols = map[string]string{"m1": "grpc"} },
			want:   `models.protocols contains invalid mapping "m1": "grpc"`,
		},
		{
			name:   "protocol mapping with an empty model",
			mutate: func(c *Config) { c.Models.Protocols = map[string]string{"": "chat"} },
			want:   `models.protocols contains invalid mapping ""`,
		},
		{
			name: "protocol mappings are accepted",
			mutate: func(c *Config) {
				c.Models.Protocols = map[string]string{"m1": "chat", "m2": "responses", "m3": "anthropic", "m4": "systemone"}
			},
			want: "",
		},
		{
			name:   "rotation first output seconds",
			mutate: func(c *Config) { c.Rotation.FirstOutputSeconds = 400 },
			want:   "首个有效输出超时",
		},
		{
			name: "rotation duplicate aliases",
			mutate: func(c *Config) {
				c.Rotation = RotationConfig{SOTA: RotationGroup{Alias: "x", Order: SOTAModels}, Sweet: RotationGroup{Alias: "x"}}
			},
			want: "两个分组的别名不能相同",
		},
		{
			name: "reasoning effort",
			mutate: func(c *Config) {
				c.Reasoning = ReasoningConfig{Effort: "Turbo"}
			},
			want: "reasoning.effort must be one of",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.mutate(&cfg)
			_, err := Normalize("config.json", cfg)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Normalize = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Normalize = nil, want error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Normalize error = %q, want it to contain %q", err.Error(), tt.want)
			}
		})
	}
}

// The rotation block is validated before every other rule, so a bad alias
// masks an otherwise invalid listen address.
func TestNormalizeValidatesRotationFirst(t *testing.T) {
	cfg := baseConfig()
	cfg.Listen = ""
	cfg.Rotation = RotationConfig{SOTA: RotationGroup{Alias: "x", Order: SOTAModels}, Sweet: RotationGroup{Alias: "x"}}
	_, err := Normalize("config.json", cfg)
	if err == nil {
		t.Fatal("Normalize = nil, want an error")
	}
	if !strings.Contains(err.Error(), "两个分组的别名不能相同") {
		t.Fatalf("error = %q, want the rotation error to win", err.Error())
	}
}

func TestNormalizeMissingProxyFile(t *testing.T) {
	dir := t.TempDir()
	cfg := baseConfig()
	cfg.ProxyFile = "missing.txt"
	_, err := Normalize(filepath.Join(dir, "config.json"), cfg)
	if err == nil {
		t.Fatal("Normalize = nil, want an error")
	}
	if !strings.Contains(err.Error(), "load proxy file") {
		t.Fatalf("error = %q, want a proxy file error", err.Error())
	}
}

// --------------------------------------------------------------------- Load --

func TestLoadAppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{
  // only the fields an operator must set
  "server_keys": ["sk-local-1"],
  "zen_keys": ["zen-1"]
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != "127.0.0.1:8080" {
		t.Errorf("Listen = %q, want 127.0.0.1:8080", cfg.Listen)
	}
	if cfg.Upstream.Zen != "https://opencode.ai/zen" || cfg.Upstream.Go != "https://opencode.ai/zen/go" {
		t.Errorf("Upstream = %+v", cfg.Upstream)
	}
	if cfg.Retry.MaxAttempts != 3 || cfg.Retry.TimeoutSeconds != 300 {
		t.Errorf("Retry = %+v", cfg.Retry)
	}
	if cfg.Models.RefreshSeconds != 300 {
		t.Errorf("Models.RefreshSeconds = %d, want 300", cfg.Models.RefreshSeconds)
	}
	if cfg.Logging.Level != "info" || cfg.Logging.RingSize != 2000 {
		t.Errorf("Logging = %+v", cfg.Logging)
	}
	if cfg.WebUI.Listen != "0.0.0.0:8081" || cfg.WebUI.SessionTTLMinutes != 720 {
		t.Errorf("WebUI = %+v", cfg.WebUI)
	}
	if cfg.Prefer != TierGo {
		t.Errorf("Prefer = %q, want go", cfg.Prefer)
	}
	if cfg.Performance.MaxIdleConns != 2048 || cfg.Performance.MaxConnsPerHost != 0 {
		t.Errorf("Performance = %+v", cfg.Performance)
	}
	// Defaults are still subject to Normalize, so proxies resolve and the
	// rotation block is filled in.
	if want := []string{"direct"}; !reflect.DeepEqual(cfg.RuntimeProxies(), want) {
		t.Errorf("RuntimeProxies() = %v, want %v", cfg.RuntimeProxies(), want)
	}
	if cfg.Rotation.SOTA.Alias != "sota" || cfg.FirstEnabledServerKey() != "sk-local-1" {
		t.Errorf("rotation=%+v firstKey=%q", cfg.Rotation, cfg.FirstEnabledServerKey())
	}
}

func TestLoadOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{
  "listen": "0.0.0.0:9000",
  "prefer": "zen",
  "server_keys": ["sk-local-1"],
  "zen_keys": ["zen-1"],
  "retry": {"max_attempts": 5, "timeout_seconds": 60},
  "logging": {"level": "debug", "ring_size": 500},
  "proxies": ["socks5://p.example:1080"],
  "rotation": {"sota": {"alias": "top", "order": ["glm-5.3", "grok-4.6", "kimi-k3", "qwen3.8-max", "deepseek-v4.1-flash"]}, "sweet": {"alias": "nice", "order": ["glm-4.6"]}, "first_output_seconds": 12},
  "reasoning": {"effort": " HIGH ", "effort_by_model": {" glm-5.3 ": "MAX"}}
}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != "0.0.0.0:9000" || cfg.Prefer != TierZen {
		t.Errorf("Listen=%q Prefer=%q", cfg.Listen, cfg.Prefer)
	}
	if cfg.Retry.MaxAttempts != 5 || cfg.Retry.TimeoutSeconds != 60 {
		t.Errorf("Retry = %+v", cfg.Retry)
	}
	if cfg.Rotation.SOTA.Alias != "top" || cfg.Rotation.Sweet.Alias != "nice" || cfg.Rotation.FirstOutputSeconds != 12 {
		t.Errorf("Rotation = %+v", cfg.Rotation)
	}
	// The stored SOTA order is kept exactly as configured, not re-sorted.
	if cfg.Rotation.SOTA.Order[0] != "glm-5.3" || cfg.Rotation.SOTA.Order[4] != "deepseek-v4.1-flash" {
		t.Errorf("SOTA.Order = %v", cfg.Rotation.SOTA.Order)
	}
	if cfg.Reasoning.Effort != "high" || cfg.Reasoning.EffortByModel["glm-5.3"] != "max" {
		t.Errorf("Reasoning = %+v", cfg.Reasoning)
	}
	if cfg.ForcedEffort("glm-5.3") != "max" {
		t.Errorf("ForcedEffort = %q, want max", cfg.ForcedEffort("glm-5.3"))
	}
	if want := []string{"socks5://p.example:1080"}; !reflect.DeepEqual(cfg.RuntimeProxies(), want) {
		t.Errorf("RuntimeProxies() = %v, want %v", cfg.RuntimeProxies(), want)
	}
}

func TestLoadFailures(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "not JSON",
			body: "not json at all",
			want: "parse",
		},
		{
			name: "unknown field",
			body: `{"server_keys": ["k"], "zen_keys": ["z"], "surprise": 1}`,
			want: "unknown field",
		},
		{
			name: "unterminated block comment",
			body: "{ /* open",
			want: "unterminated block comment",
		},
		{
			name: "trailing content",
			body: `{"server_keys": ["k"], "zen_keys": ["z"]} {"extra": 1}`,
			want: "multiple JSON values",
		},
		{
			name: "validation failure",
			body: `{"server_keys": ["k"], "zen_keys": ["z"], "logging": {"level": "verbose", "ring_size": 2000}}`,
			want: "logging.level must be debug, info, warn, or error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(tt.body), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			if err == nil {
				t.Fatalf("Load = nil, want error containing %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error = %q, want it to contain %q", err.Error(), tt.want)
			}
		})
	}

	t.Run("missing file", func(t *testing.T) {
		_, err := Load(filepath.Join(t.TempDir(), "absent.json"))
		if err == nil {
			t.Fatal("Load = nil, want an error")
		}
		if !strings.Contains(err.Error(), "read ") {
			t.Fatalf("error = %q, want it to mention the read failure", err.Error())
		}
	})
}

func TestLoadAcceptsABOMAndComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := "\ufeff{\r\n  /* block */\r\n  \"server_keys\": [\"sk-local-1\"],\r\n  \"zen_keys\": [\"zen-1\"] // inline\r\n}\r\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.ServerKeys) != 1 || cfg.ServerKeys[0] != "sk-local-1" {
		t.Fatalf("ServerKeys = %v", cfg.ServerKeys)
	}
}

// ------------------------------------------------------------------- saving --

func TestSaveAtomicRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := baseConfig()
	cfg.Proxies = []string{"socks5://p.example:1080"}
	normalized, err := Normalize(path, cfg)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if err := SaveAtomic(path, normalized); err != nil {
		t.Fatalf("SaveAtomic: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("saved config does not end with a newline")
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("saved config is not valid JSON: %v", err)
	}
	// Resolved proxies must never be persisted.
	if !strings.Contains(string(data), `"proxies"`) {
		t.Error("proxies field missing from the saved config")
	}
	if strings.Contains(string(data), "effectiveProxies") {
		t.Error("effectiveProxies leaked into the saved config")
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if !reflect.DeepEqual(reloaded.ServerKeys, normalized.ServerKeys) {
		t.Errorf("ServerKeys after round trip = %v, want %v", reloaded.ServerKeys, normalized.ServerKeys)
	}
	if want := []string{"socks5://p.example:1080"}; !reflect.DeepEqual(reloaded.RuntimeProxies(), want) {
		t.Errorf("RuntimeProxies after round trip = %v, want %v", reloaded.RuntimeProxies(), want)
	}

	// A second save keeps the previous file as config.json.bak.
	if err := SaveAtomic(path, normalized); err != nil {
		t.Fatalf("second SaveAtomic: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
}
