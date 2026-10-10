import http from "node:http";

const models = ["big-pickle", "glm-5.3", "glm-5.3-flash", "kimi-k3", "qwen3.8-max"];
const port = Number(process.env.O2API_FIXTURE_PORT || 9092);
const server = http.createServer(async (request, response) => {
  const url = new URL(request.url, "http://localhost");
  const send = (status, value) => {
    response.writeHead(status, { "Content-Type": "application/json" });
    response.end(JSON.stringify(value));
  };
  if (url.pathname.endsWith("/models")) {
    const ids = url.pathname.startsWith("/go/") ? models.slice(1) : models;
    return send(200, { object: "list", data: ids.map((id) => ({ id, object: "model" })) });
  }
  if (url.pathname.endsWith("/usage")) {
    return send(200, {
      usage: Object.fromEntries(
        ["rolling", "weekly", "monthly"].map((name, index) => [
          name,
          {
            status: "ok",
            percent: [17, 32, 46][index],
            resetsAt: new Date(Date.now() + 3600000).toISOString(),
          },
        ]),
      ),
    });
  }
  if (url.pathname.endsWith("/chat/completions")) {
    let body = "";
    for await (const chunk of request) body += chunk;
    const payload = JSON.parse(body);
    const result = {
      id: "chatcmpl-browser-fixture",
      object: "chat.completion",
      created: 1,
      model: payload.model,
      choices: [
        {
          index: 0,
          message: { role: "assistant", content: "本地模拟上游响应" },
          finish_reason: "stop",
        },
      ],
      usage: { prompt_tokens: 18, completion_tokens: 7, total_tokens: 25 },
    };
    if (!payload.stream) return send(200, result);
    response.writeHead(200, { "Content-Type": "text/event-stream" });
    response.write(
      `data: ${JSON.stringify({ ...result, object: "chat.completion.chunk", choices: [{ index: 0, delta: result.choices[0].message, finish_reason: null }] })}\n\n`,
    );
    response.end(
      `data: ${JSON.stringify({ ...result, object: "chat.completion.chunk", choices: [{ index: 0, delta: {}, finish_reason: "stop" }] })}\n\ndata: [DONE]\n\n`,
    );
    return;
  }
  send(404, { error: { message: "fixture endpoint not found" } });
});
server.listen(port, "127.0.0.1", () => console.log(`Browser fixture: http://127.0.0.1:${port}`));
for (const signal of ["SIGINT", "SIGTERM"]) process.on(signal, () => server.close());
