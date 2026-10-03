// Opt in to the MelHttp Malbolge transport: include
//   <script src="/_melhttp/obfuscate.js"></script>
// on your pages. After the service worker activates (the first visit is
// plain), every same-origin GET travels as Malbolge programs.
(function () {
  "use strict";
  if (!("serviceWorker" in navigator)) return;
  navigator.serviceWorker.register("/_melhttp/sw.js", { scope: "/" }).then(function () {
    if (!navigator.serviceWorker.controller) {
      navigator.serviceWorker.addEventListener("controllerchange", function () {
        document.documentElement.dataset.malbolgeTransport = "active";
        window.dispatchEvent(new Event("malbolgetransport"));
      });
    } else {
      document.documentElement.dataset.malbolgeTransport = "active";
    }
  }).catch(function (err) {
    console.warn("MelHttp: service worker registration failed", err);
  });
})();
