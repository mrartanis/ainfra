(() => {
  "use strict";
  for (const viewer of document.querySelectorAll("[data-map-viewer]")) {
    const canvas = viewer.querySelector("[data-map-canvas]");
    const svg = canvas && canvas.querySelector("svg");
    const controls = viewer.querySelector("[data-map-controls]");
    if (!svg || !controls) continue;
    const width = Number(svg.getAttribute("width"));
    const height = Number(svg.getAttribute("height"));
    if (!(width > 0 && height > 0)) continue;
    const output = controls.querySelector("[data-map-scale]");
    const buttons = Array.from(controls.querySelectorAll("[data-map-zoom]"));
    const min = 0.1, max = 3;
    let scale = 1;
    let fitted = true;
    const zoom = (value, preservePosition = true) => {
      const next = Math.min(max, Math.max(min, value));
      const before = svg.getBoundingClientRect();
      const vx = Math.max(before.left, Math.min(before.right, window.innerWidth / 2));
      const vy = Math.max(before.top, Math.min(before.bottom, window.innerHeight / 2));
      const x = (vx - before.left) / scale;
      const y = (vy - before.top) / scale;
      scale = next;
      svg.setAttribute("width", String(Math.round(width * scale)));
      svg.setAttribute("height", String(Math.round(height * scale)));
      if (preservePosition) {
        const after = svg.getBoundingClientRect();
        window.scrollBy(after.left + x * scale - vx, after.top + y * scale - vy);
      }
      output.textContent = Math.round(scale * 100) + "%";
      for (const button of buttons) {
        button.disabled = button.dataset.mapZoom === "in" && scale >= max ||
          button.dataset.mapZoom === "out" && scale <= min;
      }
    };
    const act = (action) => {
      fitted = action === "fit";
      if (action === "in") zoom(scale * 1.25);
      if (action === "out") zoom(scale / 1.25);
      if (action === "reset") zoom(1);
      if (action === "fit") zoom(canvas.clientWidth / width);
    };
    for (const button of buttons) {
      button.addEventListener("click", () => act(button.dataset.mapZoom));
    }
    viewer.addEventListener("keydown", (event) => {
      if (event.ctrlKey || event.metaKey || event.altKey) return;
      const action = { "+": "in", "=": "in", "-": "out", "0": "reset" }[event.key];
      if (!action) return;
      event.preventDefault();
      act(action);
    });
    controls.hidden = false;
    // The document owns scrolling; the map expands to its full scaled height.
    // Fit the initial view and keep fitting on resize until the user zooms.
    zoom(canvas.clientWidth / width, false);
    window.addEventListener("resize", () => {
      if (fitted) zoom(canvas.clientWidth / width);
    });
  }
})();
