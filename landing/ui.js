// Small progressive-enhancement UI helpers for the landing page.
// No dependencies - safe to load with `defer`.

(function () {
  "use strict";

  // Current year in the footer.
  var yearEl = document.getElementById("year");
  if (yearEl) {
    yearEl.textContent = String(new Date().getFullYear());
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
