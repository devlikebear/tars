// Package consolewatch brings back a console page that loaded but never
// started.
//
// A server restart in the middle of a page load leaves the console's HTML
// in the window with its script missing: an empty root and a white window
// until the user opens the console again. The desktop shell restarts an
// outdated server right after it opens the console (the first launch after
// an app update), so this is what that launch showed.
package consolewatch

// Script is injected into every page the window loads. On a console page
// whose root is still empty two seconds after the load, it reloads the
// page once the server answers it; while the server is down it keeps
// waiting on this page. Attempts back off (2s, 4s, 8s, … up to 30s) and the
// count lives in sessionStorage, so a console that cannot start is not
// reloaded in a tight loop; a console that started resets it. A window that
// comes back into view or takes focus is checked at once.
const Script = `(function () {
  if (window.__tarsConsoleWatch || location.pathname.indexOf("/console") !== 0) return;
  window.__tarsConsoleWatch = true;
  var key = "tars-desktop-reloads";
  var get = function () { try { return Number(sessionStorage.getItem(key)) || 0; } catch (_) { return 0; } };
  var set = function (n) { try { sessionStorage.setItem(key, String(n)); } catch (_) {} };
  var wait = function (n) { return Math.min(2000 * Math.pow(2, n), 30000); };
  var check = function () {
    var root = document.getElementById("app");
    if (!root || root.childElementCount > 0) { set(0); return; }
    var n = get() + 1;
    set(n);
    var later = function () { setTimeout(check, wait(n)); };
    // Reload only once the server answers: a reload that fails can leave
    // an empty error page, which nothing would bring back.
    fetch(location.href, {cache: "no-store"}).then(function (r) {
      if (r.ok) location.reload(); else later();
    }, later);
  };
  setTimeout(check, wait(get()));
  // WebKit stops the timers of a page nobody can see, so a window brought
  // back is checked at once.
  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState === "visible") check();
  });
  window.addEventListener("focus", check);
})();`
