/* The landing page's gate.
 *
 * Out of line rather than an inline onsubmit, for the same reason app.js is: a
 * Content-Security-Policy that has to allow 'unsafe-inline' to run this also allows
 * whatever an injected string manages to open. With both scripts external the page can
 * carry script-src 'self' and mean it.
 *
 * The gate posts to https://fplarmband.com/gate, which validates the shape of the address,
 * records it, and answers 204. The form shipped as `onsubmit="event.preventDefault()"` with
 * no action and no name on the input, so it showed "check your inbox" and sent nothing --
 * the one state it had was success. Pending and failure exist here because the address now
 * goes somewhere, and a write that fails must not read as a write that worked.
 *
 * The URL is ABSOLUTE on purpose. This page is served both by the live site and by a local
 * `armband serve`, and a relative /gate would put a local reader's address in a local list
 * nobody collects -- the same lost submission as the discard this replaced. Absolute, both
 * copies of the page capture to one place. Locally that is a cross-origin post, which is
 * why the server echoes an Access-Control-Allow-Origin for loopback origins and answers
 * 204 rather than the redirect it used to: fetch would follow a redirect to /app and be
 * blocked by CORS, turning a submission that SUCCEEDED into a reported failure.
 *
 * ⚠️ This origin is also spelled in cmd/armband/webroutes.go, where it enters the
 * Content-Security-Policy that permits this fetch. The two are pinned equal by a test.
 */
(function () {
  'use strict';

  /* Must match signupOrigin in cmd/armband/webroutes.go, which puts this host in the
     Content-Security-Policy that permits the fetch below. Pinned equal by a test. */
  var GATE = 'https://fplarmband.com/gate';

  function wire(form) {
    var input = form.querySelector('input[type=email]');
    var button = form.querySelector('button[type=submit]');
    var done = form.querySelector('.done');
    if (!input || !button || !done) return;

    /* The server reads a form field by name. The markup had none, so nothing the reader
       typed was ever sent. */
    input.name = 'email';

    form.addEventListener('submit', function (event) {
      event.preventDefault();
      if (!input.checkValidity()) {
        input.reportValidity();
        return;
      }

      var label = button.textContent;
      button.disabled = true;
      button.textContent = 'One moment…';

      fetch(GATE, {
        method: 'POST',
        /* same-origin, so the public copy of this page sends and receives the gate
           cookie. It is load-bearing now: the public /app redirects a reader without
           that cookie back here, so a fetch that dropped it would bounce someone who
           had just signed up. The local copy sends nothing and needs nothing --
           `armband serve` has no signup store, so it does not gate at all. */
        credentials: 'same-origin',
        /* Form encoding keeps this a CORS "simple request", so the local cross-origin
           post needs no preflight -- and the server needs no OPTIONS route to answer
           one. A JSON body would have cost both. */
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: 'email=' + encodeURIComponent(input.value)
      })
        .then(function (r) {
          /* 204 means recorded. Anything else is not a success to report as one --
             400 for an address the server would not take, 500 for a write that did
             not land, and the reader can act on both by retrying.

             The body is read before anything is thrown. /gate's own error text is
             already written for the person reading it -- "that does not look like an
             email address", "we could not record that just now — please try again" --
             and discarding it in favour of the status code replaced a helpful answer
             with a number nobody can act on. */
          if (r.ok) { window.location.href = '/app'; return; }
          return r.text().then(function (msg) {
            throw new Error(msg || ('the server answered ' + r.status));
          });
        })
        .catch(function (err) {
          button.disabled = false;
          button.textContent = label;
          done.hidden = false;
          done.textContent = 'That did not go through: ' + err.message;
        });
    });
  }

  document.querySelectorAll('form.gatecard').forEach(wire);
})();
