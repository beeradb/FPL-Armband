// The probe. Deliberately a SEPARATE file from app.js: loading it proves that an
// external script from 'self' executes under `script-src 'self'`, while keeping the
// probe isolated from anything app.js does with a /api/state body it cannot hydrate.
(function () {
  var violations = [];
  document.addEventListener('securitypolicyviolation', function (e) {
    violations.push(e.violatedDirective + '<-' + (e.blockedURI || '?'));
  });

  // TWO independent report channels, because either one may be the thing that is
  // blocked. The image is governed by `img-src 'self' data:`; the fetch by
  // `connect-src 'self'`. If connect-src is what fails, the image still gets the
  // verdict out -- which is the whole question this spike exists to answer.
  function report(o) {
    var q = Object.keys(o).map(function (k) {
      return k + '=' + encodeURIComponent(String(o[k]));
    }).join('&');
    try { new Image().src = '/spike/ok?via=img&' + q; } catch (e) {}
    try { fetch('/spike/ok?via=fetch&' + q).catch(function () {}); } catch (e) {}
  }

  var out = {
    script: 'ran',
    href: location.href,
    origin: location.origin,
    proto: location.protocol,
    sheets: document.styleSheets.length
  };

  // Give the CSP a moment to have blocked whatever it is going to block.
  setTimeout(function () {
    fetch('/api/state', { credentials: 'same-origin' })
      .then(function (r) { return r.json(); })
      .then(function (d) {
        out.state = 'ok';
        out.marker = d.marker;
        out.v = violations.join('|') || 'none';
        report(out);
      })
      .catch(function (e) {
        out.state = 'blocked';
        out.err = String(e && e.message ? e.message : e);
        out.v = violations.join('|') || 'none';
        report(out);
      });
  }, 500);
})();
