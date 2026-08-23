/* FPL Armband — the projection card, shared by every tab of /wildcard.
 *
 * Draws "what a chip would buy", never "what happened": no points, no bonus, no
 * badges, no live dot, no lock/leave-out controls — the tense this page owns is
 * "would be", and results.js/app.js's cards belong to the other two tenses (see
 * the design note this implements, §2's governing rule). Every number here
 * arrives already decided, off analysis.WeekView through viewmodel.ChipTeam;
 * this module computes nothing and derives no gameweek state.
 *
 * ⚠️ This module must never learn which page it is on. No isWildcard, no
 * opts.chip, no opts.mode. The difference between the wildcard tab and the
 * free-hit tab is expressed by wildcard.js — by which half of the response it
 * passes in and what copy it wraps around the result — never by a parameter
 * here. opts on card() carries facts about the PLAYER'S ROLE IN THIS FIFTEEN
 * (isCaptain, isVice, isBench, isNew), the same discipline results.js's card()
 * already keeps for isCaptain/isVice/isBench.
 *
 * Vanilla DOM, no framework, no build step, same as results.js and team.js.
 */
'use strict';

(function () {

const CLUBC={TOT:'#132257',LIV:'#C8102E',NEW:'#241F20',MUN:'#DA291C',MCI:'#6CABDD',
  EVE:'#003399',CHE:'#034694',SUN:'#EB172B',BRE:'#E30613',BHA:'#0057B8',COV:'#78D0F3',
  FUL:'#CC0000',ARS:'#EF0107',AVL:'#95BFE5',BOU:'#DA291C',NFO:'#DD0000',CRY:'#1B458F',
  IPS:'#0044A9',LEE:'#FFCD00',WOL:'#FDB913'};

function esc(v){
  return String(v==null?'':v).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
}

/* oppChip draws the one fixture this gameweek carries for a player, in the same
   shape results.js's fxrChip uses -- reusing the .fdr convention rather than
   inventing a second one. p.fixtures is NOT the model's forward-looking window
   here: buildChipTeam overwrites it, server-side, with exactly WeekView.
   Opponents[id] for this one gameweek (see viewmodel.buildChipTeam's own
   comment on the opponent trap) -- so fixtures[0] is this week's match, or
   absent for a blanking club, and there is no fallback. */
function oppChip(p){
  const f = p.fixtures && p.fixtures[0];
  if(!f) return '<i class="blank" title="No fixture this gameweek">–</i>';
  const at = f.home ? '' : '@';
  return `<i class="f${f.fdr}" title="${esc(f.opp)}, ${f.home?'home':'away'}">${at}${esc(f.opp)}</i>`;
}

/* availabilityBox is the FPL-news corner glyph, the same three-severity read
   app.js's cardHtml uses for the interactive builder: --flag-doubt for any
   doubt, --warn for the middle band FPL owned alone before the split, --bad
   for ruled out. Reused here because a card omitting availability is exactly
   the trap this page's own design note names: a wildcard that quietly fields
   a 50%-fit player with no mark is a worse fifteen than the copy admits to. */
function availabilityBox(p){
  const av = p.availability===undefined ? 1 : p.availability;
  if(av >= 1) return '';
  const cls = av===0 ? 'bad' : av>0.5 ? '' : 'warn';
  const txt = av===0 ? 'OUT' : Math.round(av*100)+'%';
  const title = (av===0?'Ruled out':Math.round(av*100)+'% fit') + ' — see News';
  return `<span class="newsflag${cls?' '+cls:''}" title="${esc(title)}">${txt}</span>`;
}

/* card draws one projection card. opts.isNew marks a player not in the house
   fifteen today -- computed by the caller from ChipTeam.KeptIDs, never here,
   since whether that mark is worth showing at all is wildcard.js's call (a
   free hit's fifteen is almost entirely "new" every week by construction, and
   that is the design's own reason the mark is only interesting on the
   wildcard tab). */
function card(p, opts){
  opts = opts || {};
  const isCaptain = !!opts.isCaptain, isVice = !!opts.isVice, isBench = !!opts.isBench;
  const club = CLUBC[p.club] || '#4C6072';
  const band = isCaptain ? '<span class="cpband c">C</span>'
             : isVice ? '<span class="cpband v">V</span>' : '';
  const cls = [isCaptain&&'iscap', isVice&&'isvc', isBench&&'bench', opts.isNew&&'isnew']
    .filter(Boolean).join(' ');
  return `
  <div class="cpcard ${cls}">
    <div class="cpbar" style="background:${club}"></div>
    ${band}
    ${availabilityBox(p)}
    <div class="cpbody">
      <div class="cpname">${esc(p.name)}${opts.isNew ? ' <span class="cpnew">NEW</span>' : ''}</div>
      <div class="cpmeta">
        <span class="cpclub">${esc(p.club)}</span>
        <span class="cpopp fdr">${oppChip(p)}</span>
      </div>
      <div class="cpxp"><span class="cpprice">£${p.price.toFixed(1)}m</span><b>${p.xp.toFixed(1)}</b></div>
    </div>
  </div>`;
}

/* pitch lays out one chip's rebuilt fifteen: formation, the XI by line, the
   bench. t is a viewmodel.ChipTeam -- the decided shape this function reads
   and nothing more. isNew is read off t.kept_ids, the account's real fifteen
   intersected with this rebuild server-side (see buildChipTeam), never
   re-derived here. */
function pitch(t){
  const kept = new Set(t.kept_ids||[]);
  const byPos = {GKP:[], DEF:[], MID:[], FWD:[]};
  for(const p of t.xi||[]) (byPos[p.pos]||(byPos[p.pos]=[])).push(p);
  const rows = ['GKP','DEF','MID','FWD'].map(pos => {
    const players = byPos[pos]||[];
    if(!players.length) return '';
    return `<div class="line" data-pos="${pos}">
      ${players.map(p => card(p, {
        isCaptain: p.id===t.captain, isVice: p.id===t.vice, isNew: !kept.has(p.id),
      })).join('')}
    </div>`;
  }).join('');
  const bench = (t.bench||[]).map(p => card(p, {isBench:true, isNew: !kept.has(p.id)})).join('');

  return `
    <div class="chipformation">
      <span class="k">Formation</span>
      <span class="form">${esc(t.formation||'—')}</span>
    </div>
    <div class="pitch">${rows}</div>
    ${bench ? `<div class="chipbench">
      <span class="k">Bench</span>
      <div class="chipbenchrow">${bench}</div>
    </div>` : ''}
  `;
}

/* header is the strip's stat half: how much of today's fifteen this rebuild
   changes, what it was allowed to spend, and what it is expected to return —
   see the design note's own warning that the two tabs' expected-points
   figures are comparable numbers built to different objectives, which is why
   the objective clause travels WITH the number rather than living once at the
   top of the page. t carries budget/budget_source merged in by wildcard.js,
   since those are ChipTeams-level facts, not per-chip ones — this function
   still just reads what it is handed. */
function header(t){
  const changes = t.changes || 0;
  const budget = t.budget!=null ? `£${t.budget.toFixed(1)}m to spend` : '';
  return `
    <span class="chipstat"><b>${changes}/15</b> change</span>
    ${budget ? `<span class="chipstat">${esc(budget)}</span>` : ''}
    <span class="chipstat"><b>${t.expected.toFixed(1)}</b> pts expected</span>
  `;
}

window.ArmbandProjection = { header, pitch, card };

})();
