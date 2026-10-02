// Every byte of this script was printed by a Malbolge program.
(function () {
  "use strict";
  var button = document.getElementById("count");
  var counter = document.getElementById("counter");
  var n = 0;
  if (button && counter) {
    button.addEventListener("click", function () {
      n += 1;
      counter.textContent = String(n);
    });
  }
  fetch("/data/info.json")
    .then(function (r) { return r.ok ? r.json() : null; })
    .then(function (info) {
      var el = document.getElementById("info");
      if (info && el) { el.textContent = info.name + " — " + info.tagline; }
    })
    .catch(function () {});
})();
