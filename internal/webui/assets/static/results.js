/* ArmbandResults — the shared results-card component.
 *
 * Pure rendering: no fetch, no state object, no event wiring. It draws one thing, a squad's
 * results for a gameweek, from data that has already been decided elsewhere -- see team.js's
 * own comment for why this component is safe to share while a page-mode switch would not be.
 *
 * Its tense is "what happened", never "what might happen" -- see cardState and scoreboard.
 * Every number it draws arrives already decided; this module computes no model quantity and
 * derives no gameweek state (see internal/viewmodel.Results.ResultState's own comment for
 * why that one lives server-side, in cmd/armband.houseLiveSources, and not here).
 *
 * Vanilla DOM, no framework, no build step, same as app.js and gate.js. Exposed as one
 * global, following gate.js's window.wireGateForms precedent.
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

/* plural picks the singular or plural noun for a badge title. Every badge whose count can
   legitimately be 1 (goals, assists, penalties, own goals, a pending defensive-action
   count) is routed through this so a tooltip never reads "1 goals" -- the two badges that
   cannot fire at n=1 by their own threshold (SV needs 3, GC needs 2) never call it. */
function plural(n, singular, pluralForm){
  return n === 1 ? singular : (pluralForm || singular + 's');
}

/* fxrChip draws one opponent chip in the SAME shape the interactive builder's fixture
   ribbon uses -- home is bare, away is "@", colour is difficulty and nothing else. Reusing
   the convention rather than inventing a second one for this page. */
function fxrChip(opp){
  if(!opp) return '<i class="blank" title="No fixture this gameweek">–</i>';
  const at = opp.home ? '' : '@';
  return `<i class="f${opp.fdr}" title="${esc(opp.opp)}, ${opp.home?'home':'away'}">${at}${esc(opp.opp)}</i>`;
}

/* liveDot marks a club's fixture as in progress right now -- fpl.Fixture.Started &&
   !FinishedProvisional && !Finished, computed server-side (see
   cmd/armband.houseLiveSources). Renders ONLY for 'live', never 'fulltime': a match that
   has been played out and locked in is not "in progress", even though FPL has not yet
   confirmed its bonus (see match_status's own three-way split, added 2026-08-22) -- an
   ended match still carrying this dot was the defect that split 'fulltime' out of 'live'.
   Sits next to the opponent chip because that is where a reader is already looking to
   place him. */
function liveDot(status){
  return status==='live' ? '<span class="tplive" title="In progress"></span>' : '';
}

/* cardState is the single source of truth for which of the four results-card states a
   player is in, decided from match_status and minutes ALONE. Every other rendering
   decision below -- the points slot, whether a badge row exists at all, the card's colour
   -- reads this value rather than re-deriving it from some other field. That is what lets
   an absent badge mean one thing only ("he did not do it"): "not yet known" is state
   'toplay', which never reaches badgeHtml at all, so nothing downstream can confuse the
   two.

   'fulltime' (added 2026-08-22, see match_status's own comment) deliberately maps into
   the SAME four states rather than growing a fifth: the total genuinely can still move
   (bonus is not settled), so it draws everything 'live' draws -- the points figure, the
   badge row, the asterisk -- and must not fall through to 'toplay'. The one place it does
   NOT behave like 'live' is the dot beside the opponent chip, and that is liveDot's job,
   not this function's: liveDot reads match_status directly, so it already tells 'live'
   and 'fulltime' apart without cardState needing a fifth value to do it.

   The dnp rule gets the same "full time is knowable" treatment as everything else here:
   at 'finished' zero minutes is DNP, and at 'fulltime' it is too, deliberately -- the
   match is over either way, so "did not play" is already a fact, not a placeholder for
   one. */
function cardState(p){
  if(p.match_status === 'fulltime') return p.minutes > 0 ? 'live' : 'dnp';
  if(p.match_status === 'live') return 'live';
  if(p.match_status === 'finished') return p.minutes > 0 ? 'played' : 'dnp';
  return 'toplay'; // "scheduled", or empty before a season has a current gameweek at all
}

/* badgeHtml is the complete badge vocabulary, in one fixed order (positives left,
   negatives right, never re-sorted, so position is learnable) and one rule: a badge
   renders iff FPL paid or docked points for that event and the count is non-zero.
   Everything below follows from that sentence.

   Absent outside 'live'/'played' on purpose -- states 'toplay' and 'dnp' return no row at
   all rather than an empty one, because a badge row on a match that has not been played
   would have nothing honest to assert. */
function badgeHtml(p, state){
  if(state !== 'live' && state !== 'played') return '';
  const rows = [];
  const push = (cls, label, n, title) => rows.push(
    `<span class="tpbadge ${cls}" title="${esc(title)}">${n!=null?`<b>${n}</b>`:''}${label}</span>`);

  if(p.goals > 0) push('pos','G',p.goals,`${p.goals} ${plural(p.goals,'goal')}`);
  if(p.assists > 0) push('pos','A',p.assists,`${p.assists} ${plural(p.assists,'assist')}`);
  // CS excludes forwards because FPL pays a forward nothing for one. It includes
  // midfielders (FPL pays 1 point) -- the rule is "FPL paid", not "FPL paid a lot".
  if(p.clean_sheets > 0 && p.pos !== 'FWD') push('pos','CS',null,'Clean sheet');
  // SV needs 3: FPL pays 1 point per 3 saves, so two saves is worth zero. The one badge
  // where the raw count and the paying threshold diverge, and the rule follows the points.
  if(p.pos === 'GKP' && p.saves >= 3) push('pos','SV',p.saves,`${p.saves} ${plural(p.saves,'save')}`);
  if(p.penalties_saved > 0)
    push('pos','PS',p.penalties_saved,`${p.penalties_saved} ${plural(p.penalties_saved,'penalty saved','penalties saved')}`);
  // DC loses its count and loses red once a match is finished: the bar is cleared or it
  // is not, and a red pill would be a verdict where the absent badge already says "he
  // earned nothing here". The count survives in cardState 'live', shown neutral
  // (--ink3/.pend) there. ⚠️ That state now covers 'fulltime' too (2026-08-22, see
  // cardState's own comment), where the count is no longer "progress toward a bar" --
  // full time has been reached, so a card in this branch there is already the final
  // answer, just displayed with the same not-yet-a-verdict styling 'live' uses.
  // Deliberate: splitting a fifth visual state out for it is a decision beyond the four
  // defects this change fixes, not an oversight.
  // The two conditions are mutually exclusive by construction (def_con_reached is only
  // ever non-null once his match has kicked off).
  if(p.def_con_reached === true){
    push('pos','DC',null,'Cleared the defensive contribution bar');
  } else if(state === 'live' && p.def_con != null && p.def_con_reached === false){
    push('pend','DC',p.def_con,`${p.def_con} ${plural(p.def_con,'defensive action')} — has not cleared the bar`);
  }
  if(p.penalties_missed > 0)
    push('neg','PM',p.penalties_missed,`${p.penalties_missed} ${plural(p.penalties_missed,'penalty missed','penalties missed')}`);
  if(p.own_goals > 0) push('neg','OG',p.own_goals,`${p.own_goals} ${plural(p.own_goals,'own goal')}`);
  // YC/RC carry no count -- a second yellow IS a red, and FPL reports the red.
  if(p.yellow_cards > 0) push('warn','YC',null,'Yellow card');
  if(p.red_cards > 0) push('neg','RC',null,'Red card');
  // GC needs 2 (FPL docks 1 point per 2 conceded, the SV arithmetic in the other
  // direction) and applies only to the two positions FPL docks at all.
  if((p.pos === 'GKP' || p.pos === 'DEF') && p.goals_conceded >= 2)
    push('neg','GC',p.goals_conceded,`${p.goals_conceded} goals conceded`);

  // min-height:14px (armband.css) reserves the row's rhythm even when rows is empty, so a
  // clean card does not sit shorter than a busy one in the same pitch row.
  return `<div class="tpbadges">${rows.join('')}</div>`;
}

/* bonusHtml is the ONE token that lives inside .tppts rather than .tpbadges -- bonus is
   already inside total_points, so if it rendered beside the badges a reader scanning the
   row would count it as a fourth thing that adds to the number above, and it does not, it
   IS part of that number. Position (inline, on the numeral's own line), weight/colour
   (grey, not a valence colour) and the title all say the same thing three ways.

   The figure shown is bonus × multiplier, same reasoning as the numeral: everything
   inside .tppts is expressed in the units of the number it qualifies. A bench player
   (multiplier 0) special-cases to the raw bonus, since his card shows his raw points too. */
function bonusHtml(p){
  if(!(p.bonus > 0)) return '';
  const disp = p.multiplier === 0 ? p.bonus : p.bonus * p.multiplier;
  const title = p.multiplier > 1
    ? `${disp} of these points are bonus (${p.bonus} bonus, doubled)`
    : `${p.bonus} of these points are bonus`;
  return `<span class="tpbon" title="${esc(title)}">${disp}B</span>`;
}

/* breakdownTitle turns p.breakdown -- computed server-side, see
   internal/viewmodel.TeamPlayer.Breakdown's own comment for why this script never derives
   it -- into the hover text for .tppts: one component per line, `title` honours \n. Empty
   string (so ptsHtml adds no title attribute at all) before anything has happened, or
   whenever the server withheld a disagreeing breakdown.

   The sign is rendered here, not server-side -- the same kind of display arithmetic
   ptsHtml already does for the doubled figure below, formatting a decided list rather
   than deriving one. */
function breakdownTitle(p){
  if(!p.breakdown || !p.breakdown.length) return '';
  return p.breakdown.map(l => {
    const sign = l.points < 0 ? '−' : '+';
    const detail = l.detail ? `  ${l.detail}` : '';
    return `${l.label}${detail}  ${sign}${Math.abs(l.points)}`;
  }).join('\n');
}

/* ptsHtml is the points slot -- three shapes for three facts, never alike, so a saved
   screenshot never reads one as another:
     'toplay' -- nothing is known, so nothing numeric renders. Not 0: a match that has not
                 kicked off has no score.
     'dnp'    -- DNP, a different STRING from "To play", not a different shade of the same
                 one. Colour alone would be too weak a carrier for the pair most damaging
                 to confuse.
     'live'/'played' -- the doubled figure (see card's own comment on the captain), an
                 asterisk while the match is still live OR played out but not yet settled
                 (see match_status's "fulltime"), or the settled bonus token once it is.

   The hover breakdown (Defect 4, 2026-08-22) is a `title` on the WHOLE slot, not on the
   numeral alone, so it is reachable wherever the figure renders -- 'live' and 'played'
   both get it, the two states that ever carry a non-empty Breakdown. It supplements the
   asterisk's own title (Defect 3's legend does the same, for touch); it does not replace
   it, and hovering the asterisk itself still shows its own narrower text, since a nested
   element's title wins over its ancestor's. */
function ptsHtml(p, state){
  if(state === 'toplay') return '<div class="tppts"><span class="tpstate">To play</span></div>';
  if(state === 'dnp') return '<div class="tppts"><span class="tpstate" title="Did not play">DNP</span></div>';
  // The captain's contribution to the team score, not his raw return -- see card.
  // Bench (multiplier 0) special-cases to the raw figure his card is actually reporting.
  const disp = p.multiplier === 0 ? p.points : p.points * p.multiplier;
  const num = `<b${disp<0?' class="neg"':''}>${disp}</b>`;
  const title = breakdownTitle(p);
  const attr = title ? ` title="${esc(title)}"` : '';
  if(state === 'live'){
    // The asterisk marks the NUMBER, not the bonus slot: while the match_status is
    // 'live' or 'fulltime' it is not merely the bonus that is unknown, it is the total,
    // which is still moving until match_status reaches 'finished' (FPL applies bonus at
    // the same moment it settles FinishedProvisional into Finished -- see
    // houseLiveSources).
    return `<div class="tppts"${attr}>${num}<i class="tpprov" title="Provisional — bonus is not settled">*</i></div>`;
  }
  return `<div class="tppts"${attr}>${num}${bonusHtml(p)}</div>`;
}

/* card draws one results card. opts carries facts about the PLAYER'S ROLE IN THE SQUAD for
   this gameweek -- isCaptain, isVice, isBench -- never a fact about which page is calling.
   See this file's own opening comment and team.js's for why that line is the whole point
   of this extraction.

   The band chip (C / 3× / V) is driven by Multiplier, NOT by id === captain: FPL's own
   picks response already resolves a captain-did-not-play week to the vice's multiplier, so
   reading multiplier is reading what actually happened rather than who was originally
   handed the armband -- and a Triple Captain week must render 3×, which id === captain
   could never tell apart from an ordinary double.

   isCaptain/isVice still decide which STATIC role the card is in the eleven for -- the V
   chip only ever shows for the vice, and only when he is not the one actually carrying the
   multiplier that week. The .iscap/.isvc classes on the card wrapper track whichever chip
   actually renders (not the static role) because their only job is reserving name padding
   for whatever badge is really there -- see the padding-right fix on .tpname. */
function card(p, opts){
  opts = opts || {};
  const isVice = !!opts.isVice;
  const isBench = !!opts.isBench;
  const club = CLUBC[p.club] || '#4C6072';
  const state = cardState(p);
  const showsC = p.multiplier >= 2;
  const showsV = !showsC && isVice;
  const band = showsC ? `<span class="tpband c">${p.multiplier===3?'3×':'C'}</span>`
             : showsV ? '<span class="tpband v">V</span>' : '';
  const cls = [showsC&&'iscap', showsV&&'isvc', isBench&&'bench',
    state==='live'&&'live', state==='dnp'&&'dnp'].filter(Boolean).join(' ');
  return `
  <div class="tpcard ${cls}">
    <div class="tpbar" style="background:${club}"></div>
    ${band}
    <div class="tpbody">
      <div class="tpname">${esc(p.name)}</div>
      <div class="tpmeta">
        <span class="tpclub">${esc(p.club)}</span>
        <span class="tpopp fdr">${fxrChip(p.opponent)}${liveDot(p.match_status)}</span>
      </div>
      ${ptsHtml(p, state)}
      ${badgeHtml(p, state)}
    </div>
  </div>`;
}

/* pitch lays out one gameweek's results: formation, the XI by line, the bench and its
   "left on the bench" note, and the provisional-scoring legend. r is the decided results
   object for a squad -- this function reads only the shape of that object, never which
   page asked for it. */
function pitch(r){
  const byPos = {GKP:[], DEF:[], MID:[], FWD:[]};
  for(const p of r.xi||[]) (byPos[p.pos]||(byPos[p.pos]=[])).push(p);
  const rows = ['GKP','DEF','MID','FWD'].map(pos => {
    const players = byPos[pos]||[];
    if(!players.length) return '';
    return `<div class="line" data-pos="${pos}">
      ${players.map(p => card(p, {isCaptain: p.id===r.captain, isVice: p.id===r.vice})).join('')}
    </div>`;
  }).join('');
  const bench = (r.bench||[]).map(p => card(p, {isCaptain: p.id===r.captain, isVice: p.id===r.vice, isBench:true})).join('');

  // The bench points line is the number every FPL manager looks for after a big bench
  // score. From history[last].bench_points, already parsed -- renders iff > 0, the same
  // "nothing to show" absence as everything else on this page.
  const hist = r.history||[];
  const last = hist.length ? hist[hist.length-1] : null;
  const benchNote = last && last.bench_points > 0
    ? `<span class="teambenchpts">${last.bench_points} pts left on the bench</span>` : '';

  // Defect 3, 2026-08-22: the owner asked what the asterisk meant, and its only
  // explanation was a `title`, which a touch device cannot reach at all (see
  // ptsHtml's own comment -- this legend supplements that title, it does not
  // replace it). Renders only when at least one card actually carries the mark --
  // cardState(p)==='live' is exactly ptsHtml's own asterisk condition, covering both
  // a genuinely in-progress match and an ended-but-unsettled 'fulltime' one, so a
  // fully settled gameweek shows no legend for a mark that is not on screen.
  const anyProvisional = [...(r.xi||[]), ...(r.bench||[])].some(p => cardState(p) === 'live');
  const legend = anyProvisional
    ? `<div class="teamlegend">* Still to be settled — FPL confirms bonus after full time.</div>`
    : '';

  return `
    <div class="teamformation">
      <span class="k">Formation</span>
      <span class="form">${esc(r.formation||'—')}</span>
    </div>
    <div class="pitch">${rows}</div>
    ${bench ? `<div class="teambench">
      <span class="k">Bench</span>${benchNote}
      <div class="teambenchrow">${bench}</div>
    </div>` : ''}
    ${legend}
  `;
}

/* scoreboard is the header row: a scoreboard for one completed gameweek, not a
   projection -- the old "GW{n} projected" cell paired the rail's Current gameweek with
   this squad's own score-bug figure, which are routinely different gameweeks the moment a
   deadline has passed but the rail has not rolled over (see viewmodel.Results's own
   comment). Removing the cell removes the bug; there is no replacement projection here on
   purpose.

   Average is the one cell that makes the points total mean anything: "71" is a number,
   "71 against an average of 56" is proof of use. GW rank is the same argument at higher
   resolution. Hit renders only when it happened, in --bad -- a page that claims "whatever
   it told us to do, we did it" and then quietly omits a −4 is not that page. */
function scoreboard(r){
  const hist = r.history||[];
  const last = hist.length ? hist[hist.length-1] : null;
  const cells = ['<span class="htbadge">FPL Armband</span>'];

  if(r.result_event){
    const word = r.result_state === 'live' ? 'Live' : r.result_state === 'final' ? 'Final' : '';
    // The receipt for /wildcard's own hypothetical: the week this account actually
    // plays a chip, FPL's own record of it (never the plan) names it right here,
    // where the RESULT is, so the week nobody projected but everybody chipped is
    // not invisible on the one page that would otherwise just show a normal week.
    const chip = r.chip ? ` <i class="htchip">${esc(r.chip)}</i>` : '';
    cells.push(`<span class="htgw">Gameweek ${r.result_event}${chip}${word ? ` <i>${esc(word)}</i>` : ''}</span>`);
  }
  // Points reads live_points while the gameweek is still being played and
  // history[last].points once FPL has settled it -- FPL itself reports 0 for
  // history[last].points until scoring finishes, which is exactly the state a reader
  // is most likely to check this page in (see viewmodel.Results.LivePoints's own
  // comment; this script sums neither figure, it only picks which server-decided one
  // to show). One cell, one meaning, two sources depending on whether FPL has
  // settled. The LIVE/FINAL chip above already tells the reader which one they are
  // looking at, so this cell carries no second label for it.
  if(last){
    const pts = r.result_state === 'live' ? r.live_points : last.points;
    cells.push(`<span class="htstat"><span class="v">${pts}</span><span class="k">Points</span></span>`);
  }
  if(r.event_average > 0){
    cells.push(`<span class="htstat"><span class="v sec">${r.event_average}</span><span class="k">Average</span></span>`);
  }
  if(last && last.rank != null){
    cells.push(`<span class="htstat"><span class="v">${(+last.rank).toLocaleString()}</span><span class="k">GW rank</span></span>`);
  }
  if(last && last.hit > 0){
    cells.push(`<span class="htstat"><span class="v bad">−${last.hit}</span><span class="k">Hit</span></span>`);
  }
  if(r.overall_rank > 0){
    cells.push(`<span class="htstat"><span class="v">${(+r.overall_rank).toLocaleString()}</span><span class="k">Overall rank</span></span>`);
  }
  return `<div class="houseteamrow">${cells.join('')}</div>`;
}

window.ArmbandResults = { scoreboard, pitch, card };

})();
