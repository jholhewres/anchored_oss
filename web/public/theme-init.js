// Applies the saved theme before the bundle runs, so the first paint already
// uses it. Served from the same origin because the CSP forbids inline scripts.
// Keep the key and values in sync with src/lib/theme.ts.
(function () {
  var theme = "light";
  try {
    if (window.localStorage.getItem("anchored_theme") === "warm") theme = "warm";
  } catch (e) {
    theme = "light";
  }
  var root = document.documentElement;
  root.dataset.theme = theme;
  root.style.colorScheme = theme === "warm" ? "dark" : "light";
})();
