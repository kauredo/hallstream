/* The program's words in the browser. /i18n.js, loaded just before this file, sets window.I18N
   (web/i18n/<lang>.json for settings.lang, with pt under it). t(key, vars) fills {name} from vars.
   Static HTML: data-i18n="key" sets textContent, data-i18n-html="key" sets innerHTML (copy with
   markup), data-i18n-attr="placeholder:key;aria-label:key" sets attributes. Both pages load this
   at the end of <body>, before their own scripts, so the words are in place before anything draws. */
(function () {
  'use strict';
  var words = window.I18N || {};

  function t(key, vars) {
    var s = Object.prototype.hasOwnProperty.call(words, key) ? words[key] : key;
    if (!vars) return s;
    return s.replace(/\{([A-Za-z0-9_]+)\}/g, function (m, name) {
      return Object.prototype.hasOwnProperty.call(vars, name) ? String(vars[name]) : m;
    });
  }

  function apply(root) {
    if (!root && words['data.locale']) document.documentElement.lang = words['data.locale'];
    root = root || document;
    var all = function (sel) { return Array.prototype.slice.call(root.querySelectorAll(sel)); };
    all('[data-i18n]').forEach(function (el) { el.textContent = t(el.getAttribute('data-i18n')); });
    all('[data-i18n-html]').forEach(function (el) { el.innerHTML = t(el.getAttribute('data-i18n-html')); });
    all('[data-i18n-attr]').forEach(function (el) {
      el.getAttribute('data-i18n-attr').split(';').forEach(function (pair) {
        var i = pair.indexOf(':');
        if (i > 0) el.setAttribute(pair.slice(0, i).trim(), t(pair.slice(i + 1).trim()));
      });
    });
  }

  window.t = t;
  window.applyI18N = apply;
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', function () { apply(); });
  else apply();
})();
