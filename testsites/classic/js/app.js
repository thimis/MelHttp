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
  if (window.WebAssembly && WebAssembly.instantiateStreaming) {
    WebAssembly.instantiateStreaming(fetch("/wasm/add.wasm"))
      .then(function (r) {
        var el = document.getElementById("wasm");
        if (el) { el.textContent = "add(2, 3) = " + r.instance.exports.add(2, 3); }
      })
      .catch(function () {});
  }
  fetch("/data/info.json")
    .then(function (r) { return r.ok ? r.json() : null; })
    .then(function (info) {
      var el = document.getElementById("info");
      if (info && el) { el.textContent = info.name + " — " + info.tagline; }
    })
    .catch(function () {});
})();
