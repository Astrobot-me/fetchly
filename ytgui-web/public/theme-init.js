// Sets the theme before first paint (a separate file because the app's CSP blocks inline scripts).
// Dark is the default on first visit, whatever the OS setting.
(function () {
  var theme = "dark";
  try {
    if (window.localStorage.getItem("fetchly.theme") === "light") theme = "light";
  } catch (e) {
    /* storage blocked: stay dark */
  }
  var root = document.documentElement;
  root.setAttribute("data-theme", theme);
  root.style.colorScheme = theme;
})();
