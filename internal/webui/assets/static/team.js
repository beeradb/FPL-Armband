/* FPL Armband — the spectator team page (/armband-team).
 *
 * Deliberately its own small script, not a mode of app.js. app.js is the interactive
 * builder: drag-and-drop, locks, leave-outs, the captain picker, a session. None of that
 * exists here -- this document has no reader input at all, so it has no state object and
 * no event wiring beyond one fetch.
 *
 * The results card, the pitch and the scoreboard ARE shared now, in results.js, but that is
 * not the drift this file used to warn against. What was refused was a renderer with a
 * page-identity switch -- one #if for "is this the read-only page", growing a second, the
 * two surfaces silently converging around them. ArmbandResults.scoreboard/pitch/card take
 * none of that: they take decided data (and, for card, facts about a PLAYER's role in the
 * squad -- isCaptain, isVice, isBench) and return markup, with no branch anywhere on which
 * page called them. This file still owns everything that WOULD differ between the two
 * surfaces -- the fetch, which elements it writes into, what an error looks like here -- so
 * the two scripts can keep drawing the same card without becoming the same script. If a
 * future difference between the surfaces ever needs expressing, it belongs here, in what
 * this file passes in or wraps around the result, never as a new parameter on the shared
 * component.
 *
 * This page's tense is "what happened", never "what might happen" -- see results.js's
 * cardState and scoreboard. Every number here arrives from /api/armband-team already
 * decided; this script computes no model quantity and derives no gameweek state (see
 * internal/viewmodel.Results.ResultState's own comment for why that one lives
 * server-side, in cmd/armband.houseLiveSources, and not here).
 *
 * Vanilla DOM, no framework, no build step, same as app.js.
 */
'use strict';

fetch('/api/armband-team', {credentials:'same-origin'})
  .then(r => { if(!r.ok) throw new Error(`the server answered ${r.status}`); return r.json(); })
  .then(st => {
    const ht = st.results;
    const houseEl = document.getElementById('houseteam');
    const pitchEl = document.getElementById('teampitch');
    if(!ht){
      if(houseEl) houseEl.innerHTML='';
      if(pitchEl) pitchEl.innerHTML='<div class="panel" style="padding:24px"><b>No team configured.</b></div>';
      return;
    }
    // Names the saved copy in the one place a crawler never reads and every browser
    // surface does: the tab, the bookmark, the history entry and "Save Page As" all read
    // document.title, not og:title (see team.html for why the OG card stays generic).
    //
    // "FPL Armband's Team", not "our team" -- once a reader has imported their own squad
    // into /app, "our" reads as ambiguous (whose?), the same defect the owner flagged on
    // the nav link and the pill on this page's own <title> (see team.html). 2026-08-22.
    if(ht.result_event) document.title = `Gameweek ${ht.result_event} — FPL Armband’s Team`;
    if(houseEl) houseEl.innerHTML = ArmbandResults.scoreboard(ht);
    if(pitchEl) pitchEl.innerHTML = ArmbandResults.pitch(ht);
  })
  .catch(err => {
    const el=document.getElementById('teampitch');
    if(!el) return;
    el.innerHTML=`<div class="panel" style="padding:24px">
      <b>The team could not be loaded.</b>
      <div class="dim" style="margin-top:8px"></div>
    </div>`;
    el.querySelector('.dim').textContent = err.message;
  });
