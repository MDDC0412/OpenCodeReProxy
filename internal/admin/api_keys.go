package admin

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"opencode2api/internal/config"
	"opencode2api/internal/httpx"
)

type APIKeyView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Display   string     `json:"display"`
	Enabled   bool       `json:"enabled"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// apiKeyName returns the stored label for a key fingerprint, falling back to a
// positional name for keys that predate server_key_metadata.
func apiKeyName(cfg config.Config, id string, index int) string {
	if name := cfg.ServerKeyMetadata[id].Name; name != "" {
		return name
	}
	return fmt.Sprintf("已有 Key %d", index+1)
}

func apiKeyViews(cfg config.Config) []APIKeyView {
	items := make([]APIKeyView, 0, len(cfg.ServerKeys))
	for i, key := range cfg.ServerKeys {
		id := config.Fingerprint(key)
		item := APIKeyView{
			ID:      id,
			Name:    apiKeyName(cfg, id, i),
			Display: config.MaskValue(key),
			Enabled: !cfg.ServerKeyMetadata[id].Disabled,
		}
		if created := cfg.ServerKeyMetadata[id].CreatedAt; !created.IsZero() {
			item.CreatedAt = &created
		}
		items = append(items, item)
	}
	return items
}

func (a *Server) handleListAPIKeys(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"keys": apiKeyViews(a.manager.Config())})
}

func validateKeyName(name string) error {
	if name == "" || len([]rune(name)) > 80 {
		return errors.New("名称需为 1–80 个字符")
	}
	return nil
}

func addAPIKey(cfg *config.Config, name, key string) error {
	if err := validateKeyName(name); err != nil {
		return err
	}
	if len(key) < 16 || len(key) > 256 {
		return errors.New("自定义 Key 需为 16–256 位")
	}
	for _, ch := range key {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.') {
			return errors.New("Key 仅支持英文字母、数字、短横线、下划线和点")
		}
	}
	if len(cfg.ServerKeys) >= 200 {
		return errors.New("最多创建 200 个 API Key")
	}
	id := config.Fingerprint(key)
	for _, existing := range cfg.ServerKeys {
		if config.Fingerprint(existing) == id {
			return errors.New("此 API Key 已存在")
		}
	}
	if cfg.ServerKeyMetadata == nil {
		cfg.ServerKeyMetadata = map[string]config.ServerKeyMetadata{}
	}
	cfg.ServerKeys = append(cfg.ServerKeys, key)
	cfg.ServerKeyMetadata[id] = config.ServerKeyMetadata{Name: name, CreatedAt: time.Now().UTC()}
	return nil
}

func (a *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminError(w, 400, "invalid_request", err.Error())
		return
	}
	key := input.Value
	if key == "" {
		var random [24]byte
		if _, err := rand.Read(random[:]); err != nil {
			writeAdminError(w, 500, "generation_failed", "无法生成密钥")
			return
		}
		key = "sk-local-" + hex.EncodeToString(random[:])
	}
	_, err := a.manager.Update(func(cfg *config.Config) error { return addAPIKey(cfg, strings.TrimSpace(input.Name), key) })
	if err != nil {
		writeAdminError(w, 400, "key_create_failed", a.manager.Redact(err.Error()))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"key":  key,
		"id":   config.Fingerprint(key),
		"keys": apiKeyViews(a.manager.Config()),
	})
}

func changeAPIKey(cfg *config.Config, id string, name *string, enabled *bool, remove bool) error {
	index := -1
	for i, key := range cfg.ServerKeys {
		if config.Fingerprint(key) == id {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New("API Key 不存在，请刷新列表")
	}
	if cfg.ServerKeyMetadata == nil {
		cfg.ServerKeyMetadata = map[string]config.ServerKeyMetadata{}
	}
	if remove {
		cfg.ServerKeys = append(cfg.ServerKeys[:index], cfg.ServerKeys[index+1:]...)
		delete(cfg.ServerKeyMetadata, id)
	} else {
		meta := cfg.ServerKeyMetadata[id]
		if name != nil {
			value := strings.TrimSpace(*name)
			if err := validateKeyName(value); err != nil {
				return err
			}
			meta.Name = value
		}
		if enabled != nil {
			meta.Disabled = !*enabled
		}
		cfg.ServerKeyMetadata[id] = meta
	}
	if cfg.FirstEnabledServerKey() == "" {
		return errors.New("至少保留一个启用的 API Key，请先创建替代密钥")
	}
	return nil
}

func (a *Server) handleUpdateAPIKey(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name    *string `json:"name"`
		Enabled *bool   `json:"enabled"`
	}
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminError(w, 400, "invalid_request", err.Error())
		return
	}
	_, err := a.manager.Update(func(cfg *config.Config) error {
		return changeAPIKey(cfg, r.PathValue("id"), input.Name, input.Enabled, false)
	})
	if err != nil {
		writeAdminError(w, 400, "key_update_failed", a.manager.Redact(err.Error()))
		return
	}
	a.handleListAPIKeys(w, r)
}

func (a *Server) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	_, err := a.manager.Update(func(cfg *config.Config) error { return changeAPIKey(cfg, r.PathValue("id"), nil, nil, true) })
	if err != nil {
		writeAdminError(w, 400, "key_delete_failed", a.manager.Redact(err.Error()))
		return
	}
	a.handleListAPIKeys(w, r)
}

// ccswitchTarget describes one CC Switch import target. Claude writes the URL to
// ANTHROPIC_BASE_URL and appends /v1/messages itself, so it must receive the
// bare base. Codex writes base_url and appends /responses, while OpenCode writes
// options.baseURL for the @ai-sdk/openai-compatible client, which appends
// /chat/completions; both therefore need exactly one /v1.
type ccswitchTarget struct {
	suffix string
}

var ccswitchTargets = map[string]ccswitchTarget{
	"claude":   {},
	"codex":    {suffix: "/v1"},
	"opencode": {suffix: "/v1"},
}

// ccswitchEndpoint appends the target's path suffix exactly once.
func ccswitchEndpoint(base, suffix string) string {
	if suffix == "" || strings.HasSuffix(base, suffix) {
		return base
	}
	return base + suffix
}

// validateImportBase accepts only a plain http(s) origin: CC Switch writes this
// value straight into a client config, so a scheme, userinfo or path would let a
// caller redirect the generated link somewhere unexpected.
func validateImportBase(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("base 不能为空")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", errors.New("base 无法解析")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("base 只支持 http 或 https")
	}
	if parsed.Host == "" {
		return "", errors.New("base 缺少主机名")
	}
	if parsed.User != nil {
		return "", errors.New("base 不能包含用户信息")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", errors.New("base 不能包含路径")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("base 不能包含查询或片段")
	}
	return strings.TrimRight(trimmed, "/"), nil
}

func validateImportModel(model string) (string, error) {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return "", errors.New("model 不能为空")
	}
	if len([]rune(trimmed)) > 120 {
		return "", errors.New("model 长度不能超过 120")
	}
	for _, ch := range trimmed {
		if unicode.IsSpace(ch) || unicode.IsControl(ch) {
			return "", errors.New("model 不能包含空白或控制字符")
		}
	}
	return trimmed, nil
}

// handleCCSwitchImport issues a ccswitch:// deep link for one server key. The
// plaintext key leaves the server only inside the link the operator explicitly
// requested, and only for that single response.
func (a *Server) handleCCSwitchImport(w http.ResponseWriter, r *http.Request) {
	var input struct {
		App   string `json:"app"`
		Model string `json:"model"`
		Base  string `json:"base"`
	}
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminError(w, 400, "invalid_request", err.Error())
		return
	}

	app := strings.TrimSpace(input.App)
	target, ok := ccswitchTargets[app]
	if !ok {
		writeAdminError(w, 400, "invalid_app", "该客户端不支持 CC Switch 导入")
		return
	}

	model, err := validateImportModel(input.Model)
	if err != nil {
		writeAdminError(w, 400, "invalid_model", a.manager.Redact(err.Error()))
		return
	}

	base, err := validateImportBase(input.Base)
	if err != nil {
		writeAdminError(w, 400, "invalid_base", a.manager.Redact(err.Error()))
		return
	}

	cfg := a.manager.Config()
	id := r.PathValue("id")
	secret, index := "", -1
	for i, key := range cfg.ServerKeys {
		if config.Fingerprint(key) == id {
			secret, index = key, i
			break
		}
	}
	if secret == "" {
		writeAdminError(w, 400, "key_not_found", "API Key 不存在，请刷新列表")
		return
	}

	name := apiKeyName(cfg, id, index)
	query := url.Values{}
	query.Set("resource", "provider")
	query.Set("app", app)
	query.Set("name", name)
	query.Set("homepage", base)
	query.Set("endpoint", ccswitchEndpoint(base, target.suffix))
	query.Set("apiKey", secret)
	query.Set("model", model)
	query.Set("configFormat", "json")

	a.logger.Info("cc switch import link issued", "component", "auth",
		"event", "api_key_import_link_issued", "client_ip", clientIP(r), "key_id", id)

	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"url": "ccswitch://v1/import?" + query.Encode(),
	})
}
