package identity

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
)

// bodyJSON decodes a JSON object literal into the map shape the gateway uses
// after request-body decoding.
func bodyJSON(t *testing.T, raw string) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("decode body %s: %v", raw, err)
	}
	return body
}

func TestConversationSeed(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
		want string
	}{
		{
			name: "nil body has no seed",
			body: nil,
			want: "",
		},
		{
			name: "empty body has no seed",
			body: map[string]any{},
			want: "",
		},
		{
			name: "string input is returned verbatim, not JSON encoded",
			body: map[string]any{"input": "hello there"},
			want: "hello there",
		},
		{
			name: "empty string input is skipped",
			body: map[string]any{"input": ""},
			want: "",
		},
		{
			name: "first user message content is JSON encoded",
			body: map[string]any{"messages": []any{
				map[string]any{"role": "assistant", "content": "prior answer"},
				map[string]any{"role": "user", "content": "first question"},
			}},
			want: `"first question"`,
		},
		{
			name: "later user turns never win over the first",
			body: map[string]any{"messages": []any{
				map[string]any{"role": "user", "content": "first"},
				map[string]any{"role": "user", "content": "second"},
			}},
			want: `"first"`,
		},
		{
			name: "assistant turns are ignored",
			body: map[string]any{"messages": []any{
				map[string]any{"role": "assistant", "content": "assistant text"},
			}},
			want: "",
		},
		{
			name: "role match is case sensitive",
			body: map[string]any{"messages": []any{
				map[string]any{"role": "User", "content": "capitalised"},
			}},
			want: "",
		},
		{
			name: "null content is skipped and the next user turn wins",
			body: map[string]any{"messages": []any{
				map[string]any{"role": "user", "content": nil},
				map[string]any{"role": "user", "content": "second"},
			}},
			want: `"second"`,
		},
		{
			name: "missing content is skipped",
			body: map[string]any{"messages": []any{
				map[string]any{"role": "user"},
				map[string]any{"role": "user", "content": "has content"},
			}},
			want: `"has content"`,
		},
		{
			name: "non-map entries are skipped",
			body: map[string]any{"messages": []any{
				"plain string",
				42,
				map[string]any{"role": "user", "content": "survivor"},
			}},
			want: `"survivor"`,
		},
		{
			name: "structured content is preserved as JSON",
			body: map[string]any{"messages": []any{
				map[string]any{"role": "user", "content": []any{
					map[string]any{"type": "text", "text": "part"},
				}},
			}},
			want: `[{"text":"part","type":"text"}]`,
		},
		{
			name: "input array is inspected when there is no string input",
			body: map[string]any{"input": []any{
				map[string]any{"role": "user", "content": "array input"},
			}},
			want: `"array input"`,
		},
		{
			name: "messages are searched before the input array",
			body: map[string]any{
				"messages": []any{map[string]any{"role": "user", "content": "from messages"}},
				"input":    []any{map[string]any{"role": "user", "content": "from input"}},
			},
			want: `"from messages"`,
		},
		{
			name: "input array is used when messages yield nothing",
			body: map[string]any{
				"messages": []any{map[string]any{"role": "assistant", "content": "only assistant"}},
				"input":    []any{map[string]any{"role": "user", "content": "from input"}},
			},
			want: `"from input"`,
		},
		{
			name: "non-string non-array input yields no seed",
			body: map[string]any{"input": 42.0},
			want: "",
		},
		{
			name: "unrelated fields are ignored",
			body: map[string]any{"model": "gpt", "stream": true},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := conversationSeed(tc.body); got != tc.want {
				t.Fatalf("conversationSeed() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConversationSeedIsStable(t *testing.T) {
	first := bodyJSON(t, `{"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"}]}`)
	again := bodyJSON(t, `{"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"}]}`)
	if a, b := conversationSeed(first), conversationSeed(again); a != b {
		t.Fatalf("seed is not stable: %q != %q", a, b)
	}
}

func TestConversationSeedCollisions(t *testing.T) {
	cases := []struct {
		name  string
		left  map[string]any
		right map[string]any
		same  bool
	}{
		{
			name:  "identical first user turn collides across every other field",
			left:  bodyJSON(t, `{"model":"a","messages":[{"role":"user","content":"same"}],"stream":true}`),
			right: bodyJSON(t, `{"model":"b","messages":[{"role":"user","content":"same"}],"stream":false}`),
			same:  true,
		},
		{
			name:  "extra fields on the user item are ignored",
			left:  bodyJSON(t, `{"messages":[{"role":"user","content":"same"}]}`),
			right: bodyJSON(t, `{"messages":[{"role":"user","content":"same","name":"alice"}]}`),
			same:  true,
		},
		{
			name:  "different first user content does not collide",
			left:  bodyJSON(t, `{"messages":[{"role":"user","content":"one"}]}`),
			right: bodyJSON(t, `{"messages":[{"role":"user","content":"two"}]}`),
			same:  false,
		},
		{
			name:  "later turns do not participate",
			left:  bodyJSON(t, `{"messages":[{"role":"user","content":"one"},{"role":"user","content":"a"}]}`),
			right: bodyJSON(t, `{"messages":[{"role":"user","content":"one"},{"role":"user","content":"b"}]}`),
			same:  true,
		},
		{
			name:  "whitespace in content changes the seed",
			left:  bodyJSON(t, `{"messages":[{"role":"user","content":"one"}]}`),
			right: bodyJSON(t, `{"messages":[{"role":"user","content":"one "}]}`),
			same:  false,
		},
		{
			name:  "a string input and a message with the same text do NOT collide: the string form is not JSON encoded",
			left:  bodyJSON(t, `{"input":"one"}`),
			right: bodyJSON(t, `{"messages":[{"role":"user","content":"one"}]}`),
			same:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			left, right := conversationSeed(tc.left), conversationSeed(tc.right)
			if (left == right) != tc.same {
				t.Fatalf("seeds %q and %q: collision=%v, want %v", left, right, left == right, tc.same)
			}
		})
	}
}

func TestBase62Fixed(t *testing.T) {
	cases := []struct {
		name  string
		value *big.Int
		width int
		want  string
	}{
		{"zero is all leading zeros", big.NewInt(0), 14, "00000000000000"},
		{"width one zero", big.NewInt(0), 1, "0"},
		{"alphabet index 9", big.NewInt(9), 1, "9"},
		{"alphabet index 10 is uppercase A", big.NewInt(10), 1, "A"},
		{"alphabet index 35 is uppercase Z", big.NewInt(35), 1, "Z"},
		{"alphabet index 36 is lowercase a", big.NewInt(36), 1, "a"},
		{"alphabet index 61 is lowercase z", big.NewInt(61), 1, "z"},
		{"62 rolls over to two digits", big.NewInt(62), 2, "10"},
		{"61 in two digits is 0z (no leading digit)", big.NewInt(61), 2, "0z"},
		{"zero width produces an empty string", big.NewInt(12345), 0, ""},
		{"value wider than the field is truncated to zeros", new(big.Int).Exp(big.NewInt(62), big.NewInt(14), nil), 14, "00000000000000"},
		{"value wider than the field truncates leading digits", big.NewInt(62*62*62 + 5), 2, "05"},
		{"large value fills the canonical width", new(big.Int).SetBytes([]byte{
			0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x10, 0x32,
		}), 14, "01fFltgbRs8yPq"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := base62Fixed(tc.value, tc.width)
			if got != tc.want {
				t.Fatalf("base62Fixed(%s, %d) = %q, want %q", tc.value, tc.width, got, tc.want)
			}
			if len(got) != tc.width {
				t.Fatalf("width = %d, want %d", len(got), tc.width)
			}
		})
	}
}

func TestBase62FixedIsDestructive(t *testing.T) {
	// base62Fixed divides the supplied *big.Int in place; callers must not
	// reuse the value afterwards.
	value := big.NewInt(100)
	if got := base62Fixed(value, 14); len(got) != 14 {
		t.Fatalf("unexpected width %d", len(got))
	}
	if value.Sign() != 0 {
		t.Fatalf("input mutated to %s, want 0", value)
	}
}

func TestBase62FixedNegativeUsesEuclideanRemainder(t *testing.T) {
	// big.Int.DivMod is Euclidean, so a negative value never yields a negative
	// digit index; -1 encodes as the maximum digit repeated.
	if got := base62Fixed(big.NewInt(-1), 3); got != "zzz" {
		t.Fatalf("base62Fixed(-1, 3) = %q, want %q", got, "zzz")
	}
}

func TestBase62FixedUsesDocumentedAlphabet(t *testing.T) {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	if len(alphabet) != 62 {
		t.Fatalf("alphabet length = %d", len(alphabet))
	}
	for i := 0; i < 62; i++ {
		digit := base62Fixed(big.NewInt(int64(i)), 1)
		if digit != string(alphabet[i]) {
			t.Fatalf("digit %d = %q, want %q", i, digit, string(alphabet[i]))
		}
		if strings.ContainsAny(digit, " \t\n") {
			t.Fatalf("digit %d contains whitespace", i)
		}
	}
}
