// Run with node scripts/map-zoom-test.js. Checks page-level zoom without a browser.
function testMapZoom(source) {
  const assert = (ok, message) => { if (!ok) throw new Error(message); };
  const equal = (actual, expected, message) => assert(Math.abs(actual - expected) < 0.001, message + ": " + actual + " != " + expected);
  const listeners = {}, windowListeners = {};
  const attrs = { width: "2000", height: "12000" };
  let left = 0, top = 1200;
  const output = { textContent: "100%" };
  const buttons = ["in", "out", "reset", "fit"].map((action) => ({
    dataset: { mapZoom: action }, disabled: false, callback: null,
    addEventListener(event, callback) { assert(event === "click", "button handler"); this.callback = callback; },
    click() { if (!this.disabled) this.callback(); },
  }));
  const button = (action) => buttons.find((b) => b.dataset.mapZoom === action);
  const window = {
    innerWidth: 1000, innerHeight: 600,
    scrollBy(dx, dy) {
      assert(Number.isFinite(dx) && Number.isFinite(dy), "finite page scroll deltas");
      left = Math.max(0, Math.min(left + dx, Math.max(0, 24 + Number(attrs.width) - this.innerWidth)));
      top = Math.max(0, Math.min(top + dy, Math.max(0, 300 + Number(attrs.height) + 1000 - this.innerHeight)));
    },
    addEventListener: (event, callback) => { windowListeners[event] = callback; },
  };
  const svg = {
    getAttribute: (key) => attrs[key], setAttribute: (key, value) => { attrs[key] = value; },
    getBoundingClientRect: () => ({ left: 24-left, right: 24-left+Number(attrs.width), top: 300-top, bottom: 300-top+Number(attrs.height) }),
  };
  // The canvas expands naturally and deliberately provides no internal scroll API.
  const canvas = { clientWidth: 952, querySelector: (query) => query === "svg" ? svg : null };
  const controls = { hidden: true, querySelector: () => output, querySelectorAll: () => buttons };
  const viewer = {
    querySelector: (query) => query === "[data-map-canvas]" ? canvas : controls,
    addEventListener: (event, callback) => { listeners[event] = callback; },
  };
  Function("document", "window", source)({ querySelectorAll: () => [viewer] }, window);
  assert(!controls.hidden, "show controls");
  equal(Number(attrs.width), 952, "initial fit to browser width");
  equal(top, 1200, "initialization does not move page");
  button("reset").click();
  left = 600; top = 3000;
  button("in").click();
  equal(Number(attrs.width), 2500, "zoom in width");
  equal(Number(attrs.height), 15000, "full natural diagram height");
  equal(left, 869, "preserve viewport horizontal anchor");
  equal(top, 3750, "preserve viewport vertical anchor");
  assert(output.textContent === "125%", "announce percentage");
  button("out").click();
  equal(left, 600, "round trip horizontal position");
  equal(top, 3000, "round trip vertical position");
  for (let i=0; i<30; i++) button("in").click();
  equal(Number(attrs.width), 6000, "upper bound 300%");
  assert(button("in").disabled, "disable upper-bound button");
  for (let i=0; i<50; i++) button("out").click();
  equal(Number(attrs.width), 200, "lower bound 10%");
  assert(button("out").disabled, "disable lower-bound button");
  equal(left, 0, "page scroll clamped when map fits");
  button("fit").click();
  equal(Number(attrs.width), 952, "fit width");
  const key = (key, modifiers={}) => {
    const event = { key, prevented: false, preventDefault() { this.prevented=true; }, ...modifiers };
    listeners.keydown(event); return event;
  };
  assert(key("+").prevented, "map keyboard zoom");
  equal(Number(attrs.width), 1190, "keyboard zoom");
  assert(!key("+", {ctrlKey:true}).prevented, "preserve browser Ctrl shortcuts");
  assert(!key("-", {metaKey:true}).prevented, "preserve browser Cmd shortcuts");
  equal(Number(attrs.width), 1190, "modifier shortcuts do not zoom map");
  assert(!key("ArrowDown").prevented, "native page scrolling");
  assert(key("0").prevented, "keyboard reset");
  equal(Number(attrs.width), 2000, "reset 100%");
  button("fit").click();
  canvas.clientWidth=1200; window.innerWidth=1248; windowListeners.resize();
  equal(Number(attrs.width), 1200, "fit follows browser resize");
  button("in").click();
  canvas.clientWidth=1400; window.innerWidth=1448; windowListeners.resize();
  equal(Number(attrs.width), 1500, "manual zoom retained on resize");
  return {status:"passed", behavior:"page anchors, initial fit, natural height, resize, limits, keyboard"};
}
if (typeof module !== "undefined" && require.main === module) {
  const fs=require("node:fs"), path=require("node:path");
  console.log(testMapZoom(fs.readFileSync(path.join(__dirname,"../internal/ui/templates/map.js"),"utf8")));
}
