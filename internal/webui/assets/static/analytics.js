/* GA4 loader for the landing page only. /app never references this file.
 *
 * Out of line rather than an inline bootstrap, for the same reason landing.js and app.js
 * are: script-src 'self' has no 'unsafe-inline', so an inline <script> block would not
 * run at all under this page's Content-Security-Policy — it would just silently fail to
 * track anyone. Everything below is DOM manipulation from a file the policy already
 * trusts, which is the only shape that works.
 *
 * The measurement id travels through the server, not this file. `armband serve` fills
 * the <meta name="armband-ga4"> tag from ARMBAND_GA4_ID; the embedded document ships
 * with the tag's content empty, so a copy of this page taken off disk -- or any local
 * run with the env var unset, which is every test and the common case -- loads nothing
 * and sends nothing. That is the return below, and it is the whole gate: no id, no
 * script tag, no request to a Google host, ever.
 *
 * connect-src and img-src only permit the Google origins when the same env var was set
 * server-side (see cmd/armband/webroutes.go). An empty id here always pairs with an
 * unwidened policy, so there is nothing for this file to reach even if it ran anyway.
 */
(function () {
  'use strict';

  var meta = document.querySelector('meta[name="armband-ga4"]');
  var id = meta ? meta.content : '';
  if (!id) return;

  /* The standard GA4 bootstrap, built as elements rather than written as markup: a
     template string handed to innerHTML would need an inline <script> to execute, which
     is exactly what this page's policy forbids. */
  var loader = document.createElement('script');
  loader.async = true;
  loader.src = 'https://www.googletagmanager.com/gtag/js?id=' + encodeURIComponent(id);
  document.head.appendChild(loader);

  window.dataLayer = window.dataLayer || [];
  function gtag() { window.dataLayer.push(arguments); }
  window.gtag = gtag;
  gtag('js', new Date());
  gtag('config', id);
})();
