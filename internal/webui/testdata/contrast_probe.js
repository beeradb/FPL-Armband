/* The contrast probe: it runs INSIDE the rendered page and publishes one record per
 * visible run of text — the colour the glyphs are painted in, the colour actually painted
 * behind them, and enough of the element to name it in a failure message.
 *
 * It lives here, in the page, because the question cannot be answered anywhere else. A
 * stylesheet scan cannot resolve inheritance, cannot composite an alpha against whatever is
 * really behind it, cannot tell which ancestor paints the background under a given run, and
 * cannot see a rule that never matches anything. It also cannot see `landing.html`'s own
 * <style> block or the styles `app.js` sets inline, both of which are invisible to every
 * scan in this repository — one of which is how a 7px declaration came to sit on the public
 * front page under a stylesheet that states 9px as absolute.
 *
 * The rules it measures against are WCAG 1.4.3: 4.5:1 for text, 3:1 for large text. What it
 * deliberately does NOT measure is in contrast_test.go, next to the reason.
 *
 * ---------------------------------------------------------------------------------------
 * The five cases that decide whether an instrument like this is worth having. Each is
 * decided here rather than left to fall out of the implementation.
 *
 * 1. TEXT OVER A GRADIENT. Different pixels of one glyph run sit over different colours, so
 *    there is no "the background". This samples the WORST CASE over the gradient's own
 *    colour stops. That is a true bound rather than a guess: sRGB channel values interpolate
 *    linearly between adjacent stops, the sRGB transfer function is monotonic per channel,
 *    and relative luminance is a positive weighted sum of the linearised channels — so every
 *    luminance along the run lies between the luminances of its bracketing stops, and the
 *    extremes are attained AT stops. The cost is that it can over-report where the text does
 *    not actually overlap the worst stop. If that ever cries wolf, the answer is to sample
 *    the text's own band, not to exempt the gradient.
 *
 * 2. SEMI-TRANSPARENT BACKGROUNDS. Composited source-over against what is actually beneath,
 *    never against white. `--scrim` is rgba(4,7,10,.72) and means nothing until it is
 *    composited.
 *    `opacity` is handled in the same walk and is NOT a per-layer multiplier, which is the
 *    reading that looks obvious and is wrong. It groups: the element and its subtree are
 *    rendered into a buffer and that buffer is composited once, at that opacity, over
 *    whatever is beneath the element. So the fade applies to everything accumulated so far
 *    and to nothing below. Treating it per layer charges an ancestor's opacity to the glyph
 *    twice and to its background once, which is a different colour, not a rounding — see
 *    `#s-group` in testdata/contrast_stacks.html, where the two readings differ by #ffffff
 *    against #c2c2c2.
 *
 * 3. BACKGROUNDS INHERITED FROM AN ANCESTOR. The walk starts at the TEXT and goes outward,
 *    compositing each layer beneath what it has already accumulated, and it TERMINATES AT THE
 *    PAGE — body and html are ancestors of everything, so the canvas colour is the last layer
 *    and a chain of `transparent` cannot run off the end.
 *    ⚠️ The direction is the whole of it, and getting it backwards is silent. The first
 *    version of this file folded from the canvas upward, so the first opaque layer it met was
 *    the page and it stopped there: every run on every screen scored against `--bg`, which is
 *    the darkest surface here, so with light inks the error ran in the flattering direction
 *    and sixteen screens came back clean. Everything below — the backdrops, the gradient
 *    stops, the whole of this case — was dead code beneath it.
 *    White is the final backstop and is reported if it is ever reached, because reaching it
 *    means the page has no background and the answer would otherwise be quietly wrong.
 *
 *    One thing paints outside that chain and is added to it explicitly: a NEGATIVE Z-INDEX
 *    backdrop layer, which paints above the canvas background and below in-flow content.
 *    `landing.html` has one — the fixed `.lbg` glow — and ignoring it makes the reported
 *    ratio about 1.18x the true one for every run that sits on the page background rather
 *    than on a panel: `--bg` is luminance 0.0032 and the glow at full strength puts 0.0128
 *    over it. Its coverage is treated as total rather than per-pixel, which is conservative
 *    in the same direction as case 1.
 *
 * 4. TEXT THAT IS NOT VISIBLE. Excluded: no layout box, `visibility` other than visible,
 *    an effective opacity of zero, and a box that lies entirely off the left, top or right of
 *    the page. Text BELOW the fold is kept — a phone shows it by scrolling, which is reading,
 *    not hiding. Text merely COVERED by something — the page behind an open sheet, under the
 *    scrim — is measured against its own painting stack rather than through the scrim,
 *    because the dimmed state is transient and nobody reads in it.
 *    ⚠️ That choice runs in the LAX direction and the whole weight is on the argument above,
 *    not on any safety property. Compositing two colours through a common overlay moves both
 *    toward the overlay, so the ratio moves toward 1: `--ink` on `--panel` is 14.6:1 in the
 *    open and about 2.0:1 through `--scrim`. An earlier version of this comment claimed the
 *    opposite — "stricter, never laxer" — which was simply false, and a comment that carries
 *    an argument owes the same correctness as one that carries a number.
 *
 * 5. ELEMENTS THAT RENDER NO TEXT. Only DIRECT child text nodes count, so a wrapper is never
 *    charged for the colour of a child's run and no run is measured twice.
 *
 * ---------------------------------------------------------------------------------------
 * How it gets its answer out: the browser is driven with --dump-dom, so the only channel is
 * the serialised document. The payload goes in a <script type="application/json"> as base64,
 * because an HTML serialisation escapes text and stops at a literal `</script`, and base64
 * survives both without a quoting rule anyone will remember. See browsertest.Probe. */
(function () {
  'use strict';

  var OUT_ID = 'browsertest-probe';

  /* Chromium serialises computed colours as rgb()/rgba(). Anything else is a shape this
     probe has not been taught, and it throws rather than guessing — a guessed colour is a
     wrong ratio that reads exactly like a right one. */
  function parseColor(s) {
    s = (s || '').trim();
    if (s === '' || s === 'none' || s === 'transparent') return [0, 0, 0, 0];
    var m = /^rgba?\(([^)]*)\)$/i.exec(s);
    if (!m) throw new Error('unparseable colour: ' + s);
    var parts = m[1].split(/[\s,/]+/).filter(function (x) { return x !== ''; });
    if (parts.length < 3) throw new Error('unparseable colour: ' + s);
    var num = function (v) { return v.slice(-1) === '%' ? parseFloat(v) * 2.55 : parseFloat(v); };
    var a = 1;
    if (parts.length > 3) {
      a = parts[3].slice(-1) === '%' ? parseFloat(parts[3]) / 100 : parseFloat(parts[3]);
    }
    return [num(parts[0]), num(parts[1]), num(parts[2]), a];
  }

  function over(top, bottom) {
    var a = top[3] + bottom[3] * (1 - top[3]);
    if (a === 0) return [0, 0, 0, 0];
    var c = function (i) {
      return (top[i] * top[3] + bottom[i] * bottom[3] * (1 - top[3])) / a;
    };
    return [c(0), c(1), c(2), a];
  }

  function luminance(c) {
    var f = function (v) {
      v = v / 255;
      return v <= 0.04045 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
    };
    return 0.2126 * f(c[0]) + 0.7152 * f(c[1]) + 0.0722 * f(c[2]);
  }

  function ratio(a, b) {
    var la = luminance(a), lb = luminance(b);
    var hi = Math.max(la, lb), lo = Math.min(la, lb);
    return (hi + 0.05) / (lo + 0.05);
  }

  function hex(c) {
    var h = function (v) {
      var s = Math.max(0, Math.min(255, Math.round(v))).toString(16);
      return s.length === 1 ? '0' + s : s;
    };
    return '#' + h(c[0]) + h(c[1]) + h(c[2]);
  }

  var unmeasured = [];
  function cannotMeasure(what, why) { unmeasured.push(why + ': ' + what); }

  function describe(el) {
    var s = el.tagName.toLowerCase();
    if (el.id) s += '#' + el.id;
    if (el.classList.length) s += '.' + Array.prototype.join.call(el.classList, '.');
    return s;
  }

  function path(el) {
    var parts = [], n = el, depth = 0;
    while (n && depth < 3) { parts.unshift(describe(n)); n = n.parentElement; depth++; }
    return parts.join(' > ');
  }

  /* What one element paints behind its own content, as a list of candidates — more than one
     only when a gradient means more than one answer is true. `mul` is the opacity of this
     element and its ancestors; see case 2. */
  function candidates(cs, mul, what) {
    var base = parseColor(cs.backgroundColor);
    base = [base[0], base[1], base[2], base[3] * mul];
    var img = cs.backgroundImage;
    if (!img || img === 'none') return base[3] === 0 ? null : [base];

    var stops = [], m, re = /rgba?\([^)]*\)/gi;
    while ((m = re.exec(img)) !== null) stops.push(parseColor(m[0]));
    if (stops.length === 0) {
      /* A url() or a named-colour gradient: this instrument cannot say what is behind the
         text, and says so rather than falling back to the background colour, which would be
         a confident wrong answer. */
      cannotMeasure(what, 'background-image with no rgb() stops');
      return base[3] === 0 ? null : [base];
    }
    var out = stops.map(function (s) {
      return over([s[0], s[1], s[2], s[3] * mul], base);
    });
    /* The gradient need not cover the whole box — background-size and background-position
       are free — so the plain background colour stays a candidate. */
    out.push(base);
    out = out.filter(function (c) { return c[3] > 0; });
    return out.length ? out : null;
  }

  /* Layers that paint above the canvas background and below everything in flow. Collected
     once: the element with a negative z-index, everything inside it, and their
     ::before/::after — `.lbg`'s glow lives entirely in two pseudo-elements. */
  var MAX_BACKDROP_LAYERS = 12;
  function collectBackdrops() {
    var roots = [], all = document.querySelectorAll('*');
    for (var i = 0; i < all.length; i++) {
      var z = parseInt(getComputedStyle(all[i]).zIndex, 10);
      if (!isNaN(z) && z < 0) roots.push(all[i]);
    }
    var layers = [];
    roots.forEach(function (root) {
      var members = [root].concat(Array.prototype.slice.call(root.querySelectorAll('*')));
      members.forEach(function (el) {
        /* Stops at body: body's and html's own opacity is applied by their layers in the
           text's own chain, and counting it here as well would fade the backdrop twice. */
        var mul = 1;
        for (var n = el; n && n.tagName !== 'BODY' && n.tagName !== 'HTML'; n = n.parentElement) {
          mul *= parseFloat(getComputedStyle(n).opacity);
        }
        [null, '::before', '::after'].forEach(function (pseudo) {
          var cs = getComputedStyle(el, pseudo);
          if (pseudo && (cs.content === 'none' || cs.content === 'normal')) return;
          var cands = candidates(cs, mul, 'backdrop ' + describe(el) + (pseudo || ''));
          if (cands) layers.push({ root: root, cands: cands });
        });
      });
    });
    if (layers.length > MAX_BACKDROP_LAYERS) {
      throw new Error('more backdrop layers (' + layers.length + ') than this probe was ' +
        'designed for; the cross-product below is no longer affordable and the page needs ' +
        'a look rather than a bigger constant');
    }
    /* Later in document order paints on top. */
    return layers.reverse();
  }

  function coversEverything(el) {
    var cs = getComputedStyle(el), r = el.getBoundingClientRect();
    return cs.position === 'fixed' && r.left <= 0 && r.top <= 0 &&
      r.right >= document.documentElement.clientWidth &&
      r.bottom >= document.documentElement.clientHeight;
  }

  function intersects(a, b) {
    return a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;
  }

  var TRANSPARENT = [0, 0, 0, 0];
  var WHITE = [255, 255, 255, 1];

  /* The painting stack under a run: one entry per thing that paints between the glyphs and
     the page, ordered FROM THE TEXT OUTWARD.
   *
   * `cands` is what that entry paints — more than one only where a gradient means more than
   * one answer is true. `fade` is the element's own `opacity`, and where it sits in the walk
   * is the whole of case 2.
   *
   * ⚠️ `opacity` is NOT a per-layer multiplier, and treating it as one charges an ancestor's
   * opacity to the text twice. It GROUPS: the element and its subtree are rendered into a
   * buffer, and the buffer is composited once, at that opacity, over whatever is beneath the
   * element. So the fold applies `fade` to everything accumulated SO FAR at the moment it
   * leaves that element, and to nothing below it. For a group at opacity a over a page P, the
   * glyph really lands at a·text + (1−a)·P; the per-layer reading gives a·text + (1−a)·bg′
   * with bg′ already faded, which is a different and wrong colour. */
  var MAX_COMBINATIONS = 512;
  function stackFor(el, backdrops) {
    var chain = [], flow = [], canvas = [];
    for (var n = el; n; n = n.parentElement) chain.push(n);

    for (var j = 0; j < chain.length; j++) {
      var cs = getComputedStyle(chain[j]);
      var layer = {
        cands: candidates(cs, 1, describe(chain[j])) || [TRANSPARENT],
        fade: parseFloat(cs.opacity)
      };
      /* body and html are held back: their background is what propagates to the CANVAS,
         which is painted BENEATH the backdrop layers rather than with the rest of the chain.
         Recognised by tag rather than by position, because a transparent body would
         otherwise leave the wrong entries at the end. */
      var tag = chain[j].tagName;
      (tag === 'BODY' || tag === 'HTML' ? canvas : flow).push(layer);
    }

    var rect = el.getBoundingClientRect();
    var below = [];
    backdrops.forEach(function (b) {
      if (coversEverything(b.root) || intersects(b.root.getBoundingClientRect(), rect)) {
        /* A backdrop is not an ancestor of the text, so its own opacity really is a plain
           multiplier on what it paints — already applied in collectBackdrops. */
        below.push({ cands: b.cands, fade: 1 });
      }
    });

    return prune(flow.concat(below, canvas), el);
  }

  /* Drop what cannot affect the answer: layers that paint nothing and never fade, and
     everything below the first opaque layer that no later fade can reopen. Without this the
     landing page's backdrop stops would be enumerated under every run on the page, including
     the ones sitting on an opaque card that hides the backdrop completely. */
  function prune(layers, el) {
    var fadesBelow = new Array(layers.length + 1);
    fadesBelow[layers.length] = true;
    for (var i = layers.length - 1; i >= 0; i--) {
      fadesBelow[i] = fadesBelow[i + 1] && layers[i].fade === 1;
    }
    var cut = layers.length;
    for (var k = 0; k < layers.length; k++) {
      if (layers[k].cands.length === 1 && layers[k].cands[0][3] >= 1 && fadesBelow[k]) {
        cut = k + 1;
        break;
      }
    }
    var kept = [];
    for (var m = 0; m < cut; m++) {
      var l = layers[m];
      if (l.fade === 1 && l.cands.length === 1 && l.cands[0][3] === 0) continue;
      kept.push(l);
    }

    var combos = kept.reduce(function (a, l) { return a * l.cands.length; }, 1);
    if (combos > MAX_COMBINATIONS) {
      throw new Error('the painting stack under ' + describe(el) + ' has ' + combos +
        ' worst-case combinations; this probe refuses to guess which one to try');
    }
    return kept;
  }

  /* Composite `seed` down through the stack under one choice of gradient stops. */
  function fold(seed, layers, choice) {
    var acc = seed;
    for (var i = 0; i < layers.length; i++) {
      acc = over(acc, choice[i]);
      var f = layers[i].fade;
      if (f < 1) acc = [acc[0], acc[1], acc[2], acc[3] * f];
    }
    return acc;
  }

  /* The lowest ratio any pixel of this run can have, over every combination of the gradient
     stops in its stack. The SAME choice feeds both folds, because the glyph and the pixel
     beside it sit over the same gradient. */
  function worstRatio(text, layers) {
    var worst = Infinity, worstBg = null, worstFg = null, sawWhiteBackstop = false;
    var choice = new Array(layers.length);

    var enumerate = function (i) {
      if (i === layers.length) {
        var bg = fold(TRANSPARENT, layers, choice);
        var fg = fold(text, layers, choice);
        /* WHITE is the last resort and never the expected answer: reaching it means the walk
           came out of the top of the document without meeting an opaque background. */
        if (bg[3] < 1) {
          sawWhiteBackstop = true;
          bg = over(bg, WHITE);
          fg = over(fg, WHITE);
        }
        var r = ratio(fg, bg);
        if (r < worst) { worst = r; worstBg = bg; worstFg = fg; }
        return;
      }
      layers[i].cands.forEach(function (c) { choice[i] = c; enumerate(i + 1); });
    };
    enumerate(0);
    return { ratio: worst, bg: worstBg, fg: worstFg, white: sawWhiteBackstop };
  }

  var runs = [], exempt = [];
  function skip(reason, el, text) {
    exempt.push({ reason: reason, at: path(el), text: text.slice(0, 40) });
  }

  /* A run made only of separator glyphs is a boundary that happens to be drawn as a
     character — the `·` between two links, a `•`, a `|`. WCAG 1.4.11 exempts a decorative
     boundary, and this project has already declined to light every hairline for the reason
     that the page gets louder and the ask was the opposite. Same reasoning, same answer, and
     it is written here rather than accumulating as a list of selectors nobody revisits. The
     `·` in `.gatefoot .dotsep` is 1.44:1; flagging it would be the crying-wolf failure that
     earns a rule a blanket exemption list.
   *
   * ⚠️ It is an ALLOWLIST of three glyphs and not "anything without a letter or a digit",
   * which is where this started and which was much too wide. That version exempted the
   * close control's only label (`✕`), the price column's heading (`£`), the transfer
   * arithmetic (`+`, `=`) and the null marker (`—`) — four runs that carry meaning, none of
   * which anyone would have noticed going dim. All four pass today; they now pass measured
   * rather than by not being looked at. */
  var SEPARATOR_ONLY = /^[\s·•|]+$/;

  var DISABLED = 'button:disabled,input:disabled,select:disabled,textarea:disabled,' +
    'fieldset:disabled,[aria-disabled="true"]';

  function measure(el, cs, text, pseudo, backdrops) {
    if (SEPARATOR_ONLY.test(text)) { skip('a boundary drawn as a glyph, not text', el, text); return; }
    if (el.closest(DISABLED)) { skip('an inactive control; WCAG exempts one', el, text); return; }

    var layers = stackFor(el, backdrops);
    if (pseudo) {
      /* A pseudo-element paints its OWN background, inside the host and above the host's.
         `.card.isvc .shirt::after` — the vice-captain's badge — is `--ink2` on `--panel3`
         inside a shirt whose background app.js sets to the club colour, so scoring it
         against the host's background reads 1.11:1 for a badge that renders at 6.55:1. */
      var own = candidates(cs, 1, describe(el) + pseudo);
      layers = [{ cands: own || [TRANSPARENT], fade: parseFloat(cs.opacity) }].concat(layers);
    }

    var color = parseColor(cs.color);
    if (color[3] === 0) { skip('the text itself is transparent', el, text); return; }
    /* An opacity of zero anywhere above the run makes the glyphs and their background the
       same colour, which is a ratio of 1 and would be a loud false failure. It is not text
       anyone can read, so it is not text this measures. */
    var seen = layers.reduce(function (a, l) { return a * l.fade; }, 1);
    if (seen <= 0.01) { skip('faded to nothing by an opacity above it', el, text); return; }

    var size = parseFloat(cs.fontSize);
    var weight = parseInt(cs.fontWeight, 10);
    if (isNaN(weight)) weight = /bold/i.test(cs.fontWeight) ? 700 : 400;

    var w = worstRatio(color, layers);
    if (w.white) cannotMeasure(path(el), 'the painting stack ran off the top of the document');

    runs.push({
      at: path(el) + (pseudo || ''),
      text: text.replace(/\s+/g, ' ').trim().slice(0, 48),
      fg: hex(w.fg),
      bg: hex(w.bg),
      size: size,
      weight: weight,
      ratio: Math.round(w.ratio * 1000) / 1000
    });
  }

  function run() {
    var backdrops = collectBackdrops();
    var vw = document.documentElement.clientWidth;
    var all = document.body.querySelectorAll('*');

    for (var i = 0; i < all.length; i++) {
      var el = all[i];
      /* Not an HTML element: SVG paints its glyphs with `fill`, which is a different
         property from `color` and a different question from this one. */
      if (el.namespaceURI !== 'http://www.w3.org/1999/xhtml') continue;
      if (/^(script|style|noscript|template|option|title)$/i.test(el.tagName)) continue;

      var cs = getComputedStyle(el);
      if (cs.visibility !== 'visible') continue;

      var rect = null, rects = el.getClientRects();
      for (var k = 0; k < rects.length; k++) {
        if (rects[k].width > 0 && rects[k].height > 0) { rect = rects[k]; break; }
      }
      if (!rect) continue;
      /* Off the left, off the top, or off the right edge of the page. NOT off the bottom:
         a phone reaches that text by scrolling, and scrolling is reading.

         The asymmetry between the two axes is deliberate, not an oversight. Down is the
         page's own scroll and everything below the fold is reachable; sideways it is not,
         because `body` is `overflow-x:hidden`, so a box to the right of the viewport is
         clipped rather than scrolled to — and `left:-9999px`, `left:110%` and a sheet parked
         off-canvas are all the same idiom for "not on screen". MEASURED both ways: dropping
         the right-hand clause adds 2 runs across the five phone screens and finds nothing
         new, so it buys no coverage and gives up the off-canvas idiom. */
      if (rect.right <= 0 || rect.bottom <= 0 || rect.left >= vw) continue;

      var direct = '';
      for (var c = 0; c < el.childNodes.length; c++) {
        if (el.childNodes[c].nodeType === 3) direct += el.childNodes[c].nodeValue;
      }
      direct = direct.trim();
      if (direct) measure(el, cs, direct, null, backdrops);

      /* Generated text is text: `.card.isvc .shirt::after` is the vice-captain's badge, and
         nothing else in this repository can see it at all. */
      var pseudos = ['::before', '::after'];
      for (var q = 0; q < pseudos.length; q++) {
        var ps = getComputedStyle(el, pseudos[q]);
        var content = ps.content;
        if (!content || content === 'none' || content === 'normal') continue;
        var quoted = /^"(.*)"$/.exec(content);
        if (!quoted) continue;   /* counters, attr(), url() — not a literal run of text */
        var t = quoted[1].trim();
        if (t) measure(el, ps, t, pseudos[q], backdrops);
      }

      /* A placeholder is text the reader is asked to read, and it is the one run in this
         design that is styled by a pseudo-element whose content comes from an attribute
         rather than from CSS. `.gateform input::placeholder` is `--ink3` on `--bg2`, which
         is precisely the pairing that broke. */
      if (el.tagName === 'INPUT' && el.getAttribute('placeholder')) {
        measure(el, getComputedStyle(el, '::placeholder'),
          el.getAttribute('placeholder'), '::placeholder', backdrops);
      }
    }
  }

  /* --dump-dom serialises the MAIN frame only, and this page is normally inside a frame —
     the frame is how a true 390x844 viewport is obtained at all, since the window itself has
     a 500px floor. So the answer is written into the parent document, which is same-origin
     because the test server serves both. */
  function publish(payload) {
    var doc = document;
    try {
      if (window.parent && window.parent !== window && window.parent.document) {
        doc = window.parent.document;
      }
    } catch (e) { /* cross-origin: publish here and let the test say it found nothing */ }
    var el = doc.createElement('script');
    el.type = 'application/json';
    el.id = OUT_ID;
    el.textContent = btoa(unescape(encodeURIComponent(JSON.stringify(payload))));
    doc.body.appendChild(el);
  }

  /* The app fetches /api/state and renders from the answer, so there is nothing to measure
     at `load`. --virtual-time-budget holds the virtual clock while a fetch is outstanding,
     so a virtual two seconds after load is after the render rather than a race with it. */
  window.addEventListener('load', function () {
    setTimeout(function () {
      try {
        run();
        publish({
          url: location.href, width: window.innerWidth, height: window.innerHeight,
          runs: runs, exempt: exempt, unmeasured: unmeasured
        });
      } catch (err) {
        publish({ url: location.href, error: String((err && err.stack) || err) });
      }
    }, 2000);
  });
})();
