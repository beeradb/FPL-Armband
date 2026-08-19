/* The landing page's gate.
 *
 * Out of line rather than an inline onsubmit, for the same reason app.js is: a
 * Content-Security-Policy that has to allow 'unsafe-inline' to run this also allows
 * whatever an injected string manages to open. With both scripts external the page can
 * carry script-src 'self' and mean it.
 *
 * What the gate does today is let you through. It posts to /gate, which validates the shape
 * of the address, stores nothing anywhere, sets a session cookie and redirects. The form
 * shipped as `onsubmit="event.preventDefault()"` with no action and no name on the input,
 * so it showed "check your inbox" and sent nothing -- the one state it had was success.
 * Pending and failure exist here because the network can fail even when the server cannot.
 */
(function () {
  'use strict';

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

      fetch('/gate', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: 'email=' + encodeURIComponent(input.value)
      })
        .then(function (r) {
          /* The handler answers 303 to /app. fetch follows redirects, so a success is an
             ok response whose URL is the application -- and going there is the whole
             point of the gate. */
          if (!r.ok) throw new Error('the server answered ' + r.status);
          window.location.href = '/app';
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
