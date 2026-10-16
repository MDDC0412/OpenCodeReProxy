// Playwright CLI evaluates this file as a function expression.
// prettier-ignore
async (page) => {
  const baseURL = "http://127.0.0.1:19090";
  const gatewayURL = "http://127.0.0.1:19091";
  const fixtureURL = "http://127.0.0.1:19092";
  await page.unrouteAll({ behavior: "wait" });
  const checks = [];
  const errors = [];
  let expectedMonitorFailure = false;
  const assert = (value, message) => {
    if (!value) throw new Error(message);
  };
  const check = async (name, action) => {
    try {
      await action();
    } catch (error) {
      await page.screenshot({ path: "output/playwright/failure.png", fullPage: true });
      throw new Error(`${name}: ${error.message}`);
    }
    checks.push(name);
    console.log(`PASS ${name}`);
  };
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("console", (message) => {
    const expected = message.text().includes("401 (Unauthorized)") && message.location().url.endsWith("/api/auth/session")
      || expectedMonitorFailure && message.location().url.endsWith("/api/monitor") && message.text().includes("503 (Service Unavailable)");
    if (message.type() === "error" && !expected)
      errors.push(message.text());
  });
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(`${baseURL}/`);
  if (await page.locator("#login").isVisible()) {
    await page.getByRole("textbox", { name: "账号", exact: true }).fill("admin");
    await page.getByRole("textbox", { name: "密码", exact: true }).fill("browser-fixture-password-only");
    await page.getByRole("button", { name: "登录", exact: true }).click();
  }
  await page.locator("#app").waitFor({ state: "visible" });
  await page.waitForFunction(() => document.querySelector("#guide-summary").textContent.includes("通过"));
  const csrf = (await (await page.request.get(`${baseURL}/api/auth/session`)).json())
    .csrf_token;
  const api = async (path, method = "GET", data) => {
    const response = await page.request.fetch(`${baseURL}${path}`, {
      method,
      data,
      headers: { "X-CSRF-Token": csrf },
    });
    const value = await response.json();
    assert(response.ok(), `${method} ${path}: ${JSON.stringify(value)}`);
    return value;
  };
  const navigate = async (name) => {
    await page.locator(".nav").getByRole("button", { name, exact: true }).click();
  };
  const visible = async (selector) => page.locator(selector).waitFor({ state: "visible" });
  const text = (selector) => page.locator(selector).textContent();
  const config = await api("/api/config");
  assert(
    config.upstream.zen === `${fixtureURL}/zen` &&
      config.upstream.go === `${fixtureURL}/go` &&
      config.upstream.anonymous === `${fixtureURL}/anonymous`,
    "Run browser regression only against the local fixture configuration",
  );
  for (const key of (await api("/api/api-keys")).keys) {
    if (key.name.startsWith("浏览器回归")) await api(`/api/api-keys/${key.id}`, "DELETE");
  }

  await check("navigation and checklist", async () => {
    assert((await page.locator(".nav button").count()) === 9, "nine navigation entries");
    assert(
      (await page.locator(".nav button").nth(1).textContent()).trim() === "请求台",
      "playground should be second",
    );
    await page.locator("#guide-box").click();
    await visible("#guide-items");
    await page.evaluate(() => document.querySelector("#toast").classList.add("hidden"));
    assert((await page.locator("#guide-items li").count()) === 6, "six shared status checks");
    await page.locator("#guide-box").click();
  });

  await check("zero requests and visible overview details", async () => {
    await page.route("**/api/monitor", async (route) => {
      const value = await (await route.fetch()).json();
      value.metrics.last_hour.total = 0;
      value.metrics.last_hour.success_rate = 0;
      await route.fulfill({ json: value });
    });
    await page.locator("#refresh-now").click();
    await page.waitForFunction(() => document.querySelector("#m-rate").textContent === "暂无请求");
    await page.waitForFunction(() => !document.querySelector("#attention-list").textContent.includes("成功率偏低"));
    assert((await page.locator("#m-rate").getAttribute("class")).includes("is-idle"), "idle is not a success-rate failure");
    assert((await text("#health-text")).includes("暂无请求"), "idle health does not claim inference success");
    assert((await page.locator('#guide-items [data-check="requests"]').getAttribute("data-tone")) === "idle", "request check is neutral without traffic");
    assert(await page.locator("#upstream-requests").isVisible(), "recent requests visible");
    assert(await page.locator("#usage-models").isVisible(), "model usage visible");
    await page.unroute("**/api/monitor");
  });

  await check("overview warnings share checklist status and show failed requests", async () => {
    const originalTheme = await page.evaluate(() => window.o2apiTheme.get());
    await page.route("**/api/monitor", async (route) => {
      const value = await (await route.fetch()).json();
      value.metrics.last_hour = { ...value.metrics.last_hour, total: 20, success: 11, errors: 9, success_rate: 0.55 };
      value.upstream.requests = [
        { time: new Date().toISOString(), request_id: "req-browser-success", model: "big-pickle", channel: "anonymous", status: 200, success: true, duration_ms: 2 },
        { time: new Date().toISOString(), request_id: "req-browser-failed", model: "big-pickle", channel: "anonymous", status: 403, success: false, duration_ms: 3 },
      ];
      await route.fulfill({ json: value });
    });
    await page.locator("#refresh-now").click();
    await page.waitForFunction(() => document.querySelector("#health-text").textContent.includes("55.0%"));
    assert((await text("#guide-summary")) === "状态检查 1 项待处理", "summary agrees with request warning");
    assert((await page.locator('#guide-items li[data-tone="warn"]').count()) === 1, "one warning in checklist");
    assert((await page.locator('#guide-items [data-check="requests"]').getAttribute("data-tone")) === "warn", "request success rate is the failed check");
    assert((await text("#attention-list")).includes("9 次失败"), "warning includes actual failure count");
    await page.locator("#guide-box").click();
    await visible("#guide-items");
    for (const width of [1440, 1024, 390, 320]) {
      await page.setViewportSize({ width, height: 900 });
      for (const mode of ["light", "dark"]) {
        await page.evaluate((mode) => applyTheme({ mode, accent: "#0d9488" }), mode);
        await page.evaluate(() => Promise.all(document.getAnimations().map((animation) => animation.finished.catch(() => {}))));
        const framed = await page.locator("#guide-items li").evaluateAll((items) => items.every((item) => {
          const style = getComputedStyle(item), box = item.getBoundingClientRect(),
            name = item.querySelector(".checklist-label").getBoundingClientRect(),
            status = item.querySelector(".status").getBoundingClientRect(),
            detail = item.querySelector(".checklist-detail").getBoundingClientRect();
          return style.borderTopStyle === "solid" && parseFloat(style.borderTopWidth) >= 1
            && name.left >= box.left && name.right <= status.left
            && status.right <= box.right && detail.top >= name.bottom;
        }));
        assert(framed, `labels and dots stay inside their own borders at ${width}px ${mode}`);
        assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `overview status does not overflow at ${width}px ${mode}`);
        if (width === 1440 || width === 390) {
          const top = await page.locator(".status-bar").boundingBox(), bottom = await page.locator("#attention").boundingBox();
          await page.screenshot({ path: `output/playwright/overview-status-${mode}-${width}.png`, clip: { x: top.x, y: top.y, width: top.width, height: bottom.y + bottom.height - top.y } });
        }
      }
    }
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.locator("#attention-list").getByRole("button", { name: "查看失败请求", exact: true }).click();
    assert(await page.locator("#requests-failed-only").isChecked(), "action enables failed request filter");
    assert((await text("#upstream-requests")).includes("req-browser-failed"), "failed request is shown");
    assert(!(await text("#upstream-requests")).includes("req-browser-success"), "successful requests do not obscure failures");
    await page.locator("#requests-failed-only").uncheck();
    assert((await text("#upstream-requests")).includes("req-browser-success"), "filter can be turned off");
    await page.locator("#guide-box").click();
    await page.evaluate((theme) => applyTheme(theme), originalTheme);
    await page.unroute("**/api/monitor");
    await page.locator("#refresh-now").click();
    await page.waitForFunction(() => document.querySelector("#attention-list").textContent === "");
    assert(!(await text("#guide-summary")).includes("待处理"), "summary recovers with banner");
  });

  await check("monitor error and stale resource alerts recover", async () => {
    expectedMonitorFailure = true;
    await page.route("**/api/monitor", (route) => route.fulfill({ status: 503, json: { error: { message: "浏览器回归监控失败" } } }));
    await page.evaluate(() => document.querySelector("#toast").classList.add("hidden"));
    await page.locator("#refresh-now").click();
    await page.waitForFunction(() => document.querySelector("#monitor-error")?.textContent.includes("浏览器回归监控失败"));
    assert(await page.locator("#monitor-error").isVisible(), "refresh failure is explicit");
    assert((await text("#guide-summary")).includes("待处理"), "stale monitor data cannot claim all checks passed");
    assert(await page.locator("#toast").isHidden(), "failed refresh does not claim success");
    await page.unroute("**/api/monitor");
    expectedMonitorFailure = false;
    await page.route("**/api/monitor", async (route) => {
      const value = await (await route.fetch()).json();
      value.resources.models.stale = true;
      value.resources.proxies[0].healthy = false;
      await route.fulfill({ json: value });
    });
    await page.locator("#refresh-now").click();
    await page.waitForFunction(() => document.querySelector("#attention-list").textContent.includes("目录已过期"));
    assert((await text("#attention-list")).includes("代理节点不可用"), "unhealthy proxy is shown");
    assert((await text("#guide-summary")) === "状态检查 2 项待处理", "proxy and stale catalog checks agree with banner");
    assert(await page.locator("#monitor-error").isHidden(), "successful refresh clears error");
    await page.unroute("**/api/monitor");
    await page.locator("#refresh-now").click();
    await page.waitForFunction(() => document.querySelector("#attention-list").textContent === "");
  });

  const catalog = await api("/api/debug/models");
  await check("playground search keyboard and mouse", async () => {
    await navigate("请求台");
    const input = page.locator("#play-model-input");
    await input.click();
    assert(
      (await page.locator("#play-model-options [role=option]").count()) > 1,
      "focus shows whole catalog",
    );
    await input.fill("kimi");
    await input.press("Enter");
    assert(
      JSON.parse(await page.locator("#play-request").inputValue()).model === "kimi-k3",
      "selected model updates JSON",
    );
    await input.click();
    await input.fill("big");
    await page.getByRole("option", { name: "big-pickle", exact: true }).click();
    assert(
      JSON.parse(await page.locator("#play-request").inputValue()).model === "big-pickle",
      "mouse selection updates JSON",
    );
    await input.click();
    await input.fill("no-matching-model");
    await input.press("Escape");
    assert((await input.inputValue()) === "big-pickle", "Escape restores selected model");
    await page.locator("#play-run").click();
    await page.waitForFunction(() =>
      document.querySelector("#play-output").textContent.includes("本地模拟上游响应"),
    );
  });

  await check("live monitor quota and routing fields match UI", async () => {
    const monitor = await api("/api/monitor");
    assert(monitor.usage.last_hour.tokens.total_tokens >= 25, "fixture usage recorded");
    assert(monitor.upstream.requests.some((item) => item.model === "big-pickle" && item.channel && item.success), "upstream route recorded");
    await navigate("总览");
    await page.locator("#refresh-now").click();
    await page.waitForFunction(() => Number(document.querySelector("#u-total").textContent.replace(/,/g, "")) >= 25);
    assert((await text("#usage-models")).includes("big-pickle"), "model tokens rendered");
    assert((await text("#upstream-requests")).includes("big-pickle"), "recent request rendered");
    const quota = await api("/api/model-quotas");
    await navigate("模型与额度");
    await page.waitForFunction(() => document.querySelector("#quota-status").textContent.includes("已连接"));
    for (const item of Object.values(quota.accounts[0].windows)) assert((await text("#quota-cards")).includes(`${item.remaining_percent}%`), "shared quota percentage rendered");
    await page.locator("#diagnostic-models").locator("xpath=ancestor::details").locator("summary").click();
    assert((await text("#diagnostic-models")).includes(`${catalog.models[0].native_protocol} · ${catalog.models[0].protocol_source}`), "routing fields rendered from API");
    assert(!(await text("#page-models")).includes("undefined"), "no guessed undefined fields");
  });

  await check("quota without Go credentials and stale data", async () => {
    const quota = await api("/api/model-quotas");
    await page.route("**/api/model-quotas", (route) => route.fulfill({ json: { ...quota, accounts: [] } }));
    await page.locator("#quota-refresh").click();
    await page.waitForFunction(() => document.querySelector("#quota-status").textContent.includes("请先"));
    assert((await text("#quota-cards")).includes("未知"), "missing credentials show unknown quota");
    await page.unroute("**/api/model-quotas");
    await page.route("**/api/model-quotas", (route) => route.fulfill({ json: { ...quota, accounts: quota.accounts.map((item) => ({ ...item, status: "stale", error: "浏览器回归额度查询失败" })) } }));
    await page.locator("#quota-refresh").click();
    await page.waitForFunction(() => document.querySelector("#quota-status").textContent.includes("上次成功数据"));
    assert((await text("#quota-models")).includes("（历史）"), "stale percentages labelled");
    await page.unroute("**/api/model-quotas");
  });

  let created;
  await check("API key create rename disable enable", async () => {
    await navigate("API Key");
    await page.locator("#key-form-box summary").click();
    await page.locator("#api-key-name").fill("浏览器回归密钥");
    const createdResponse = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/api-keys") && response.request().method() === "POST",
    );
    await page.locator("#api-key-create").click();
    created = await (await createdResponse).json();
    assert(created.key && created.id, "create returns plaintext once");
    await page.locator("#api-key-created").waitFor({ state: "visible" });
    await page.evaluate(() => {
      window.browserKeyCopies = [];
      navigator.clipboard.writeText = async (value) => window.browserKeyCopies.push(value);
    });
    await page.locator("#api-key-copy").click();
    assert((await page.evaluate(() => window.browserKeyCopies.at(-1))) === created.key, "copy key uses new plaintext key");
    await page.locator("#api-key-copy-env").click();
    const env = await page.evaluate(() => window.browserKeyCopies.at(-1));
    assert(env.includes(`OPENAI_API_KEY=${created.key}`) && env.includes(`OPENAI_BASE_URL=${gatewayURL}/v1`), "copy environment uses key and API endpoint");
    await page.evaluate(() => { navigator.clipboard.writeText = async () => { throw new Error("denied"); }; });
    await page.locator("#api-key-copy").click();
    await page.locator("#copy-modal").waitFor({ state: "visible" });
    assert((await page.locator("#copy-value").inputValue()) === created.key, "copy denial offers selectable key");
    await page.locator("#copy-value").press("Escape");
    assert((await page.locator("#copy-value").inputValue()) === "", "closing fallback clears key");
    let row = page.locator(`#api-key-list tr[data-key-id="${created.id}"]`);
    await row.getByRole("button", { name: "重命名" }).click();
    await row.getByRole("textbox", { name: "密钥名称" }).fill("浏览器回归已重命名");
    await row.getByRole("button", { name: "保存", exact: true }).click();
    await page.waitForFunction(() => document.querySelector("#api-key-list").textContent.includes("浏览器回归已重命名"));
    await row.waitFor();
    await row.getByRole("button", { name: "停用", exact: true }).click();
    await row.getByRole("button", { name: "启用", exact: true }).waitFor();
    const denied = await page.request.get(`${gatewayURL}/v1/models`, {
      headers: { Authorization: `Bearer ${created.key}` },
    });
    assert(denied.status() === 401, "disabled key rejected immediately");
    await row.getByRole("button", { name: "启用", exact: true }).click();
    await row.getByRole("button", { name: "停用", exact: true }).waitFor();
    const allowed = await page.request.get(`${gatewayURL}/v1/models`, {
      headers: { Authorization: `Bearer ${created.key}` },
    });
    assert(allowed.ok(), "enabled key accepted");
  });

  const importRow = () => page.locator(`#api-key-list tr[data-key-id="${created.id}"]`);
  const openImport = async () => {
    await importRow().getByRole("button", { name: "导入 CCS" }).click();
    await visible("#ccswitch-modal");
  };
  await check("CC Switch default focus keyboard clipboard and endpoints", async () => {
    const height = (await importRow().boundingBox()).height;
    await openImport();
    assert(
      (await page.locator("#ccswitch-model-input").inputValue()) === "big-pickle",
      "big-pickle preferred over Go models",
    );
    assert((await importRow().boundingBox()).height === height, "modal does not resize table row");
    assert(await page.locator("#app").evaluate((node) => node.inert), "background cannot receive focus");
    await page.locator("#ccswitch-open").press("Tab");
    assert(await page.locator("#ccswitch-close").evaluate((node) => document.activeElement === node), "Tab wraps within modal");
    await page.locator("#ccswitch-close").press("Shift+Tab");
    assert(await page.locator("#ccswitch-open").evaluate((node) => document.activeElement === node), "Shift+Tab wraps within modal");
    await page.locator("#ccswitch-model-input").click();
    assert(
      (await page.locator("#ccswitch-model-options [role=option]").count()) > 1,
      "all options visible on focus",
    );
    const rect = await page.locator("#ccswitch-model-options").boundingBox();
    const hit = await page.evaluate(
      ({ x, y }) => document.elementFromPoint(x, y)?.closest(".combo-list")?.id,
      { x: rect.x + 20, y: rect.y + 20 },
    );
    assert(hit === "ccswitch-model-options", "popup is not clipped");
    await page.locator("#ccswitch-model-input").press("Escape");
    await page.evaluate(() => {
      window.browserCopies = [];
      navigator.clipboard.writeText = async (value) => window.browserCopies.push(value);
    });
    for (const [app, label] of [
      ["claude", "Claude Code"],
      ["codex", "Codex"],
      ["opencode", "OpenCode"],
    ]) {
      await page.getByRole("radio", { name: label, exact: true }).check();
      await page.locator("#ccswitch-copy").click();
      await page.waitForFunction(
        () => document.querySelector("#ccswitch-status").textContent === "导入链接已复制。",
      );
      const url = new URL(await page.evaluate(() => window.browserCopies.at(-1)));
      assert(url.searchParams.get("app") === app, "app matches radio");
      assert(url.searchParams.get("model") === "big-pickle", "model preserved");
      assert(
        url.searchParams.get("endpoint") ===
          `${gatewayURL}${app === "claude" ? "" : "/v1"}`,
        "endpoint suffix matches client",
      );
    }
    await page.evaluate(() => {
      navigator.clipboard.writeText = async () => {
        throw new Error("denied");
      };
    });
    await page.locator("#ccswitch-copy").click();
    await visible("#ccswitch-fallback");
    assert(
      (await text("#ccswitch-status")).includes("复制未获允许"),
      "clipboard denial is not success",
    );
    await page.locator("#ccswitch-fallback").press("Escape");
    assert(await page.locator("#ccswitch-modal").isHidden(), "Escape closes modal");
    assert(
      await importRow()
        .getByRole("button", { name: "导入 CCS" })
        .evaluate((node) => document.activeElement === node),
      "focus restored",
    );
  });

  await check("CC Switch empty catalog and stale async reply", async () => {
    await page.route("**/api/debug/models", async (route) => {
      await route.fulfill({ json: { ...catalog, models: [] } });
    });
    await page.locator("#refresh-now").click();
    await page.waitForFunction(() => debugData.models.length === 0);
    await openImport();
    await page.locator("#ccswitch-model-input").fill("big-pickle");
    let release;
    const gate = new Promise((resolve) => {
      release = resolve;
    });
    await page.route("**/ccswitch-import", async (route) => {
      await gate;
      await route.fulfill({ json: { url: "ccswitch://v1/import?model=stale" } });
    });
    await page.locator("#ccswitch-copy").click();
    await page.locator("#ccswitch-close").click();
    await openImport();
    const staleResponse = page.waitForResponse((response) => response.url().endsWith("/ccswitch-import"));
    release();
    await (await staleResponse).finished();
    await page.unroute("**/ccswitch-import");
    await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    assert(
      (await text("#ccswitch-status")).includes("手动启用"),
      "old response cannot modify reopened modal",
    );
    await page.locator("#ccswitch-close").click();
    await page.unroute("**/api/debug/models");
    await page.locator("#refresh-now").click();
    await page.waitForFunction(() => debugData.models.length > 0);
  });

  await check("rotation draft survives reorder refresh manual switch and saves", async () => {
    const original = await api("/api/rotation");
    await navigate("模型轮转");
    const alias = page.locator("#rotation-alias-sota");
    await alias.fill("sota-browser-check");
    await page.locator("#rotation-timeout").fill("42");
    await page
      .locator("#rotation-groups .card")
      .first()
      .getByRole("button", { name: "下移", exact: true })
      .first()
      .click();
    assert((await alias.inputValue()) === "sota-browser-check", "reorder keeps alias");
    assert(
      (await page.locator("#rotation-timeout").inputValue()) === "42",
      "reorder keeps timeout",
    );
    await page.locator("#rotation-refresh").click();
    assert((await alias.inputValue()) === "sota-browser-check", "refresh keeps draft");
    await page
      .locator("#rotation-groups .card")
      .first()
      .getByRole("button", { name: "设为当前" })
      .first()
      .click();
    assert((await alias.inputValue()) === "sota-browser-check", "manual switch keeps draft");
    const savedResponse = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/rotation") && response.request().method() === "PUT",
    );
    await page.locator("#rotation-save").click();
    assert((await savedResponse).ok(), "rotation save succeeds");
    const saved = await api("/api/rotation");
    assert(saved.groups[0].alias === "sota-browser-check", "alias persisted");
    assert(saved.first_output_seconds === 42, "timeout persisted");
    assert(saved.groups[0].order[0] === original.groups[0].order[1], "order persisted");
    await api("/api/rotation", "PUT", {
      first_output_seconds: original.first_output_seconds,
      ...Object.fromEntries(
        original.groups.map((group) => [group.id, { alias: group.alias, order: group.order }]),
      ),
    });
  });

  await check("theme presets extremes persistence and blocked storage", async () => {
    await page.locator("#theme-color").click();
    await page.getByRole("radio", { name: "黑夜", exact: true }).check();
    assert(await page.evaluate(() => window.o2apiTheme.get().mode === "dark"), "theme modal changes mode");
    await page.locator("#theme-swatches button[title='#7c3aed']").click();
    assert(await page.evaluate(() => window.o2apiTheme.get().accent === "#7c3aed"), "preset changes palette");
    assert(await page.locator("#theme-swatches button[title='#7c3aed']").evaluate((node) => document.activeElement === node), "preset retains keyboard focus");
    await page.locator("#theme-custom").fill("#ea580c");
    assert(await page.evaluate(() => window.o2apiTheme.get().accent === "#ea580c"), "color picker changes palette");
    await page.getByRole("radio", { name: "白天", exact: true }).check();
    await page.locator("#theme-hex").fill("#2563eb");
    await page.locator("#theme-hex").press("Tab");
    assert((await page.locator("#theme-custom").inputValue()) === "#2563eb", "hex input synchronizes color picker");
    await page.locator("#theme-close").click();
    await page.reload();
    await page.locator("#app").waitFor({ state: "visible" });
    assert(
      (await page.evaluate(() => window.o2apiTheme.get().accent)) === "#2563eb",
      "theme survives reload",
    );
    await navigate("总览");
    await page.screenshot({ path: "output/playwright/overview-light-1440.png", fullPage: true });
    const paletteResults = await page.evaluate(() => {
      const luminance = (hex) => {
        const values = [1, 3, 5]
          .map((i) => parseInt(hex.slice(i, i + 2), 16) / 255)
          .map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
        return values[0] * 0.2126 + values[1] * 0.7152 + values[2] * 0.0722;
      };
      const ratio = (a, b) => {
        const values = [luminance(a), luminance(b)].sort((x, y) => y - x);
        return (values[0] + 0.05) / (values[1] + 0.05);
      };
      const failures = [];
      const toHex = (rgb) => "#" + rgb.match(/\d+/g).slice(0, 3).map((value) => Number(value).toString(16).padStart(2, "0")).join("");
      for (const mode of ["light", "dark"])
        for (const accent of ["#000000", "#ffffff", "#fffde7", "#2563eb", "#7c3aed", "#0d9488"]) {
          applyTheme({ mode, accent });
          const style = getComputedStyle(document.documentElement);
          const get = (name) => style.getPropertyValue(`--${name}`).trim();
          const button = getComputedStyle(document.querySelector("#api-key-create"));
          const selected = getComputedStyle(document.querySelector(".nav button.active"));
          if (
            ratio(accent, get("accent-on")) < 4.5 ||
            ratio(get("accent-ink"), get("accent-soft")) < 4.5 ||
            ratio(get("accent-hover"), get("accent-hover-on")) < 4.5 ||
            ratio(toHex(button.color), toHex(button.backgroundColor)) < 4.5 ||
            ratio(toHex(selected.color), toHex(selected.backgroundColor)) < 4.5
          )
            failures.push(`${mode} ${accent}`);
        }
      applyTheme({ mode: "light", accent: "#2563eb" });
      return failures;
    });
    assert(paletteResults.length === 0, `contrast failures: ${paletteResults.join(", ")}`);
    await page.addInitScript(() => {
      Storage.prototype.getItem = function () {
        throw new Error("blocked");
      };
      Storage.prototype.setItem = function () {
        throw new Error("blocked");
      };
    });
    await page.reload();
    await page.locator("#app").waitFor({ state: "visible" });
    await page.evaluate(() => applyTheme({ mode: "light", accent: "#7c3aed" }));
    await page.locator("#theme-toggle").click();
    assert(
      (await page.evaluate(() => window.o2apiTheme.get().accent)) === "#7c3aed",
      "blocked storage keeps in-memory accent",
    );
    assert(
      (await page.evaluate(() => window.o2apiTheme.get().mode)) === "dark",
      "mode toggle works without storage",
    );
  });

  await check("long key and model names stay inside mobile viewport", async () => {
    await navigate("API Key");
    const name = "浏览器回归已重命名" + "长名称".repeat(20);
    await importRow().getByRole("button", { name: "重命名" }).click();
    await importRow().getByRole("textbox", { name: "密钥名称" }).fill(name);
    await importRow().getByRole("button", { name: "保存", exact: true }).click();
    await page.waitForFunction((name) => document.querySelector("#api-key-list").textContent.includes(name), name);
    const longModel = "browser-long-model-" + "abcdefgh".repeat(11);
    await page.route("**/api/debug/models", (route) => route.fulfill({ json: { ...catalog, models: [...catalog.models, { ...catalog.models[0], model: longModel }] } }));
    await page.locator("#refresh-now").click();
    await page.waitForFunction((model) => debugData.models.some((item) => item.model === model), longModel);
    await page.setViewportSize({ width: 390, height: 844 });
    await openImport();
    await page.locator("#ccswitch-model-input").fill("browser-long-model");
    await page.getByRole("option", { name: longModel, exact: true }).click();
    assert(await page.locator("#ccswitch-modal").isVisible(), "mouse selection must not click through to backdrop");
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), "long names do not overflow document");
    await page.screenshot({ path: "output/playwright/import-long-390.png" });
    await page.locator("#ccswitch-close").click();
    await openImport();
    await page.locator("#ccswitch-modal").click({ position: { x: 2, y: 2 } });
    assert(await page.locator("#ccswitch-modal").isHidden(), "backdrop click still closes modal");
    await page.unroute("**/api/debug/models");
    await page.locator("#refresh-now").click();
  });

  await check("all pages desktop mobile no overflow", async () => {
    for (const width of [1440, 1024, 390, 320]) {
      await page.setViewportSize({ width, height: width === 390 ? 844 : 900 });
      for (const name of [
        "总览",
        "请求台",
        "接入",
        "事件日志",
        "API Key",
        "模型与额度",
        "模型轮转",
        "配置",
        "账号",
      ]) {
        await navigate(name);
        const overflow = await page.evaluate(
          () => document.documentElement.scrollWidth > innerWidth + 1,
        );
        assert(!overflow, `${name} overflows ${width}px`);
      }
      await page.evaluate(() => applyTheme({ mode: "dark", accent: "#7c3aed" }));
      await navigate("总览");
      if (width <= 390) {
        const table = page.locator("#upstream-requests").locator("xpath=ancestor::table");
        assert((await table.boundingBox()).width >= 672, "wide request table stays legible inside scroll container");
      }
      await page.screenshot({ path: `output/playwright/overview-${width}.png`, fullPage: true });
      await navigate("API Key");
      await openImport();
      await page.locator("#ccswitch-model-input").click();
      await page.screenshot({ path: `output/playwright/import-${width}.png` });
      await page.locator("#ccswitch-model-input").press("Escape");
      await page.locator("#ccswitch-close").click();
    }
  });

  await check("delete key and logout clears modal secrets", async () => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await navigate("API Key");
    await page.evaluate(() => {
      window.browserConfirm = window.confirm;
      window.confirm = () => true;
    });
    await importRow().getByRole("button", { name: "删除", exact: true }).click();
    await importRow().waitFor({ state: "detached" });
    await page.evaluate(() => {
      window.confirm = window.browserConfirm;
      delete window.browserConfirm;
    });
    const denied = await page.request.get(`${gatewayURL}/v1/models`, {
      headers: { Authorization: `Bearer ${created.key}` },
    });
    assert(denied.status() === 401, "deleted key rejected");
    await page.evaluate(() => {
      document.querySelector("#a-current").value = "test-account-password";
      document.querySelector("#api-key-value").value = "test-custom-key";
      document.querySelector("#copy-value").value = "test-secret";
      openModal("#copy-modal");
      showLogin();
    });
    assert((await page.locator("#copy-value").inputValue()) === "", "logout clears copy buffer");
    assert(await page.locator("#copy-modal").isHidden(), "logout closes modal");
    assert((await page.locator("#a-current").inputValue()) === "", "logout clears password form");
    assert((await page.locator("#api-key-value").inputValue()) === "", "logout clears custom key form");
    await page.evaluate(async () => {
      let deny;
      navigator.clipboard.writeText = () => new Promise((resolve, reject) => { deny = reject; });
      const pending = copyText("test-secret-after-logout");
      showLogin();
      deny(new Error("delayed clipboard denial"));
      await pending;
    });
    assert((await page.locator("#copy-value").inputValue()) === "", "late clipboard failure cannot restore secret buffer");
    assert(await page.locator("#copy-modal").isHidden(), "late clipboard failure cannot reopen modal");
  });
  assert(errors.length === 0, `Browser errors: ${errors.join("\n")}`);
  return { passed: checks, browserErrors: errors };
}
