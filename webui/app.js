const $ = (selector) => document.querySelector(selector),
  $$ = (selector) => [...document.querySelectorAll(selector)];

let activeModal = null;

function closeModal(restoreFocus = true) {
  if (!activeModal) return;
  const { node, trigger } = activeModal;
  activeModal = null;
  node.classList.add("hidden");
  $("#app").inert = false;
  $("#login").inert = false;
  document.body.classList.remove("modal-open");
  if (node.id === "modal") $("#modal-content").textContent = "";
  if (node.id === "copy-modal") $("#copy-value").value = "";
  if (node.id === "ccswitch-modal") {
    ccswitchGeneration++;
    ccswitchCombo?.destroy?.();
    ccswitchCombo = null;
    ccswitchTarget = null;
    $("#ccswitch-fallback").value = "";
    $("#ccswitch-fallback-field").classList.add("hidden");
  }
  if (restoreFocus && trigger?.isConnected) trigger.focus();
}

function openModal(id, focusSelector) {
  const trigger = document.activeElement;
  closeModal(false);
  const node = $(id);
  activeModal = { node, trigger };
  node.classList.remove("hidden");
  $("#app").inert = true;
  $("#login").inert = true;
  document.body.classList.add("modal-open");
  (node.querySelector(focusSelector || "button, input, textarea") || node).focus();
}

document.addEventListener("keydown", (event) => {
  if (!activeModal) return;
  if (event.key === "Escape") {
    event.preventDefault();
    closeModal();
  } else if (event.key === "Tab") {
    const controls = [
      ...activeModal.node.querySelectorAll("button, input, textarea, select, [tabindex='0']"),
    ].filter((node) => !node.disabled && node.getClientRects().length);
    const first = controls[0],
      last = controls.at(-1);
    if (!controls.length) return event.preventDefault();
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }
});
for (const modal of $$(".modal")) {
  let pointerStartedOnBackdrop = false;
  modal.addEventListener("pointerdown", (event) => {
    pointerStartedOnBackdrop = event.target === modal;
  });
  modal.addEventListener("click", (event) => {
    // Selecting a portaled option hides it on pointerdown. Its click may then
    // target the backdrop, although the gesture started inside the list.
    if (event.target === modal && pointerStartedOnBackdrop) closeModal();
    pointerStartedOnBackdrop = false;
  });
}

/* ---------- theme ---------- */
const themePresets = ["#0d9488", "#2563eb", "#7c3aed", "#db2777", "#ea580c", "#475569"];
const themeModes = [
  ["light", "白天"],
  ["dark", "黑夜"],
];
function readStoredTheme() {
  return window.o2apiTheme.get();
}

function applyTheme(theme) {
  const current = window.o2apiTheme.apply(theme);
  const label = current.mode === "dark" ? "切换到白天模式" : "切换到黑夜模式";
  $("#theme-toggle").title = label;
  $("#theme-toggle").setAttribute("aria-label", label);
}

function renderThemeModal() {
  const theme = readStoredTheme();
  const modes = $("#theme-modes");
  modes.replaceChildren();
  for (const [value, label] of themeModes) {
    const wrap = document.createElement("label");
    wrap.className = "choice";
    const radio = document.createElement("input");
    radio.type = "radio";
    radio.name = "theme-mode";
    radio.value = value;
    radio.checked = value === theme.mode;
    radio.onchange = () => applyTheme({ ...readStoredTheme(), mode: value });
    const span = document.createElement("span");
    span.textContent = label;
    wrap.append(radio, span);
    modes.appendChild(wrap);
  }

  const swatches = $("#theme-swatches");
  swatches.replaceChildren();
  for (const color of themePresets) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = `swatch${color === theme.accent ? " is-active" : ""}`;
    button.style.background = color;
    button.title = color;
    button.onclick = () => {
      applyTheme({ ...readStoredTheme(), accent: color });
      $("#theme-custom").value = color;
      $("#theme-hex").value = color;
      renderThemeSwatchState();
    };
    swatches.appendChild(button);
  }

  $("#theme-custom").value = theme.accent;
  $("#theme-hex").value = theme.accent;
}

function openThemeModal() {
  renderThemeModal();
  openModal("#theme-modal", "input:checked");
}

$("#theme-toggle").onclick = () =>
  applyTheme({
    ...readStoredTheme(),
    mode: readStoredTheme().mode === "dark" ? "light" : "dark",
  });
$("#theme-color").onclick = openThemeModal;
$("#theme-close").onclick = () => closeModal();
$("#theme-custom").oninput = (event) => {
  applyTheme({ ...readStoredTheme(), accent: event.target.value });
  $("#theme-hex").value = event.target.value;
  renderThemeSwatchState();
};
$("#theme-hex").onchange = (event) => {
  const value = event.target.value.trim();
  if (!/^#[0-9a-f]{6}$/i.test(value)) {
    toast("请输入 #RRGGBB 形式的颜色", true);
    return;
  }
  applyTheme({ ...readStoredTheme(), accent: value });
  $("#theme-custom").value = readStoredTheme().accent;
  renderThemeSwatchState();
};

function renderThemeSwatchState() {
  const accent = readStoredTheme().accent;
  for (const button of $$("#theme-swatches .swatch")) {
    button.classList.toggle("is-active", button.title.toLowerCase() === accent.toLowerCase());
  }
}

// theme.js initializes the saved palette before the stylesheet loads.
applyTheme(readStoredTheme());

// A searchable combo for long option lists. The model catalog runs to hundreds of
// entries, which a native <select> makes unusable, so long fields use this:
// typing filters, arrows move, Enter picks, Esc closes.
function makeCombobox(host, initial) {
  const state = { options: [], value: initial || "", open: false, active: 0, filtered: [] };
  const root = document.createElement("div");
  root.className = "combo";
  const input = document.createElement("input");
  input.type = "text";
  input.className = "combo-input";
  input.autocomplete = "off";
  input.id = `${host.id}-input`;
  input.setAttribute("role", "combobox");
  input.setAttribute("aria-autocomplete", "list");
  input.setAttribute("aria-expanded", "false");
  const label = document.querySelector(`label[for="${host.id}"], label[for="${input.id}"]`);
  if (label) label.htmlFor = input.id;
  const caret = document.createElement("span");
  caret.className = "combo-caret";
  caret.textContent = "▾";
  const list = document.createElement("div");
  list.id = `${host.id}-options`;
  list.className = "combo-list hidden";
  list.setAttribute("role", "listbox");
  input.setAttribute("aria-controls", list.id);
  const modal = host.closest(".modal");
  root.append(input, caret);
  (modal || root).appendChild(list);
  if (modal) list.classList.add("combo-portal");
  host.replaceChildren(root);

  const positionList = () => {
    if (!modal || !state.open) return;
    const rect = input.getBoundingClientRect();
    const below = window.innerHeight - rect.bottom - 12;
    const above = rect.top - 12;
    const height = Math.min(264, Math.max(below, above));
    list.style.width = `${rect.width}px`;
    list.style.left = `${rect.left}px`;
    list.style.maxHeight = `${height}px`;
    list.style.top = `${below >= Math.min(264, above) ? rect.bottom + 4 : rect.top - Math.min(list.scrollHeight, height) - 4}px`;
  };

  const renderList = () => {
    list.replaceChildren();
    if (!state.filtered.length) {
      const empty = document.createElement("div");
      empty.className = "combo-empty";
      empty.textContent = "无匹配项";
      list.appendChild(empty);
      input.removeAttribute("aria-activedescendant");
      positionList();
      return;
    }
    state.filtered.forEach((item, index) => {
      const option = document.createElement("div");
      option.className = `combo-option${index === state.active ? " is-active" : ""}`;
      option.textContent = item;
      option.title = item;
      option.id = `${list.id}-${index}`;
      option.setAttribute("role", "option");
      option.setAttribute("aria-selected", String(item === state.value));
      option.onpointerdown = (event) => {
        event.preventDefault();
        pick(item);
      };
      list.appendChild(option);
    });
    input.setAttribute("aria-activedescendant", `${list.id}-${state.active}`);
    positionList();
  };
  const filter = (query = "") => {
    query = query.trim().toLowerCase();
    state.filtered = query
      ? state.options.filter((item) => item.toLowerCase().includes(query))
      : state.options.slice();
    state.active = Math.min(state.active, Math.max(0, state.filtered.length - 1));
    renderList();
  };
  const openList = (search = false) => {
    const wasOpen = state.open;
    state.open = true;
    list.classList.remove("hidden");
    input.setAttribute("aria-expanded", "true");
    if (!wasOpen) {
      state.active = Math.max(0, state.options.indexOf(state.value));
      window.addEventListener("resize", positionList);
      window.addEventListener("scroll", positionList, true);
    }
    if (search) state.active = 0;
    filter(search ? input.value : "");
    list.children[state.active]?.scrollIntoView({ block: "nearest" });
  };
  const closeList = () => {
    state.open = false;
    list.classList.add("hidden");
    input.setAttribute("aria-expanded", "false");
    input.removeAttribute("aria-activedescendant");
    input.value = state.value;
    window.removeEventListener("resize", positionList);
    window.removeEventListener("scroll", positionList, true);
  };
  const pick = (item) => {
    state.value = item;
    input.value = item;
    closeList();
    input.dispatchEvent(new CustomEvent("combo:change", { bubbles: true, detail: item }));
  };

  input.addEventListener("focus", () => {
    openList();
    input.select();
  });
  input.addEventListener("click", () => {
    if (!state.open) openList();
  });
  input.addEventListener("input", () => openList(true));
  input.addEventListener("blur", closeList);
  input.addEventListener("keydown", (event) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      if (!state.open) {
        openList();
        return;
      }
      const step = event.key === "ArrowDown" ? 1 : -1;
      state.active = Math.max(0, Math.min(state.filtered.length - 1, state.active + step));
      renderList();
      list.children[state.active]?.scrollIntoView({ block: "nearest" });
    } else if (event.key === "Enter" && state.open) {
      event.preventDefault();
      if (state.filtered[state.active]) pick(state.filtered[state.active]);
    } else if (event.key === "Escape" && state.open) {
      event.preventDefault();
      event.stopPropagation();
      closeList();
    }
  });

  input.value = state.value;
  return {
    setOptions(next, preferred) {
      state.options = [...new Set(next || [])];
      const keep = state.options.includes(state.value);
      if (!keep) state.value = preferred || state.options[0] || "";
      input.value = state.value;
      if (state.open) filter();
    },
    get value() {
      return state.value;
    },
    focus: () => input.focus(),
    destroy: () => {
      closeList();
      list.remove();
    },
  };
}

let csrf = "",
  config = null,
  monitorData = null,
  debugData = null,
  lastPlayground = null,
  logEvents = [],
  eventSource = null,
  paused = false,
  monitorTimer = null,
  quotaTimer = null,
  logRenderPending = false,
  toastTimer = null;
let authGeneration = 0;
let monitorError = "";

// Old hashes fold into the merged pages; bookmarked links keep working.
// `playground` is now a page of its own, so only the older ones need aliasing.
const pageAliases = {
  usage: "overview",
  guide: "overview",
  diagnostics: "models",
  "model-quotas": "models",
  availability: "models",
};
const pages = {
  overview: "总览",
  playground: "请求台",
  access: "接入",
  models: "模型与额度",
  "api-keys": "API Key",
  rotation: "模型轮转",
  config: "配置",
  logs: "事件日志",
  account: "账号",
};
const resolvePage = (raw) => {
  const page = String(raw || "");
  const mapped = pageAliases[page] || page;
  return pages[mapped] ? mapped : "overview";
};

async function api(path, options = {}) {
  const generation = authGeneration;
  options.headers = { ...(options.headers || {}), Accept: "application/json" };
  if (options.body) options.headers["Content-Type"] = "application/json";
  if (csrf && !["GET", "HEAD"].includes(options.method || "GET"))
    options.headers["X-CSRF-Token"] = csrf;
  const response = await fetch(path, options);
  if (generation !== authGeneration) throw new Error("会话已结束");
  if (response.status === 401) {
    showLogin();
    throw new Error("登录已失效");
  }
  if (response.status === 204) return null;
  const data = await response.json().catch(() => ({}));
  if (generation !== authGeneration) throw new Error("会话已结束");
  if (!response.ok) throw new Error(data.error?.message || `HTTP ${response.status}`);
  return data;
}

function toast(message, bad = false) {
  const node = $("#toast");
  node.textContent = message;
  node.classList.toggle("bad-toast", bad);
  node.classList.remove("hidden");
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => node.classList.add("hidden"), 3600);
}

function showLogin() {
  authGeneration++;
  csrf = "";
  closeModal(false);
  config = monitorData = debugData = lastPlayground = rotationData = null;
  monitorError = "";
  rotationDirty = false;
  logEvents = [];
  $("#app").classList.add("hidden");
  $("#login").classList.remove("hidden");
  if (eventSource) eventSource.close();
  eventSource = null;
  clearInterval(monitorTimer);
  clearInterval(quotaTimer);
  clearCreatedKey();
  for (const form of ["#config-form", "#account-form", "#api-key-form"]) $(form).reset();
  $("#login-pass").value = "";
}

async function enterConsole(session) {
  csrf = session.csrf_token;
  $("#login").classList.add("hidden");
  $("#app").classList.remove("hidden");
  await loadConfig();
  await Promise.all([refreshMonitor(), loadDebugModels()]);
  clearInterval(monitorTimer);
  monitorTimer = setInterval(refreshMonitor, 3000);
  connectLogs();
  openPage(resolvePage(location.hash.slice(1)));
}

async function boot() {
  try {
    await enterConsole(await api("/api/auth/session"));
  } catch (_) {
    showLogin();
  }
}

$("#login-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  $("#login-error").textContent = "";
  try {
    const session = await api("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ username: $("#login-user").value, password: $("#login-pass").value }),
    });
    $("#login-pass").value = "";
    await enterConsole(session);
  } catch (error) {
    $("#login-error").textContent = error.message;
  }
});

$("#logout").onclick = async () => {
  try {
    await api("/api/auth/logout", { method: "POST", body: "{}" });
  } finally {
    showLogin();
  }
};

// Help text stays collapsed until the reader asks for it.
$$("[data-help]").forEach((button) => {
  button.type = "button";
  button.onclick = () => {
    const body = $(`#help-${button.dataset.help}`);
    const open = body.classList.toggle("hidden");
    button.setAttribute("aria-expanded", String(!open));
  };
});

function openPage(page) {
  if (!pages[page]) return;
  history.replaceState(null, "", `#${page}`);
  clearInterval(quotaTimer);
  $$(".nav button").forEach((button) =>
    button.classList.toggle("active", button.dataset.page === page),
  );
  $$(".page").forEach((node) => node.classList.toggle("hidden", node.id !== `page-${page}`));
  $("#page-title").textContent = pages[page];
  if (page === "access") renderAccessExamples();
  if (page === "playground" && !debugData) loadDebugModels();
  if (page === "models") {
    loadModelQuotas();
    quotaTimer = setInterval(() => {
      if (!document.hidden) loadModelQuotas();
    }, 60000);
    loadAvailability();
    loadDebugModels();
  }
  if (page === "api-keys") loadAPIKeys();
  if (page === "rotation") loadRotation();
}
$$(".nav button").forEach((button) => (button.onclick = () => openPage(button.dataset.page)));

/* ---------- config ---------- */

async function loadConfig() {
  config = await api("/api/config");
  fillConfig(config);
  renderAccessExamples();
  renderOverviewStatus();
}

function fillConfig(value) {
  $("#c-listen").value = value.listen;
  $("#c-prefer").value = value.prefer;
  $("#c-web-listen").value = value.webui.listen;
  $("#c-session").value = value.webui.session_ttl_minutes;
  $("#c-web-enabled").checked = value.webui.enabled;
  $("#c-anonymous").checked = !!value.anonymous;
  $("#c-proxyfile").value = value.proxyfile;
  $("#c-up-zen").value = value.upstream.zen;
  $("#c-up-go").value = value.upstream.go;
  $("#c-attempts").value = value.retry.max_attempts;
  $("#c-timeout").value = value.retry.timeout_seconds;
  $("#c-refresh").value = value.models.refresh_seconds;
  $("#c-protocols").value = JSON.stringify(value.models.protocols || {}, null, 2);
  $("#c-free-only").checked = !!value.models.free_only;
  $("#c-up-anonymous").value = value.upstream.anonymous;
  const reasoning = value.reasoning || {};
  $("#c-reasoning-effort").value = reasoning.effort || "";
  $("#c-reasoning-by-model").value = JSON.stringify(reasoning.effort_by_model || {}, null, 2);
  $("#c-idle").value = value.performance.max_idle_conns;
  $("#c-idle-host").value = value.performance.max_idle_conns_per_host;
  $("#c-max-host").value = value.performance.max_conns_per_host;
  $("#c-idle-timeout").value = value.performance.idle_conn_timeout_seconds;
  $("#c-connect").value = value.performance.connect_timeout_seconds;
  $("#c-cooldown").value = value.performance.failure_cooldown_seconds;
  $("#c-attempt-timeout").value = value.performance.attempt_timeout_seconds;
  $("#c-first-event-timeout").value = value.performance.first_event_timeout_seconds || 0;
  $("#c-level").value = value.logging.level;
  $("#c-ring").value = value.logging.ring_size;
  $("#c-dump-bodies").checked = !!value.logging.dump_request_bodies;
  ["server_keys", "zen_keys", "go_keys", "proxies"].forEach(renderChips);
}

function renderChips(name) {
  const host = $(`#${name}-chips`);
  host.replaceChildren();
  (config[name] || []).forEach((item) => {
    const chip = document.createElement("span"),
      text = document.createElement("span"),
      remove = document.createElement("button");
    chip.className = "chip";
    chip.dataset.id = item.id;
    text.textContent = item.display;
    remove.type = "button";
    remove.textContent = "×";
    remove.onclick = () => chip.remove();
    chip.append(text, remove);
    host.appendChild(chip);
  });
}

function secretInputs(name) {
  const kept = [...$(`#${name}-chips`).children].map((node) => ({ id: node.dataset.id }));
  const added = $(`#${name}-new`)
    .value.split(/\r?\n/)
    .map((value) => value.trim())
    .filter(Boolean)
    .map((value) => ({ value }));
  return kept.concat(added);
}

const number = (id) => Number($(id).value);

$("#config-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    const update = {
      listen: $("#c-listen").value,
      server_keys: secretInputs("server_keys"),
      zen_keys: secretInputs("zen_keys"),
      go_keys: secretInputs("go_keys"),
      anonymous: $("#c-anonymous").checked,
      proxies: secretInputs("proxies"),
      proxyfile: $("#c-proxyfile").value,
      prefer: $("#c-prefer").value,
      upstream: {
        zen: $("#c-up-zen").value,
        go: $("#c-up-go").value,
        anonymous: $("#c-up-anonymous").value,
      },
      retry: { max_attempts: number("#c-attempts"), timeout_seconds: number("#c-timeout") },
      models: {
        refresh_seconds: number("#c-refresh"),
        free_only: $("#c-free-only").checked,
        protocols: JSON.parse($("#c-protocols").value || "{}"),
      },
      reasoning: {
        effort: $("#c-reasoning-effort").value,
        effort_by_model: JSON.parse($("#c-reasoning-by-model").value || "{}"),
      },
      performance: {
        max_idle_conns: number("#c-idle"),
        max_idle_conns_per_host: number("#c-idle-host"),
        max_conns_per_host: number("#c-max-host"),
        idle_conn_timeout_seconds: number("#c-idle-timeout"),
        connect_timeout_seconds: number("#c-connect"),
        failure_cooldown_seconds: number("#c-cooldown"),
        attempt_timeout_seconds: number("#c-attempt-timeout"),
        first_event_timeout_seconds: number("#c-first-event-timeout"),
      },
      logging: {
        level: $("#c-level").value,
        ring_size: number("#c-ring"),
        dump_request_bodies: $("#c-dump-bodies").checked,
      },
      webui: {
        enabled: $("#c-web-enabled").checked,
        listen: $("#c-web-listen").value,
        username: config.webui.username,
        session_ttl_minutes: number("#c-session"),
      },
    };
    const result = await api("/api/config", { method: "PUT", body: JSON.stringify(update) });
    config = result.config;
    ["server_keys", "zen_keys", "go_keys", "proxies"].forEach(
      (name) => ($(`#${name}-new`).value = ""),
    );
    fillConfig(config);
    await loadDebugModels();
    toast(
      result.result.restart_required
        ? `已应用；${result.result.restart_fields.join(", ")} 需重启`
        : "配置已保存并立即应用",
    );
  } catch (error) {
    toast(error.message, true);
  }
});

$("#reload-config").onclick = async () => {
  try {
    const result = await api("/api/config/reload", { method: "POST", body: "{}" });
    await loadConfig();
    await loadDebugModels();
    toast(result.restart_required ? "磁盘配置已重载，监听字段需重启" : "磁盘配置已重载");
  } catch (error) {
    toast(error.message, true);
  }
};

$("#reveal").onclick = async () => {
  const password = prompt("请再次输入管理密码");
  if (password === null) return;
  try {
    const values = await api("/api/config/reveal", {
      method: "POST",
      body: JSON.stringify({ password }),
    });
    $("#modal-content").textContent = JSON.stringify(values, null, 2);
    openModal("#modal");
  } catch (error) {
    toast(error.message, true);
  }
};

$("#modal-close").onclick = () => closeModal();

$("#account-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  try {
    await api("/api/account", {
      method: "PUT",
      body: JSON.stringify({
        current_password: $("#a-current").value,
        username: $("#a-user").value,
        new_password: $("#a-new").value,
      }),
    });
    toast("账号已更新，请重新登录");
    setTimeout(showLogin, 800);
  } catch (error) {
    toast(error.message, true);
  }
});

/* ---------- shared render helpers ---------- */

function appendCell(row, value, className = "") {
  const cell = document.createElement("td");
  cell.textContent = String(value);
  if (className) cell.className = className;
  row.appendChild(cell);
}

function emptyRow(host, columns, message = "暂无数据") {
  const row = document.createElement("tr"),
    cell = document.createElement("td");
  cell.colSpan = columns;
  cell.className = "empty";
  cell.textContent = message;
  row.appendChild(cell);
  host.appendChild(row);
}

function statusCell(text, tone) {
  const span = document.createElement("span");
  span.className = `status status-${tone}`;
  span.textContent = text;
  return span;
}

const formatTokens = (value) => Number(value || 0).toLocaleString();

/* ---------- overview ---------- */

async function refreshMonitor() {
  try {
    monitorData = await api("/api/monitor");
    monitorError = "";
    const metrics = monitorData.metrics;
    $("#version").textContent = `v${monitorData.version}`;
    $("#m-total").textContent = Number(metrics.last_hour.total || 0).toLocaleString();
    const rate = metrics.last_hour.success_rate || 0;
    const hasRequests = metrics.last_hour.total > 0;
    $("#m-rate").textContent = hasRequests ? `${(rate * 100).toFixed(1)}%` : "暂无请求";
    $("#m-rate").className =
      `stat-value ${!hasRequests ? "is-idle" : rate < 0.9 ? "is-bad" : rate < 0.99 ? "is-warn" : "is-ok"}`;
    $("#m-p95").textContent = hasRequests ? `${metrics.last_hour.p95_ms || 0} ms` : "—";
    $("#m-active").textContent = metrics.active_requests || 0;
    $("#m-streams").textContent = metrics.active_streams || 0;
    renderUsage();
    renderResources();
    renderBreakdowns();
    renderOverviewStatus();
    renderRecentRequests();
    renderLastPlayground();
    return true;
  } catch (error) {
    if (csrf) {
      monitorError = error.message;
      renderOverviewStatus();
    }
    return false;
  }
}

$("#refresh-now").onclick = async () => {
  try {
    const [monitorOK] = await Promise.all([refreshMonitor(), loadConfig(), loadDebugModels()]);
    const page = resolvePage(location.hash.slice(1));
    if (page === "models") await loadModelQuotas();
    if (page === "api-keys") await loadAPIKeys();
    if (page === "rotation") await loadRotation();
    if (monitorOK) toast("状态已刷新");
  } catch (error) {
    toast(error.message, true);
  }
};

function renderResources() {
  const keys = $("#key-table");
  keys.replaceChildren();
  const resources = monitorData?.resources || {};
  if (resources.anonymous) {
    const row = document.createElement("tr");
    appendCell(row, "anonymous");
    appendCell(row, "zen");
    const cell = document.createElement("td");
    cell.appendChild(statusCell("启用", "ok"));
    row.appendChild(cell);
    keys.appendChild(row);
  }
  (resources.keys || []).forEach((key) => {
    const row = document.createElement("tr");
    appendCell(row, key.id);
    appendCell(row, key.tier);
    const cell = document.createElement("td");
    cell.appendChild(key.cooldown_until ? statusCell("冷却", "warn") : statusCell("正常", "ok"));
    row.appendChild(cell);
    keys.appendChild(row);
  });
  if (!keys.children.length) emptyRow(keys, 3);

  const proxies = $("#proxy-table");
  proxies.replaceChildren();
  (resources.proxies || []).forEach((proxy) => {
    const row = document.createElement("tr");
    appendCell(row, proxy.address);
    const cell = document.createElement("td");
    cell.appendChild(proxy.healthy ? statusCell("正常", "ok") : statusCell("不可用", "bad"));
    row.appendChild(cell);
    appendCell(row, `Z ${proxy.zen_keys} · G ${proxy.go_keys}`);
    proxies.appendChild(row);
  });
  if (!proxies.children.length) emptyRow(proxies, 3);
}

function renderBreakdowns() {
  const host = $("#breakdowns");
  host.replaceChildren();
  const metrics = monitorData?.metrics;
  [
    ["端点", metrics?.endpoints],
    ["模型", metrics?.models],
    ["Tier", metrics?.tiers],
  ].forEach(([title, data]) => {
    const block = document.createElement("div"),
      table = document.createElement("table"),
      body = document.createElement("tbody");
    table.className = "table";
    Object.entries(data || {})
      .sort((a, b) => b[1] - a[1])
      .slice(0, 5)
      .forEach(([key, value]) => {
        const row = document.createElement("tr");
        appendCell(row, key);
        appendCell(row, value);
        body.appendChild(row);
      });
    if (!body.children.length) emptyRow(body, 2, "暂无请求");
    const head = document.createElement("div");
    head.className = "card-note";
    head.textContent = title;
    table.appendChild(body);
    block.append(head, table);
    host.appendChild(block);
  });
}

function renderUsage() {
  if (!monitorData) return;
  const usage = monitorData.usage?.last_hour || {},
    tokens = usage.tokens || {};
  $("#u-coverage").textContent = `${((usage.coverage || 0) * 100).toFixed(1)}%`;
  $("#u-total").textContent = formatTokens(tokens.total_tokens);
  $("#u-cached").textContent = formatTokens(tokens.cached_tokens);
  $("#u-reasoning").textContent = formatTokens(tokens.reasoning_tokens);

  const trend = $("#usage-trend"),
    series = monitorData.metrics?.series || [],
    maximum = Math.max(1, ...series.map((point) => point.total_tokens || 0));
  trend.replaceChildren();
  const hasTrend = series.some((point) => point.total_tokens > 0);
  trend.classList.toggle("trend-empty", !hasTrend);
  if (!hasTrend) trend.textContent = "尚无 Token 用量";
  series
    .filter(() => hasTrend)
    .forEach((point) => {
      const bar = document.createElement("div");
      bar.className = "trend-bar";
      bar.style.height = `${Math.max(2, Math.round(((point.total_tokens || 0) / maximum) * 100))}%`;
      bar.title = `${new Date(point.minute).toLocaleTimeString()} · 输入 ${formatTokens(point.input_tokens)} · 输出 ${formatTokens(point.output_tokens)} · 总量 ${formatTokens(point.total_tokens)}`;
      trend.appendChild(bar);
    });

  const models = $("#usage-models");
  models.replaceChildren();
  Object.entries(usage.models || {})
    .sort((a, b) => (b[1].total_tokens || 0) - (a[1].total_tokens || 0))
    .slice(0, 15)
    .forEach(([model, value]) => {
      const row = document.createElement("tr");
      appendCell(row, model);
      appendCell(row, formatTokens(value.total_tokens));
      models.appendChild(row);
    });
  if (!models.children.length) emptyRow(models, 2, "尚无 Token 用量");

  const tiers = $("#usage-tiers"),
    entries = Object.entries(usage.tiers || {}).sort(
      (a, b) => (b[1].total_tokens || 0) - (a[1].total_tokens || 0),
    ),
    total = Math.max(1, ...entries.map(([, value]) => value.total_tokens || 0));
  tiers.replaceChildren();
  entries.forEach(([tier, value]) => {
    const row = document.createElement("div"),
      name = document.createElement("span"),
      track = document.createElement("div"),
      fill = document.createElement("div"),
      amount = document.createElement("span");
    row.className = "bar-row";
    name.textContent = tier;
    track.className = "bar-track";
    fill.className = "bar-fill";
    fill.style.width = `${Math.round(((value.total_tokens || 0) / total) * 100)}%`;
    amount.textContent = formatTokens(value.total_tokens);
    track.appendChild(fill);
    row.append(name, track, amount);
    tiers.appendChild(row);
  });
  if (!entries.length) {
    const empty = document.createElement("div");
    empty.className = "empty";
    empty.textContent = "尚无 Tier 用量";
    tiers.appendChild(empty);
  }
}

// The banner, checklist and actionable alerts share one status snapshot.
function overviewChecks() {
  const checks = [];
  if (config && monitorData) {
    const resources = monitorData.resources || {},
      models = resources.models || {},
      proxies = resources.proxies || [],
      unhealthy = proxies.filter((proxy) => !proxy.healthy),
      zen = (config.zen_keys || []).length,
      go = (config.go_keys || []).length,
      local = (config.server_keys || []).length,
      upstream = zen + go > 0 || config.anonymous,
      metrics = monitorData.metrics?.last_hour || {},
      total = Number(metrics.total || 0),
      rate = Number(metrics.success_rate || 0),
      percent = `${(rate * 100).toFixed(1)}%`;
    checks.push(
      {
        id: "login",
        label: "管理登录",
        tone: "ok",
        status: "通过",
        detail: "当前管理会话有效。",
      },
      {
        id: "local-key",
        label: "本地 Key",
        tone: local ? "ok" : "warn",
        status: local ? "已配置" : "未配置",
        detail: `已配置 ${local} 个本地 Key。`,
        problem: "未配置本地 Key",
        page: "api-keys",
        action: "配置 API Key",
      },
      {
        id: "upstream",
        label: "上游通道",
        tone: upstream ? "ok" : "warn",
        status: upstream ? "已配置" : "未配置",
        detail: `${zen} 个 Zen Key · ${go} 个 Go Key · 匿名通道${config.anonymous ? "已启用" : "未启用"}`,
        problem: "上游通道未配置",
        page: "config",
        action: "检查上游配置",
      },
      {
        id: "proxy",
        label: "代理节点",
        tone: proxies.length && !unhealthy.length ? "ok" : "warn",
        status: proxies.length && !unhealthy.length ? "正常" : "不可用",
        detail: proxies.length
          ? `${proxies.length - unhealthy.length}/${proxies.length} 个节点健康${unhealthy.length ? `；不可用：${unhealthy.map((proxy) => proxy.address).join("、")}` : ""}`
          : "没有可用的代理节点。",
        problem: "代理节点不可用",
        page: "config",
        action: "检查代理配置",
      },
      {
        id: "models",
        label: "模型目录",
        tone: models.exposed > 0 && !models.stale ? "ok" : "warn",
        status: models.exposed > 0 ? (models.stale ? "已过期" : "就绪") : "无可用模型",
        detail: `当前配置下 ${models.exposed || 0} 个可路由模型${models.stale ? "；目录已过期，等待后台刷新。" : "。"}`,
        problem: models.exposed > 0 ? "模型目录已过期" : "当前配置下没有可路由的模型",
        page: "models",
        action: "查看模型状态",
      },
      {
        id: "requests",
        label: "请求成功率",
        tone: total ? (rate < 0.9 ? "warn" : "ok") : "idle",
        status: total ? (rate < 0.9 ? "偏低" : "正常") : "暂无请求",
        detail: total
          ? `近 1 小时 ${total} 次请求，${Number(metrics.success || 0)} 次成功，${Number(metrics.errors || 0)} 次失败${rate < 0.9 ? "；低于 90% 检查阈值。" : "。"}`
          : "近 1 小时没有请求记录。",
        problem: `请求成功率偏低：${percent}`,
        page: "requests",
        action: "查看失败请求",
      },
    );
  }
  if (monitorError)
    checks.push({
      id: "monitor",
      label: "监控更新",
      tone: "warn",
      status: "失败",
      detail: `监控更新失败：${monitorError}。${monitorData ? "当前显示上次成功取得的数据。" : "当前运行状态未知。"}`,
      problem: "无法获取最新运行状态",
      page: "refresh",
      action: "重试更新",
    });
  return checks;
}

function renderOverviewStatus() {
  const checks = overviewChecks();
  if (!checks.length) return;
  const problems = checks.filter((item) => item.tone === "warn"),
    passed = checks.filter((item) => item.tone === "ok").length,
    idle = checks.some((item) => item.tone === "idle");
  const list = $("#guide-items");
  list.replaceChildren();
  for (const check of checks) {
    const item = document.createElement("li"),
      head = document.createElement("div"),
      name = document.createElement("span"),
      detail = document.createElement("p");
    item.dataset.check = check.id;
    item.dataset.tone = check.tone;
    head.className = "checklist-head";
    name.className = "checklist-label";
    name.textContent = check.label;
    detail.className = "checklist-detail";
    detail.textContent = check.detail;
    head.append(name, statusCell(check.status, check.tone));
    item.append(head, detail);
    list.appendChild(item);
  }
  $("#guide-summary").textContent = problems.length
    ? `状态检查 ${problems.length} 项待处理`
    : `状态检查 ${passed} 项通过`;
  $("#guide-box").classList.toggle("is-warn", problems.length > 0);
  $("#health").className = `health ${problems.length ? "is-bad" : idle ? "is-idle" : "is-ok"}`;
  $("#health-text").textContent = problems.length
    ? problems.length === 1
      ? problems[0].problem
      : `${problems.length} 项需要处理`
    : idle
      ? "配置就绪 · 暂无请求"
      : "运行正常";

  $("#attention").classList.toggle("hidden", problems.length === 0);
  const attention = $("#attention-list");
  attention.replaceChildren();
  for (const check of problems) {
    const item = document.createElement("div"),
      copy = document.createElement("div"),
      title = document.createElement("strong"),
      detail = document.createElement("div"),
      action = document.createElement("button");
    item.className = "attention-item";
    if (check.id === "monitor") item.id = "monitor-error";
    copy.className = "attention-copy";
    title.textContent = check.problem;
    detail.textContent = check.detail;
    action.type = "button";
    action.className = "btn btn-sm";
    action.textContent = check.action;
    action.onclick = () => {
      if (check.page === "requests") {
        $("#requests-failed-only").checked = true;
        renderRecentRequests();
        $("#recent-requests").scrollIntoView({ behavior: "smooth", block: "start" });
        $("#requests-failed-only").focus({ preventScroll: true });
      } else if (check.page === "refresh") refreshMonitor();
      else openPage(check.page);
    };
    copy.append(title, detail);
    item.append(copy, action);
    attention.appendChild(item);
  }
}

$("#guide-box").onclick = (event) => {
  const box = $("#guide-box");
  if (box.dataset.open === "1") {
    box.dataset.open = "0";
    box.setAttribute("aria-expanded", "false");
    $("#guide-items").classList.add("hidden");
    return;
  }
  box.dataset.open = "1";
  box.setAttribute("aria-expanded", "true");
  const list = $("#guide-items");
  if (list.childElementCount) list.classList.remove("hidden");
  event.stopPropagation();
};

function renderRecentRequests() {
  const host = $("#upstream-requests");
  const failedOnly = $("#requests-failed-only").checked;
  host.replaceChildren();
  (monitorData?.upstream?.requests || [])
    .filter((request) => !failedOnly || !request.success)
    .slice(-100)
    .reverse()
    .forEach((request) => {
      const row = document.createElement("tr");
      appendCell(row, new Date(request.time).toLocaleTimeString());
      appendCell(row, request.request_id);
      appendCell(row, request.model);
      appendCell(row, request.channel);
      const cell = document.createElement("td");
      const terminal = { stream_error: "流式失败", client_canceled: "客户端取消" };
      cell.appendChild(
        statusCell(
          terminal[request.outcome] || request.status || request.outcome || "未知",
          request.success ? "ok" : "bad",
        ),
      );
      row.appendChild(cell);
      appendCell(row, `${request.duration_ms} ms`);
      host.appendChild(row);
    });
  if (!host.children.length) emptyRow(host, 6, failedOnly ? "暂无失败路由记录" : "尚无请求路由");
}

$("#requests-failed-only").addEventListener("change", renderRecentRequests);

/* ---------- access examples ---------- */

function derivedAPIBase() {
  if (!config) return `${location.protocol}//${location.hostname}:8080`;
  const listen = config.effective?.listen || config.listen || ":8080";
  const bracket = listen.match(/^\[.*\]:(\d+)$/),
    plain = listen.match(/:(\d+)$/),
    port = (bracket || plain)?.[1] || "8080";
  return `${location.protocol}//${location.hostname}:${port}`;
}

async function copyText(text, fallback = true) {
  const generation = authGeneration;
  try {
    await navigator.clipboard.writeText(text);
    if (generation !== authGeneration) return false;
    toast("已复制");
    return true;
  } catch {
    if (generation !== authGeneration) return false;
    toast("浏览器未允许复制，请手动选中复制", true);
    if (fallback) {
      $("#copy-value").value = text;
      openModal("#copy-modal", "textarea");
      $("#copy-value").select();
    }
    return false;
  }
}
$("#copy-close").onclick = () => closeModal();

function codeCard(label, text) {
  const card = document.createElement("div"),
    head = document.createElement("div"),
    title = document.createElement("span"),
    copy = document.createElement("button"),
    pre = document.createElement("pre"),
    code = document.createElement("code");
  card.className = "code-card";
  head.className = "code-head";
  title.textContent = label;
  copy.type = "button";
  copy.className = "btn btn-sm";
  copy.textContent = "复制";
  copy.onclick = () => copyText(text);
  head.append(title, copy);
  code.textContent = text;
  pre.appendChild(code);
  card.append(head, pre);
  return card;
}

function renderAccessExamples() {
  if (!config) return;
  const base = derivedAPIBase(),
    model = $("#access-model")?.value.trim() || "YOUR_MODEL";
  $("#access-base-value").value = base;
  const protocols = $("#access-protocol-examples"),
    sdks = $("#access-sdk-examples");
  protocols.replaceChildren();
  sdks.replaceChildren();
  const chat = `curl ${base}/v1/chat/completions \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model}","messages":[{"role":"user","content":"Hello"}]}'`;
  const responses = `curl ${base}/v1/responses \\\n  -H "Authorization: Bearer YOUR_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model}","input":"Hello"}'`;
  const anthropic = `curl ${base}/v1/messages \\\n  -H "x-api-key: YOUR_API_KEY" \\\n  -H "anthropic-version: 2023-06-01" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model}","max_tokens":512,"messages":[{"role":"user","content":"Hello"}]}'`;
  protocols.append(
    codeCard("Chat Completions", chat),
    codeCard("Responses", responses),
    codeCard("Anthropic Messages", anthropic),
  );
  const python = `from openai import OpenAI\n\nclient = OpenAI(base_url="${base}/v1", api_key="YOUR_API_KEY")\nresponse = client.chat.completions.create(\n    model="${model}",\n    messages=[{"role": "user", "content": "Hello"}],\n)\nprint(response.choices[0].message.content)`;
  const javascript = `import OpenAI from "openai";\n\nconst client = new OpenAI({\n  baseURL: "${base}/v1",\n  apiKey: "YOUR_API_KEY",\n});\nconst response = await client.responses.create({\n  model: "${model}",\n  input: "Hello",\n});\nconsole.log(response.output_text);`;
  sdks.append(
    codeCard("Python · OpenAI SDK", python),
    codeCard("JavaScript · OpenAI SDK", javascript),
  );
}

$("#access-model").addEventListener("input", renderAccessExamples);

/* ---------- playground ---------- */

let playModelCombo = null;

function renderPlayKeys() {
  const select = $("#play-key");
  if (!select) return;
  const selected = select.value;
  select.replaceChildren();
  const auto = document.createElement("option");
  auto.value = "auto";
  auto.textContent = "自动路由";
  select.appendChild(auto);
  const keys = debugData?.keys || {};
  [
    ["zen", "Zen Keys"],
    ["go", "Go Keys"],
  ].forEach(([tier, label]) => {
    const items = keys[tier] || [];
    if (!items.length) return;
    const group = document.createElement("optgroup");
    group.label = label;
    items.forEach((item) => {
      const option = document.createElement("option");
      option.value = `${tier}:${item.id}`;
      const cooling = item.cooldown_until ? ` · 冷却中` : "";
      option.textContent = `${tier.toUpperCase()} · ${item.display}${cooling}`;
      group.appendChild(option);
    });
    select.appendChild(group);
  });
  if (selected && [...select.options].some((option) => option.value === selected))
    select.value = selected;
  else select.value = "auto";
}

async function loadDebugModels() {
  try {
    debugData = await api("/api/debug/models");
    if (debugData.last_inference) lastPlayground = debugData.last_inference;
    // The catalog runs to hundreds of models, so this field is a searchable combo
    // rather than a native <select>.
    const host = $("#play-model");
    if (host && !playModelCombo) playModelCombo = makeCombobox(host, "");
    playModelCombo?.setOptions((debugData.models || []).map((item) => item.model));
    renderPlayKeys();
    if (!$("#play-request").value.trim()) resetPlayTemplate();
    renderModelRouting();
    renderLastPlayground();
  } catch (error) {
    toast(error.message, true);
  }
}

function playgroundTemplate() {
  const protocol = $("#play-protocol").value,
    model = playModelCombo?.value || "YOUR_MODEL";
  if (protocol === "responses") return { model, input: "用一句话说明这个模型的能力。" };
  if (protocol === "anthropic")
    return {
      model,
      max_tokens: 512,
      messages: [{ role: "user", content: "用一句话说明这个模型的能力。" }],
    };
  return { model, messages: [{ role: "user", content: "用一句话说明这个模型的能力。" }] };
}

function resetPlayTemplate() {
  $("#play-request").value = JSON.stringify(playgroundTemplate(), null, 2);
}

$("#play-protocol").addEventListener("change", resetPlayTemplate);
// The combo lives inside #play-model, so listen on its input.
$("#play-model").addEventListener("combo:change", () => {
  try {
    const request = JSON.parse($("#play-request").value);
    request.model = playModelCombo?.value || "";
    $("#play-request").value = JSON.stringify(request, null, 2);
  } catch (_) {
    resetPlayTemplate();
  }
});
$("#play-reset").onclick = resetPlayTemplate;

function renderLastPlayground() {
  const last = lastPlayground || debugData?.last_inference;
  const host = $("#last-playground");
  if (!host) return;
  host.replaceChildren();
  if (!last) {
    host.appendChild(
      Object.assign(document.createElement("div"), {
        className: "empty",
        textContent: "尚无诊断追踪",
      }),
    );
    return;
  }
  const entries = [
    ["结果", last.ok ? "成功" : "失败"],
    ["HTTP", last.http_status],
    ["耗时", `${last.duration_ms} ms`],
    ["模型", last.route?.model || "—"],
  ];
  for (const [label, value] of entries) {
    const fact = document.createElement("dl");
    fact.className = "fact";
    const term = document.createElement("dt");
    const detail = document.createElement("dd");
    term.textContent = label;
    detail.textContent = value;
    fact.append(term, detail);
    host.appendChild(fact);
  }
}

$("#play-form").addEventListener("submit", async (event) => {
  event.preventDefault();
  let request;
  try {
    request = JSON.parse($("#play-request").value);
  } catch (error) {
    toast(`请求 JSON 无效：${error.message}`, true);
    return;
  }
  const keyValue = $("#play-key").value,
    key =
      keyValue === "auto"
        ? { mode: "auto" }
        : {
            mode: "selected",
            tier: keyValue.split(":")[0],
            id: keyValue.split(":").slice(1).join(":"),
          };
  $("#play-run").disabled = true;
  $("#play-state").textContent = key.mode === "selected" ? "指定 Key 测试中…" : "请求中…";
  try {
    const result = await api("/api/debug/inference", {
      method: "POST",
      body: JSON.stringify({ protocol: $("#play-protocol").value, key, request }),
    });
    displayPlayground(result);
  } catch (error) {
    toast(error.message, true);
    $("#play-state").textContent = "诊断入口请求失败";
  } finally {
    $("#play-run").disabled = false;
  }
});

function displayPlayground(result) {
  lastPlayground = result;
  $("#play-state").textContent = result.ok
    ? `成功 · HTTP ${result.http_status}`
    : `失败 · HTTP ${result.http_status}`;
  const facts = $("#play-facts");
  facts.replaceChildren();
  const entries = [
    ["HTTP", result.http_status],
    ["耗时", `${result.duration_ms} ms`],
    ["Request ID", result.request_id || "—"],
    ["模型", result.route?.model || "—"],
  ];
  for (const [label, value] of entries) {
    const fact = document.createElement("dl");
    fact.className = "fact";
    const term = document.createElement("dt");
    const detail = document.createElement("dd");
    term.textContent = label;
    detail.textContent = value;
    fact.append(term, detail);
    facts.appendChild(fact);
  }
  $("#play-output").textContent = JSON.stringify(result.response, null, 2);
  renderLastPlayground();
}

$("#play-copy").onclick = () => copyText($("#play-output").textContent);

/* ---------- models & quota ---------- */

let quotaData = null,
  quotaLoading = false;
const localTime = (value) =>
  value ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "—";

async function loadModelQuotas() {
  if (quotaLoading) return;
  quotaLoading = true;
  $("#quota-refresh").disabled = true;
  try {
    const value = await api("/api/model-quotas");
    if (!csrf) return;
    quotaData = value;
    const select = $("#quota-account"),
      selected = select.value;
    select.replaceChildren();
    for (const account of value.accounts || []) {
      select.appendChild(new Option(`Go Key ${account.display}`, account.id));
    }
    if ([...select.options].some((option) => option.value === selected)) select.value = selected;
    if (!select.options.length) select.appendChild(new Option("尚未配置 Go Key", ""));
    renderModelQuotas();
  } catch (error) {
    $("#quota-status").textContent = `查询失败：${error.message}`;
  } finally {
    quotaLoading = false;
    $("#quota-refresh").disabled = false;
  }
}

function renderModelQuotas() {
  if (!quotaData) return;
  const account = quotaData.accounts.find((item) => item.id === $("#quota-account").value);
  const stale = account?.status === "stale";
  const windows = [
    ["rolling", "5 小时窗口", true],
    ["weekly", "每周窗口", false],
    ["monthly", "每月窗口", false],
  ];
  const cards = $("#quota-cards");
  cards.replaceChildren();
  for (const [id, label, big] of windows) {
    const item = account?.windows?.[id];
    const box = document.createElement("div"),
      heading = document.createElement("h3"),
      progress = document.createElement("progress"),
      reset = document.createElement("div");
    box.className = `quota-box${item && item.remaining_percent < 20 ? " low" : ""}`;
    heading.textContent = label;
    if (big) {
      const value = document.createElement("div");
      value.className = "big";
      value.textContent = item ? `≈ ${item.remaining_percent}%` : "未知";
      box.append(heading, value);
    } else {
      box.append(heading);
      const sub = document.createElement("div");
      sub.className = "sub";
      sub.textContent = item ? `≈ ${item.remaining_percent}%` : "未知";
      box.appendChild(sub);
    }
    progress.max = 100;
    progress.value = item ? item.remaining_percent : 0;
    reset.className = "sub";
    reset.textContent = item ? `重置于 ${localTime(item.resets_at)}` : "等待上游额度数据";
    box.append(progress, reset);
    cards.appendChild(box);
  }
  $("#quota-status").textContent = !account
    ? "请先在配置页添加 OpenCode Go Key。"
    : account.error
      ? `${account.error}${stale ? "；显示的是上次成功数据。" : "；当前额度未知。"}`
      : "已连接 OpenCode · 共享套餐额度。";
  $("#quota-updated").textContent = account?.fetched_at
    ? `${stale ? "上次成功查询" : "更新时间"} ${localTime(account.fetched_at)} · 缓存 60 秒`
    : "";

  const query = $("#quota-model-search").value.trim().toLowerCase();
  const models = quotaData.models.filter((model) => model.id.toLowerCase().includes(query));
  $("#quota-model-count").textContent = `${models.length} / ${quotaData.models.length}`;
  $("#quota-catalog-status").textContent =
    `目录更新时间：${localTime(quotaData.catalog?.updated_at)}`;
  const host = $("#quota-models");
  host.replaceChildren();
  for (const model of models) {
    const row = document.createElement("tr");
    appendCell(row, model.id);
    appendCell(
      row,
      { chat: "Chat", responses: "Responses", anthropic: "Messages" }[model.protocol] ||
        model.protocol,
    );
    for (const [id] of windows) {
      const item = account?.windows?.[id];
      appendCell(row, item ? `≈ ${item.remaining_percent}%${stale ? "（历史）" : ""}` : "未知");
    }
    const cell = document.createElement("td"),
      button = document.createElement("button");
    button.className = "btn btn-sm";
    button.textContent = "复制";
    button.onclick = () => copyText(model.id);
    cell.appendChild(button);
    row.appendChild(cell);
    host.appendChild(row);
  }
  if (!models.length)
    emptyRow(host, 6, quotaData.models.length ? "没有匹配的模型" : "模型目录尚未就绪");
}

$("#quota-refresh").onclick = loadModelQuotas;
$("#quota-account").onchange = renderModelQuotas;
$("#quota-model-search").oninput = renderModelQuotas;

function renderModelRouting() {
  const host = $("#diagnostic-models");
  if (!host || !debugData) return;
  const query = $("#diagnostic-model-search").value.trim().toLowerCase();
  host.replaceChildren();
  (debugData.models || [])
    .filter((item) => !query || item.model.toLowerCase().includes(query))
    .forEach((item) => {
      const eligibility = item.anonymous_eligibility || {},
        row = document.createElement("tr"),
        plan = [...(item.anonymous ? ["anonymous"] : []), ...(item.key_tiers || [])];
      appendCell(row, item.model);
      appendCell(row, `${item.native_protocol} · ${item.protocol_source}`);
      appendCell(row, item.route_error || (plan.length ? plan.join(" → ") : "—"));
      appendCell(row, eligibility.allowed ? "允许" : "拒绝");
      appendCell(row, eligibility.source || "—");
      appendCell(
        row,
        `${eligibility.input_cost ?? "?"} / ${eligibility.output_cost ?? "?"}`,
        "mono",
      );
      host.appendChild(row);
    });
  if (!host.children.length) emptyRow(host, 6, "没有匹配模型");

  const facts = $("#metadata-facts");
  facts.replaceChildren();
  const metadata = debugData.metadata || {};
  for (const [label, value] of [
    ["就绪", metadata.ready ? "是" : "否"],
    ["模型数", formatTokens(metadata.models)],
    ["更新时间", metadata.updated_at ? new Date(metadata.updated_at).toLocaleString() : "尚未更新"],
    ["下次刷新", metadata.next_refresh ? new Date(metadata.next_refresh).toLocaleString() : "—"],
  ]) {
    const fact = document.createElement("dl");
    fact.className = "fact";
    const term = document.createElement("dt");
    const detail = document.createElement("dd");
    term.textContent = label;
    detail.textContent = value;
    fact.append(term, detail);
    facts.appendChild(fact);
  }
}

$("#diagnostic-model-search").addEventListener("input", renderModelRouting);

async function loadAvailability() {
  try {
    const data = await api("/api/models/availability");
    $("#availability-error").textContent = "";
    const host = $("#availability-models");
    host.replaceChildren();
    for (const item of data.models) {
      const row = document.createElement("tr");
      const date = (value) =>
        value && !value.startsWith("0001-") ? new Date(value).toLocaleString() : "待探测";
      appendCell(row, item.model);
      const state = document.createElement("td");
      state.appendChild(item.disabled ? statusCell("停用", "bad") : statusCell("启用", "ok"));
      row.appendChild(state);
      appendCell(row, [item.channel, item.reason].filter(Boolean).join(" / ") || "待探测");
      const control = document.createElement("td");
      control.appendChild(statusCell(item.manual ? "手动" : "自动", item.manual ? "warn" : "ok"));
      row.appendChild(control);
      appendCell(
        row,
        item.auto_effort?.effort
          ? `${item.auto_effort.effort}（${date(item.effort_checked_at)}）`
          : "未验证",
      );
      appendCell(row, date(item.checked_at));
      appendCell(row, item.manual ? "手动控制，不自动探测" : date(item.next_check));
      const cell = document.createElement("td"),
        button = document.createElement("button"),
        toggle = document.createElement("button");
      button.className = "btn btn-sm";
      button.textContent = "恢复";
      button.disabled = !item.disabled;
      toggle.className = "btn btn-sm";
      toggle.textContent = item.manual ? "切回自动" : "切为手动";
      const run = async (action) => {
        button.disabled = toggle.disabled = true;
        try {
          await action();
          await loadAvailability();
          await loadDebugModels();
        } catch (error) {
          $("#availability-error").textContent = error.message;
          button.disabled = !item.disabled;
          toggle.disabled = false;
        }
      };
      button.onclick = () =>
        run(() =>
          api("/api/models/restore", {
            method: "POST",
            body: JSON.stringify({ model: item.model }),
          }),
        );
      toggle.onclick = () =>
        run(() =>
          api("/api/models/manual", {
            method: "POST",
            body: JSON.stringify({ model: item.model, manual: !item.manual }),
          }),
        );
      cell.append(button, toggle);
      row.appendChild(cell);
      host.appendChild(row);
    }
    if (!data.models.length)
      $("#availability-error").textContent = "当前模型目录中没有符合免费资格的模型。";
  } catch (error) {
    $("#availability-error").textContent = error.message;
  }
}

$("#availability-refresh").onclick = loadAvailability;

/* ---------- API keys ---------- */

let createdKey = "",
  apiKeysLoading = false;

function clearCreatedKey() {
  createdKey = "";
  $("#api-key-env").textContent = "";
  $("#api-key-created").classList.add("hidden");
}

async function loadAPIKeys() {
  if (apiKeysLoading) return;
  apiKeysLoading = true;
  $("#api-key-refresh").disabled = true;
  try {
    const data = await api("/api/api-keys");
    if (csrf) renderAPIKeys(data.keys);
  } catch (error) {
    $("#api-key-status").textContent = `读取失败：${error.message}`;
  } finally {
    apiKeysLoading = false;
    $("#api-key-refresh").disabled = false;
  }
}

async function mutateAPIKey(id, method, body, button) {
  button.disabled = true;
  try {
    const data = await api(`/api/api-keys/${encodeURIComponent(id)}`, {
      method,
      ...(body ? { body: JSON.stringify(body) } : {}),
    });
    if (!csrf) return;
    renderAPIKeys(data.keys);
    await loadConfig();
    toast("已保存并立即生效");
    return true;
  } catch (error) {
    toast(error.message, true);
    return false;
  } finally {
    button.disabled = false;
  }
}

const ccswitchClients = [
  ["claude", "Claude Code"],
  ["codex", "Codex"],
  ["opencode", "OpenCode"],
];

// Model list for the import panel; a manual entry is allowed when it is empty.
// Always returns an object so callers never have to special-case an empty list.
//
// Include both channels so a configured Go pool does not hide big-pickle.
// Prefer big-pickle whenever it is listed; otherwise use the first option.
const importModelPreference = ["big-pickle"];

function importModelOptions() {
  const all = debugData?.models || [];
  const pool = all.filter((item) => item.available_go || item.available_zen || item.anonymous);
  if (!pool.length) return { preferred: "", list: [] };
  const names = pool.map((item) => item.model);
  const preferred = importModelPreference.find((name) => names.includes(name)) || names[0];
  return { preferred, list: names };
}

let ccswitchTarget = null;
let ccswitchCombo = null;
let ccswitchGeneration = 0;

function openCCSwitchModal(id, name) {
  closeModal(false);
  ccswitchGeneration++;
  ccswitchTarget = { id, name };
  $("#ccswitch-open").disabled = false;
  $("#ccswitch-copy").disabled = false;
  $("#ccswitch-target").textContent = `${name} · 导入地址 ${derivedAPIBase()}`;
  $("#ccswitch-status").textContent = "导入后需在 CC Switch 内手动启用该供应商。";

  const clients = $("#ccswitch-clients");
  clients.replaceChildren();
  for (const [value, text] of ccswitchClients) {
    const wrap = document.createElement("label");
    wrap.className = "choice";
    const radio = document.createElement("input");
    radio.type = "radio";
    radio.name = "ccs-client";
    radio.value = value;
    if (value === "claude") radio.checked = true;
    const span = document.createElement("span");
    span.textContent = text;
    wrap.append(radio, span);
    clients.appendChild(wrap);
  }

  const host = $("#ccswitch-model");
  host.replaceChildren();
  const models = importModelOptions();
  if (models.list.length) {
    ccswitchCombo = makeCombobox(host, models.preferred);
    ccswitchCombo.setOptions(models.list, models.preferred);
  } else {
    const manual = document.createElement("input");
    manual.id = "ccswitch-model-input";
    document.querySelector(
      'label[for="ccswitch-model"], label[for="ccswitch-model-input"]',
    ).htmlFor = manual.id;
    manual.placeholder = "输入模型 ID";
    host.appendChild(manual);
    ccswitchCombo = {
      get value() {
        return manual.value.trim();
      },
    };
  }

  openModal("#ccswitch-modal", "#ccswitch-clients input:checked");
}

function currentCCSwitchModel() {
  return ccswitchCombo?.value || "";
}

function currentCCSwitchClient() {
  return $("#ccswitch-clients input:checked")?.value || "claude";
}

async function runCCSwitchImport(action) {
  if (!ccswitchTarget) return;
  const generation = ccswitchGeneration;
  const target = ccswitchTarget;
  const app = currentCCSwitchClient();
  const base = derivedAPIBase();
  const model = currentCCSwitchModel();
  if (!model) {
    $("#ccswitch-status").textContent = "请先选择或填写模型。";
    return;
  }
  const open = $("#ccswitch-open"),
    copy = $("#ccswitch-copy");
  open.disabled = true;
  copy.disabled = true;
  $("#ccswitch-status").textContent = "生成中…";
  try {
    const data = await api(`/api/api-keys/${encodeURIComponent(target.id)}/ccswitch-import`, {
      method: "POST",
      body: JSON.stringify({ app, model, base }),
    });
    if (generation !== ccswitchGeneration || !csrf) return;
    if (action === "open") {
      window.location.assign(data.url);
      $("#ccswitch-status").textContent =
        "已交给系统；若没有弹出 CC Switch，说明未安装或协议未注册，可改用复制导入链接。";
    } else {
      const copied = await copyText(data.url, false);
      if (generation !== ccswitchGeneration || !csrf) return;
      $("#ccswitch-status").textContent = copied
        ? "导入链接已复制。"
        : "复制未获允许，可选中下方链接手动复制。";
      $("#ccswitch-fallback-field").classList.toggle("hidden", copied);
      $("#ccswitch-fallback").value = copied ? "" : data.url;
      if (!copied) {
        $("#ccswitch-fallback").focus();
        $("#ccswitch-fallback").select();
      }
    }
  } catch (error) {
    if (generation !== ccswitchGeneration || !csrf) return;
    $("#ccswitch-status").textContent = error.message || "网络失败，可稍后重试或手动复制环境变量。";
  } finally {
    if (generation === ccswitchGeneration) {
      open.disabled = false;
      copy.disabled = false;
    }
  }
}

$("#ccswitch-close").onclick = () => closeModal();
$("#ccswitch-open").onclick = () => runCCSwitchImport("open");
$("#ccswitch-copy").onclick = () => runCCSwitchImport("copy");

function renderAPIKeys(keys) {
  $("#api-key-count").textContent = `${keys.length} 个`;
  $("#api-key-status").textContent = `${keys.filter((key) => key.enabled).length} 个已启用`;
  const host = $("#api-key-list");
  host.replaceChildren();
  for (const key of keys) {
    const row = document.createElement("tr");
    row.dataset.keyId = key.id;
    const nameCell = document.createElement("td");
    nameCell.className = "key-name";
    nameCell.textContent = key.name;
    row.appendChild(nameCell);
    appendCell(row, key.display, "mono");
    const state = document.createElement("td");
    state.appendChild(key.enabled ? statusCell("启用", "ok") : statusCell("停用", "warn"));
    row.appendChild(state);
    const usage = key.usage || {},
      summary = (value = {}) =>
        `${formatTokens(value.requests)} 次 · ${formatTokens(value.total_tokens)} tokens`;
    appendCell(row, `${summary(usage.last_hour)} / ${summary(usage.lifetime)}`);
    appendCell(row, key.created_at ? localTime(key.created_at) : "已有配置");

    const cell = document.createElement("td");
    const actions = document.createElement("div");
    actions.className = "row-actions";
    for (const [label, action] of [
      [
        "重命名",
        () => {
          const editor = document.createElement("div");
          editor.className = "rename-editor";
          const input = document.createElement("input");
          input.value = key.name;
          input.maxLength = 80;
          input.setAttribute("aria-label", "密钥名称");
          const save = document.createElement("button");
          save.type = "button";
          save.className = "btn btn-sm";
          save.textContent = "保存";
          const cancel = document.createElement("button");
          cancel.type = "button";
          cancel.className = "btn btn-sm";
          cancel.textContent = "取消";
          const restore = () => {
            nameCell.textContent = key.name;
          };
          const submit = async () => {
            if (!input.value.trim()) {
              input.setCustomValidity("名称不能为空");
              input.reportValidity();
              return;
            }
            input.disabled = cancel.disabled = true;
            await mutateAPIKey(key.id, "PATCH", { name: input.value.trim() }, save);
            input.disabled = cancel.disabled = false;
          };
          save.onclick = submit;
          cancel.onclick = restore;
          input.oninput = () => input.setCustomValidity("");
          input.onkeydown = (event) => {
            if (event.key === "Escape") restore();
            if (event.key === "Enter") {
              event.preventDefault();
              submit();
            }
          };
          editor.append(input, save, cancel);
          nameCell.replaceChildren(editor);
          input.focus();
          input.select();
        },
      ],
      ["导入 CCS", () => openCCSwitchModal(key.id, key.name)],
      [
        key.enabled ? "停用" : "启用",
        (button) => mutateAPIKey(key.id, "PATCH", { enabled: !key.enabled }, button),
      ],
      [
        "删除",
        (button) => {
          if (confirm(`删除“${key.name}”后，使用它的应用将无法继续访问。确定删除？`))
            mutateAPIKey(key.id, "DELETE", null, button);
        },
      ],
    ]) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = label === "删除" ? "btn btn-sm btn-danger" : "btn btn-sm";
      button.textContent = label;
      button.onclick = () => action(button);
      actions.appendChild(button);
    }
    cell.append(actions);
    row.appendChild(cell);
    host.appendChild(row);
  }
  if (!keys.length) emptyRow(host, 5, "暂无 API Key");
}

$("#api-key-form").onsubmit = async (event) => {
  event.preventDefault();
  const button = $("#api-key-create");
  button.disabled = true;
  try {
    const data = await api("/api/api-keys", {
      method: "POST",
      body: JSON.stringify({ name: $("#api-key-name").value, value: $("#api-key-value").value }),
    });
    if (!csrf) return;
    createdKey = data.key;
    $("#api-key-env").textContent =
      `OPENAI_API_KEY=${data.key}\nOPENAI_BASE_URL=${derivedAPIBase()}/v1`;
    $("#api-key-created").classList.remove("hidden");
    $("#api-key-form").reset();
    renderAPIKeys(data.keys);
    await loadConfig();
    toast("API Key 已创建");
  } catch (error) {
    toast(error.message, true);
  } finally {
    button.disabled = false;
  }
};

$("#api-key-refresh").onclick = loadAPIKeys;
$("#api-key-dismiss").onclick = clearCreatedKey;
$("#api-key-copy").onclick = () => copyText(createdKey);
$("#api-key-copy-env").onclick = () => copyText($("#api-key-env").textContent);

/* ---------- rotation ---------- */

let rotationData = null,
  rotationDirty = false;

function mergeRotation(value) {
  if (rotationDirty && rotationData) {
    value.first_output_seconds = rotationData.first_output_seconds;
    for (const group of value.groups) {
      const draft = rotationData.groups.find((item) => item.id === group.id);
      if (draft) {
        group.alias = draft.alias;
        group.order = [
          ...draft.order,
          ...group.order.filter((model) => !draft.order.includes(model)),
        ];
      }
    }
  }
  rotationData = value;
  renderRotation();
}

function markRotationDirty() {
  rotationDirty = true;
  $("#rotation-draft").classList.remove("hidden");
}
$("#rotation-timeout").oninput = (event) => {
  if (!rotationData) return;
  rotationData.first_output_seconds = Number(event.target.value);
  markRotationDirty();
};
const rotationReasons = {
  success: "成功",
  client_canceled: "调用方取消",
  quota_or_rate_limit: "额度不足 / 限流",
  transport_error: "连接错误",
  first_output_timeout: "首个输出超时",
  stream_error: "输出中断",
  response_read_error: "读取响应失败",
  response_conversion_error: "响应解析或转换失败",
  request_conversion_error: "请求转换失败",
  upstream_error: "上游错误",
};

async function loadRotation() {
  try {
    const value = await api("/api/rotation");
    if (!csrf) return;
    mergeRotation(value);
  } catch (error) {
    toast(error.message, true);
  }
}

function renderRotation() {
  const state = rotationData;
  $("#rotation-draft").classList.toggle("hidden", !rotationDirty);
  $("#rotation-status").textContent =
    state.persistence_error || "两组独立轮转，所有调用方共享当前位置。";
  $("#rotation-timeout").value = state.first_output_seconds;
  const host = $("#rotation-groups");
  host.replaceChildren();
  for (const group of state.groups) {
    const card = document.createElement("div");
    card.className = "card";
    const title = document.createElement("div");
    title.className = "card-head";
    const heading = document.createElement("h2");
    heading.textContent = group.id === "sota" ? "SOTA" : "甜点";
    title.appendChild(heading);

    const current = document.createElement("div");
    current.className = "card-note";
    current.textContent = `当前模型：${group.current || "等待可用模型"}`;
    const stateCell = document.createElement("div");
    stateCell.className = "card-note";
    const paused = Object.entries(group.paused || {}).filter(
      ([, until]) => until && new Date(until) > new Date(),
    );
    stateCell.textContent = paused.length ? `暂停：${paused.length} 个模型` : "无暂停";

    const field = document.createElement("div");
    field.className = "field";
    const label = document.createElement("label");
    label.textContent = "调用别名";
    label.htmlFor = `rotation-alias-${group.id}`;
    const input = document.createElement("input");
    input.id = label.htmlFor;
    input.value = group.alias;
    input.maxLength = 64;
    input.required = true;
    input.oninput = () => {
      group.alias = input.value;
      markRotationDirty();
    };
    field.append(label, input);

    const table = document.createElement("table");
    table.className = "table";
    const tbody = document.createElement("tbody");
    group.order.forEach((model, index) => {
      const row = document.createElement("tr");
      appendCell(row, String(index + 1));
      appendCell(row, model, "mono");
      const until = group.paused[model],
        isPaused = until && new Date(until) > new Date();
      const status = document.createElement("td");
      status.appendChild(
        !group.available[model]
          ? statusCell("不可用", "idle")
          : isPaused
            ? statusCell("暂停", "warn")
            : model === group.current
              ? statusCell("使用中", "ok")
              : statusCell("可用", "idle"),
      );
      row.appendChild(status);
      const actions = document.createElement("td");
      actions.className = "row-actions";
      for (const [text, delta] of [
        ["上移", -1],
        ["下移", 1],
      ]) {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "btn btn-sm";
        button.textContent = text;
        button.disabled = index + delta < 0 || index + delta >= group.order.length;
        button.onclick = () => {
          [group.order[index], group.order[index + delta]] = [
            group.order[index + delta],
            group.order[index],
          ];
          markRotationDirty();
          renderRotation();
        };
        actions.appendChild(button);
      }
      const pick = document.createElement("button");
      pick.type = "button";
      pick.className = "btn btn-sm";
      pick.textContent = "设为当前";
      pick.disabled = !group.available[model];
      pick.onclick = async () => {
        pick.disabled = true;
        try {
          const result = await api("/api/rotation/current", {
            method: "POST",
            body: JSON.stringify({ group: group.id, model }),
          });
          if (!csrf) return;
          mergeRotation(result);
          toast("当前模型已切换");
        } catch (error) {
          pick.disabled = false;
          toast(error.message, true);
        }
      };
      actions.appendChild(pick);
      row.appendChild(actions);
      tbody.appendChild(row);
    });
    if (!group.order.length) emptyRow(tbody, 4, "等待 Go 模型目录");
    const wrap = document.createElement("div");
    wrap.className = "table-wrap advance";
    table.appendChild(tbody);
    wrap.appendChild(table);

    const recent = document.createElement("div");
    recent.className = "card-note";
    const switches = (group.switches || []).slice(-3).reverse();
    recent.textContent = switches.length
      ? `最近切换：${switches
          .map((item) => `${localTime(item.time)} ${item.from} → ${item.to}`)
          .join("；")}`
      : "尚无切换记录";

    card.append(title, current, stateCell, field, wrap, recent);
    host.appendChild(card);
  }

  const attempts = $("#rotation-attempts");
  attempts.replaceChildren();
  for (const item of (state.attempts || []).slice().reverse()) {
    const row = document.createElement("tr");
    for (const value of [
      localTime(item.time),
      item.request,
      item.alias,
      item.model,
      String(item.number),
      rotationReasons[item.outcome] || item.outcome,
    ])
      appendCell(row, value);
    attempts.appendChild(row);
  }
  if (!state.attempts?.length) emptyRow(attempts, 6, "调用分组别名后显示最近 100 次尝试");
}

$("#rotation-refresh").onclick = loadRotation;
$("#rotation-form").onsubmit = async (event) => {
  event.preventDefault();
  if (!rotationData) return;
  const button = $("#rotation-save");
  const controls = [...$("#rotation-form").querySelectorAll("input, button")].map((node) => [
    node,
    node.disabled,
  ]);
  for (const [node] of controls) node.disabled = true;
  try {
    const body = { first_output_seconds: Number($("#rotation-timeout").value) };
    for (const group of rotationData.groups)
      body[group.id] = { alias: group.alias.trim(), order: group.order };
    const value = await api("/api/rotation", { method: "PUT", body: JSON.stringify(body) });
    if (!csrf) return;
    rotationDirty = false;
    rotationData = value;
    renderRotation();
    toast("模型轮转配置已保存并生效");
  } catch (error) {
    toast(error.message, true);
  } finally {
    for (const [node, disabled] of controls) node.disabled = disabled;
  }
};

/* ---------- logs ---------- */

function connectLogs() {
  if (eventSource) eventSource.close();
  eventSource = new EventSource("/api/logs/stream");
  eventSource.addEventListener("log", (event) => {
    logEvents.push(JSON.parse(event.data));
    if (logEvents.length > 2000) logEvents.shift();
    if (!paused && !logRenderPending) {
      logRenderPending = true;
      requestAnimationFrame(() => {
        logRenderPending = false;
        renderLogs();
      });
    }
  });
  eventSource.addEventListener("gap", () => toast("较早日志已从内存清除", true));
}

function renderLogs() {
  const level = $("#log-level").value,
    component = $("#log-component").value.toLowerCase(),
    query = $("#log-search").value.toLowerCase(),
    rows = logEvents
      .filter(
        (item) =>
          (!level || item.level === level) &&
          (!component || (item.component || "").toLowerCase().includes(component)) &&
          (!query || JSON.stringify(item).toLowerCase().includes(query)),
      )
      .slice(-500),
    host = $("#log-list");
  host.replaceChildren();
  for (const item of rows) {
    const row = document.createElement("div");
    row.className = "log";
    const time = document.createElement("span"),
      levelNode = document.createElement("span"),
      componentNode = document.createElement("span"),
      fields = document.createElement("span");
    time.className = "log-time";
    time.textContent = new Date(item.time).toLocaleTimeString();
    levelNode.className = `log-${
      item.level === "error" ? "bad" : item.level === "warn" ? "warn" : "ok"
    }`;
    levelNode.textContent = item.level;
    componentNode.className = "log-component";
    componentNode.textContent = item.component || "core";
    fields.className = "log-fields";
    fields.textContent = JSON.stringify(item.fields || {});
    row.append(time, levelNode, componentNode, fields);
    host.appendChild(row);
  }
  if (!rows.length) {
    const empty = document.createElement("div");
    empty.className = "empty";
    empty.textContent = "等待日志事件…";
    host.appendChild(empty);
  }
  host.scrollTop = host.scrollHeight;
}

["#log-level", "#log-component", "#log-search"].forEach((id) =>
  $(id).addEventListener("input", renderLogs),
);
$("#log-pause").onclick = () => {
  paused = !paused;
  $("#log-pause").textContent = paused ? "继续" : "暂停";
  if (!paused) renderLogs();
};

boot();
