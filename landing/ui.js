// Small progressive-enhancement UI helpers for the landing page.
// No dependencies - safe to load with `defer`.

(function () {
  "use strict";

  // Current year in the footer.
  var yearEl = document.getElementById("year");
  if (yearEl) {
    yearEl.textContent = String(new Date().getFullYear());
  }

  // Light/dark toggle. The page follows the system until the visitor picks a
  // theme here; the choice is saved and applied before paint by the inline
  // script in <head>. The button shows the theme it switches to.
  var toggle = document.getElementById("themeToggle");
  if (toggle) {
    var root = document.documentElement;
    var systemDark = window.matchMedia("(prefers-color-scheme: dark)");
    var current = function () {
      return root.dataset.theme || (systemDark.matches ? "dark" : "light");
    };
    var render = function () {
      var target = current() === "dark" ? "light" : "dark";
      // Labels come from the page (translated per language).
      var label = target === "dark" ? toggle.dataset.labelDark : toggle.dataset.labelLight;
      toggle.dataset.target = target;
      toggle.setAttribute("aria-label", label);
      toggle.title = label;
    };
    toggle.addEventListener("click", function () {
      var next = current() === "dark" ? "light" : "dark";
      root.dataset.theme = next;
      try {
        localStorage.setItem("theme", next);
      } catch (e) {}
      render();
    });
    systemDark.addEventListener("change", render);
    render();
    toggle.hidden = false;
  }

  // Before / after comparison slider.
  // The range input is the single source of truth (keyboard + pointer
  // accessible); we mirror its value into a CSS custom property that drives
  // both the clip-path of the "before" layer and the handle position.
  var ba = document.getElementById("ba");
  var range = document.getElementById("baRange");
  if (ba && range) {
    var apply = function () {
      ba.style.setProperty("--pos", range.value + "%");
    };
    range.addEventListener("input", apply);
    apply();
  }
})();
