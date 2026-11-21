// Applies the saved theme before first paint to avoid a flash. Kept as an
// external file so the panel's Content-Security-Policy can forbid inline scripts.
try {
  var t = localStorage.getItem("breefer-theme") || "light";
  if (t === "system") t = matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  document.documentElement.dataset.theme = t;
} catch (e) {}
