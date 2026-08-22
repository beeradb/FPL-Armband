/* The signup gate: ONE fetch, mounted through THREE forms.
 *
 * This used to be landing.js, wiring the landing page's own hero form. The app becoming the
 * front door (see cmd/armband/webroutes.go's withSignups) gave the News tab's coming-soon
 * panel and the Pitch tab's news nudge a form of their own, asking for the same address —
 * and copying the fetch into app.js a second and third time would be this project's
 * signature failure in miniature. So the destination moved OUT of the script and onto the
 * form: every `form.gatecard` names its own target in `data-gate`, and this file wires
 * whichever of them the current page carries.
 *
 * Out of line rather than an inline onsubmit, for the same reason app.js is: a
 * Content-Security-Policy that has to allow 'unsafe-inline' to run this also allows
 * whatever an injected string manages to open. With every script external the page can
 * carry script-src 'self' and mean it.
 *
 * # The three configurations
 *
 * landing.html's two forms carry the ABSOLUTE `https://fplarmband.com/gate`. That page is
 * served both by the live site and by a local `armband serve`, and a relative /gate there
 * would put a local reader's address in a local list nobody collects — the same lost
 * submission a redirect used to cause. Locally that is a cross-origin post, which is why
 * the server echoes an Access-Control-Allow-Origin for loopback origins and answers 204
 * rather than a redirect: fetch would follow a redirect to / and be blocked by CORS,
 * turning a submission that SUCCEEDED into a reported failure.
 *
 * ⚠️ That absolute value is also spelled in cmd/armband/webroutes.go, where it enters the
 * Content-Security-Policy that permits the fetch. The two are pinned equal by
 * TestTheSignupOriginIsSpelledOnceInEffect, which now reads the URL off landing.html's
 * markup rather than off a constant in this file — there is no per-page constant left to
 * drift, only a `data-gate` value that either matches or does not.
 *
 * app.html's forms — the News tab's panel and the Pitch tab's nudge — carry the RELATIVE
 * `/gate`, because those forms are only ever served from this origin, and /app's own
 * connect-src 'self' has no reason to widen for them (see connectSrcFor's doc comment).
 *
 * # What happens on success, and why this file does not decide it
 *
 * The landing forms carry a `data-gate-redirect` and navigate there — the reader was on a
 * marketing page and the point was always to leave it. The in-app forms carry no redirect:
 * nothing may navigate out from under a reader mid-task, so this file instead dispatches a
 * bubbling `armband:signedup` event and lets the page's own script decide what changes.
 * gate.js knows nothing about the News tab or the Pitch view — see app.js's NEEDS_SIGNUP
 * comment for the one place both of those mounts read to agree with each other and with
 * this event, without a second implementation of "has this reader already answered".
 */
(function () {
  'use strict';

  function wireOne(form) {
    if (form.dataset.gateWired) return;
    form.dataset.gateWired = '1';

    var url = form.dataset.gate;
    var input = form.querySelector('input[type=email]');
    var button = form.querySelector('button[type=submit]');
    var done = form.querySelector('.done');
    if (!url || !input || !button) return;

    /* The server reads a form field by name. A form missing this sends nothing the
       reader typed, no matter how convincing the rest of the markup looks. */
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

      fetch(url, {
        method: 'POST',
        /* same-origin so a same-site form sends and receives the signup cookie; the
           cross-origin (local-serves-to-live) case sends and sets none, which is the
           whole reason allowGateOrigin omits Access-Control-Allow-Credentials. */
        credentials: 'same-origin',
        /* Form encoding keeps this a CORS "simple request" for the cross-origin case,
           so it needs no preflight and the server needs no OPTIONS route to answer one. */
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: 'email=' + encodeURIComponent(input.value)
      })
        .then(function (r) {
          /* 204 means recorded. Anything else is not a success to report as one -- 400
             for an address the server would not take, 500 for a write that did not
             land, and the reader can act on both by retrying. The body is read before
             anything is thrown: /gate's own error text is already written for the
             person reading it, and discarding it for the status code would replace a
             helpful answer with a number nobody can act on. */
          if (r.ok) {
            var redirect = form.dataset.gateRedirect;
            if (redirect) { window.location.href = redirect; return; }
            /* In place: nothing navigates, so the page's own script has to notice.
               Bubbling, so a document-level listener catches it regardless of which
               of the (possibly several) mounted forms fired. */
            form.dispatchEvent(new CustomEvent('armband:signedup', { bubbles: true }));
            return;
          }
          return r.text().then(function (msg) {
            throw new Error(msg || ('the server answered ' + r.status));
          });
        })
        .catch(function (err) {
          button.disabled = false;
          button.textContent = label;
          if (done) {
            done.hidden = false;
            done.textContent = 'That did not go through: ' + err.message;
          }
        });
    });
  }

  /* wireGateForms is called once at load for whatever the document already carries, and
     again by app.js after it renders a fresh `form.gatecard` from a template string --
     innerHTML never runs the listeners this file already attached to the nodes it
     replaced. Re-wiring an already-wired form is a no-op (the dataset guard above), so
     calling this liberally costs nothing. */
  function wireGateForms(root) {
    (root || document).querySelectorAll('form.gatecard[data-gate]').forEach(wireOne);
  }

  window.wireGateForms = wireGateForms;
  document.addEventListener('DOMContentLoaded', function () { wireGateForms(document); });
})();
