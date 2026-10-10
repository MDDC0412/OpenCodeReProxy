(function () {
  const defaults = { mode: "light", accent: "#0d9488" };
  const key = "o2api-theme";
  let current = defaults;

  function normalize(value) {
    return {
      mode: value?.mode === "dark" ? "dark" : "light",
      accent: /^#[0-9a-f]{6}$/i.test(value?.accent || "")
        ? value.accent.toLowerCase()
        : defaults.accent,
    };
  }

  function luminance(hex) {
    const [r, g, b] = [1, 3, 5].map((i) => {
      const channel = parseInt(hex.slice(i, i + 2), 16) / 255;
      return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
    });
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  }

  function contrast(a, b) {
    const values = [luminance(a), luminance(b)].sort((x, y) => y - x);
    return (values[0] + 0.05) / (values[1] + 0.05);
  }

  function mix(hex, target, amount) {
    return (
      "#" +
      [1, 3, 5]
        .map((i) => {
          const start = parseInt(hex.slice(i, i + 2), 16);
          const end = parseInt(target.slice(i, i + 2), 16);
          return Math.round(start + (end - start) * amount)
            .toString(16)
            .padStart(2, "0");
        })
        .join("")
    );
  }

  function foreground(hex) {
    return contrast(hex, "#ffffff") >= contrast(hex, "#000000") ? "#ffffff" : "#000000";
  }

  function apply(value, persist = true) {
    current = normalize(value);
    const root = document.documentElement;
    const surface = current.mode === "dark" ? "#161b22" : "#ffffff";
    const soft = mix(surface, current.accent, current.mode === "dark" ? 0.16 : 0.08);
    const target = current.mode === "dark" ? "#ffffff" : "#101828";
    let ink = current.accent;
    for (let amount = 0; contrast(ink, soft) < 4.5 && amount <= 1; amount += 0.05) {
      ink = mix(current.accent, target, amount);
    }
    const hover = mix(current.accent, "#101828", 0.14);
    const on = foreground(current.accent);
    root.dataset.mode = current.mode;
    root.dataset.accentOn = on === "#ffffff" ? "light" : "dark";
    for (const [name, color] of Object.entries({
      accent: current.accent,
      "accent-hover": hover,
      "accent-on": on,
      "accent-hover-on": foreground(hover),
      "accent-ink": ink,
      "accent-soft": soft,
      "accent-border": mix(surface, current.accent, 0.35),
    }))
      root.style.setProperty(`--${name}`, color);
    if (persist) {
      try {
        localStorage.setItem(key, JSON.stringify(current));
      } catch (_) {}
    }
    return { ...current };
  }

  try {
    current = normalize(JSON.parse(localStorage.getItem(key)));
  } catch (_) {}
  apply(current, false);
  window.o2apiTheme = { get: () => ({ ...current }), apply };
})();
