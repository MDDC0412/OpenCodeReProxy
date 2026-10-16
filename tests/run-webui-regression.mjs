import { spawn } from "node:child_process";
import { closeSync, openSync } from "node:fs";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { createServer } from "node:net";
import { createRequire } from "node:module";
import path from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const require = createRequire(import.meta.url);
const cli = path.join(
  path.dirname(require.resolve("@playwright/cli/package.json")),
  "playwright-cli.js",
);
const session = `o2api-regression-${process.pid}`;
const state = path.join(root, ".local", "browser", `run-${Date.now()}-${process.pid}`);
const output = path.join(root, "output", "playwright");
const children = [];

async function run(command, args) {
  return await new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd: root,
      windowsHide: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let text = "";
    child.stdout.on("data", (data) => {
      text += data;
    });
    child.stderr.on("data", (data) => {
      text += data;
    });
    child.once("error", reject);
    child.once("exit", (code) => {
      if (code !== 0) reject(new Error(`${command} exited with ${code}\n${text}`));
      else resolve(text);
    });
  });
}

async function available(port) {
  await new Promise((resolve, reject) => {
    const server = createServer();
    server.once("error", () =>
      reject(new Error(`Port ${port} is already in use; no existing service was stopped.`)),
    );
    server.listen(port, "127.0.0.1", () => server.close(resolve));
  });
}

function start(command, args, name, env = process.env) {
  const log = openSync(path.join(state, `${name}.log`), "w");
  const child = spawn(command, args, {
    cwd: root,
    env,
    windowsHide: true,
    stdio: ["ignore", log, log],
  });
  closeSync(log);
  let startupError;
  child.once("error", (error) => {
    startupError = error;
  });
  const exited = new Promise((resolve) => {
    child.once("exit", resolve);
    child.once("error", resolve);
  });
  children.push({ child, exited });
  return () => {
    if (startupError) throw startupError;
    if (child.exitCode !== null)
      throw new Error(`${name} exited; see ${path.join(state, `${name}.log`)}`);
  };
}

async function ready(url, alive) {
  const deadline = Date.now() + 45000;
  while (Date.now() < deadline) {
    alive();
    try {
      const response = await fetch(url, { signal: AbortSignal.timeout(1500) });
      await response.arrayBuffer();
      if (response.ok) return;
    } catch {
      /* Startup can briefly refuse connections. */
    }
    await delay(250);
  }
  throw new Error(`Timed out waiting for ${url}; inspect ${state}`);
}

try {
  for (const port of [19090, 19091, 19092]) await available(port);
  await mkdir(state, { recursive: true });
  await mkdir(output, { recursive: true });
  const config = JSON.parse(await readFile(path.join(root, "config.example.json"), "utf8"));
  config.listen = "127.0.0.1:19091";
  config.server_keys = ["browser-fixture-local-key-only"];
  config.zen_keys = ["browser-fixture-zen-key-only"];
  config.go_keys = ["browser-fixture-go-key-only"];
  config.anonymous = true;
  config.proxies = ["direct"];
  config.upstream = {
    zen: "http://127.0.0.1:19092/zen",
    go: "http://127.0.0.1:19092/go",
    anonymous: "http://127.0.0.1:19092/anonymous",
  };
  config.models.protocols = Object.fromEntries(
    ["big-pickle", "glm-5.3", "glm-5.3-flash", "kimi-k3", "qwen3.8-max"].map((id) => [id, "chat"]),
  );
  config.webui = {
    ...config.webui,
    enabled: true,
    listen: "127.0.0.1:19090",
    username: "admin",
    password: "browser-fixture-password-only",
  };
  config.retry.timeout_seconds = 15;
  const configPath = path.join(state, "config.json");
  await writeFile(configPath, JSON.stringify(config, null, 2) + "\n");
  const binary = path.join(
    state,
    process.platform === "win32" ? "opencode2api.exe" : "opencode2api",
  );
  console.log("Building WebUI regression gateway with an isolated configuration…");
  await run("go", ["build", "-trimpath", "-o", binary, "./cmd/opencode2api"]);
  const fixtureAlive = start(process.execPath, ["tests/ui-upstream.mjs"], "fixture", {
    ...process.env,
    O2API_FIXTURE_PORT: "19092",
  });
  await ready("http://127.0.0.1:19092/zen/v1/models", fixtureAlive);
  const gatewayAlive = start(binary, ["-config", configPath], "gateway");
  await ready("http://127.0.0.1:19091/healthz", gatewayAlive);
  await run(process.execPath, [cli, `-s=${session}`, "open", "http://127.0.0.1:19090/"]);
  console.log("Running Chromium WebUI regression…");
  const result = await run(process.execPath, [
    cli,
    `-s=${session}`,
    "run-code",
    "--filename",
    "tests/webui-regression.js",
  ]);
  const json = result.match(/^### Result\s*\r?\n([^\r\n]+)/m)?.[1];
  if (!json) throw new Error(`Regression did not reach its final result:\n${result}`);
  const report = JSON.parse(json);
  if (!report.passed?.length || report.browserErrors?.length !== 0)
    throw new Error(`Invalid regression result: ${json}`);
  await writeFile(path.join(output, "result.json"), JSON.stringify(report, null, 2) + "\n");
  console.log(JSON.stringify(report, null, 2));
  console.log(
    "Screenshots and result: output/playwright/. Test state: " + path.relative(root, state),
  );
} catch (error) {
  console.error(error.message);
  console.error("Regression diagnostics: " + path.relative(root, state));
  process.exitCode = 1;
} finally {
  try {
    await run(process.execPath, [cli, `-s=${session}`, "close"]);
  } catch {
    /* A failed startup may not create a session. */
  }
  for (const { child, exited } of children.reverse()) {
    if (child.exitCode === null && !child.killed) child.kill();
    await exited;
  }
}
