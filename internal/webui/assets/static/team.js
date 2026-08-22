/* FPL Armband — the spectator team page (/armband-team).
 *
 * Deliberately its own small script, not a mode of app.js. app.js is the interactive
 * builder: drag-and-drop, locks, leave-outs, the captain picker, a session. None of that
 * exists here -- this document has no reader input at all, so it has no state object and
 * no event wiring beyond one fetch. Keeping it separate is what keeps it separate: a
 * shared renderer would eventually grow one #if for "is this the read-only page" and then
 * a second, and the two surfaces would start silently drifting toward each other.
 *
 * This page's tense is "what happened", never "what might happen" -- see cardState and
 * houseTeamHtml. Every number here arrives from /api/armband-team already decided; this
 * script computes no model quantity and derives no gameweek state (see
 * internal/viewmodel.HouseTeam.ResultState's own comment for why that one lives
 * server-side, in cmd/armband.houseLiveSources, and not here).
 *
 * Vanilla DOM, no framework, no build step, same as app.js.
 */
'use strict';

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
   !Finished, computed server-side (see cmd/armband.houseLiveSources). Sits next to the
   opponent chip because that is where a reader is already looking to place him. */
function liveDot(status){
  return status==='live' ? '<span class="tplive" title="In progress"></span>' : '';
}

/* cardState is the single source of truth for which of the four results-card states a
   player is in, decided from match_status and minutes ALONE. Every other rendering
   decision below -- the points slot, whether a badge row exists at all, the card's colour
   -- reads this value rather than re-deriving it from some other field. That is what lets
   an absent badge mean one thing only ("he did not do it"): "not yet known" is state
   'toplay', which never reaches badgeHtml at all, so nothing downstream can confuse the
   two. */
function cardState(p){
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
  // earned nothing here". The count survives only in 'live', where it is genuinely
  // progress toward a bar rather than a verdict, so it is neutral (--ink3/.pend) there.
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

/* ptsHtml is the points slot -- three shapes for three facts, never alike, so a saved
   screenshot never reads one as another:
     'toplay' -- nothing is known, so nothing numeric renders. Not 0: a match that has not
                 kicked off has no score.
     'dnp'    -- DNP, a different STRING from "To play", not a different shade of the same
                 one. Colour alone would be too weak a carrier for the pair most damaging
                 to confuse.
     'live'/'played' -- the doubled figure (see teamCard's own comment on the captain), an
                 asterisk while the match is still live and bonus is not settled, or the
                 settled bonus token once it is. */
function ptsHtml(p, state){
  if(state === 'toplay') return '<div class="tppts"><span class="tpstate">To play</span></div>';
  if(state === 'dnp') return '<div class="tppts"><span class="tpstate" title="Did not play">DNP</span></div>';
  // The captain's contribution to the team score, not his raw return -- see teamCard.
  // Bench (multiplier 0) special-cases to the raw figure his card is actually reporting.
  const disp = p.multiplier === 0 ? p.points : p.points * p.multiplier;
  const num = `<b${disp<0?' class="neg"':''}>${disp}</b>`;
  if(state === 'live'){
    // The asterisk marks the NUMBER, not the bonus slot: during a live match it is not
    // merely the bonus that is unknown, it is the total, which is still moving. See
    // houseLiveSources -- match_status is "finished" iff FPL has applied bonus, so 'live'
    // is exactly the window with no honest bonus figure to show.
    return `<div class="tppts">${num}<i class="tpprov" title="Provisional — bonus is not settled">*</i></div>`;
  }
  return `<div class="tppts">${num}${bonusHtml(p)}</div>`;
}

/* teamCard draws one results card. The band chip (C / 3× / V) is driven by Multiplier, NOT
   by id === captain: FPL's own picks response already resolves a captain-did-not-play week
   to the vice's multiplier, so reading multiplier is reading what actually happened rather
   than who was originally handed the armband -- and a Triple Captain week must render 3×,
   which id === captain could never tell apart from an ordinary double.

   isCaptain/isVice still decide which STATIC role the card is in the eleven for -- the V
   chip only ever shows for the vice, and only when he is not the one actually carrying the
   multiplier that week. The .iscap/.isvc classes on the card wrapper track whichever chip
   actually renders (not the static role) because their only job is reserving name padding
   for whatever badge is really there -- see the padding-right fix on .tpname. */
function teamCard(p, isCaptain, isVice, isBench){
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

function pitchHtml(ht){
  const byPos = {GKP:[], DEF:[], MID:[], FWD:[]};
  for(const p of ht.xi||[]) (byPos[p.pos]||(byPos[p.pos]=[])).push(p);
  const rows = ['GKP','DEF','MID','FWD'].map(pos => {
    const players = byPos[pos]||[];
    if(!players.length) return '';
    return `<div class="line" data-pos="${pos}">
      ${players.map(p => teamCard(p, p.id===ht.captain, p.id===ht.vice, false)).join('')}
    </div>`;
  }).join('');
  const bench = (ht.bench||[]).map(p => teamCard(p, p.id===ht.captain, p.id===ht.vice, true)).join('');

  // The bench points line is the number every FPL manager looks for after a big bench
  // score. From history[last].bench_points, already parsed -- renders iff > 0, the same
  // "nothing to show" absence as everything else on this page.
  const hist = ht.history||[];
  const last = hist.length ? hist[hist.length-1] : null;
  const benchNote = last && last.bench_points > 0
    ? `<span class="teambenchpts">${last.bench_points} pts left on the bench</span>` : '';

  return `
    <div class="teamformation">
      <span class="k">Formation</span>
      <span class="form">${esc(ht.formation||'—')}</span>
    </div>
    <div class="pitch">${rows}</div>
    ${bench ? `<div class="teambench">
      <span class="k">Bench</span>${benchNote}
      <div class="teambenchrow">${bench}</div>
    </div>` : ''}
  `;
}

/* houseTeamHtml is the header row: a scoreboard for one completed gameweek, not a
   projection -- the old "GW{n} projected" cell paired the rail's Current gameweek with
   this squad's own score-bug figure, which are routinely different gameweeks the moment a
   deadline has passed but the rail has not rolled over (see viewmodel.HouseTeam's own
   comment). Removing the cell removes the bug; there is no replacement projection here on
   purpose.

   Average is the one cell that makes the points total mean anything: "71" is a number,
   "71 against an average of 56" is proof of use. GW rank is the same argument at higher
   resolution. Hit renders only when it happened, in --bad -- a page that claims "whatever
   it told us to do, we did it" and then quietly omits a −4 is not that page. */
function houseTeamHtml(ht){
  const hist = ht.history||[];
  const last = hist.length ? hist[hist.length-1] : null;
  const cells = ['<span class="htbadge">FPL Armband</span>'];

  if(ht.result_event){
    const word = ht.result_state === 'live' ? 'Live' : ht.result_state === 'final' ? 'Final' : '';
    cells.push(`<span class="htgw">Gameweek ${ht.result_event}${word ? ` <i>${esc(word)}</i>` : ''}</span>`);
  }
  // Points reads live_points while the gameweek is still being played and
  // history[last].points once FPL has settled it -- FPL itself reports 0 for
  // history[last].points until scoring finishes, which is exactly the state a reader
  // is most likely to check this page in (see viewmodel.HouseTeam.LivePoints's own
  // comment; this script sums neither figure, it only picks which server-decided one
  // to show). One cell, one meaning, two sources depending on whether FPL has
  // settled. The LIVE/FINAL chip above already tells the reader which one they are
  // looking at, so this cell carries no second label for it.
  if(last){
    const pts = ht.result_state === 'live' ? ht.live_points : last.points;
    cells.push(`<span class="htstat"><span class="v">${pts}</span><span class="k">Points</span></span>`);
  }
  if(ht.event_average > 0){
    cells.push(`<span class="htstat"><span class="v sec">${ht.event_average}</span><span class="k">Average</span></span>`);
  }
  if(last && last.rank != null){
    cells.push(`<span class="htstat"><span class="v">${(+last.rank).toLocaleString()}</span><span class="k">GW rank</span></span>`);
  }
  if(last && last.hit > 0){
    cells.push(`<span class="htstat"><span class="v bad">−${last.hit}</span><span class="k">Hit</span></span>`);
  }
  if(ht.overall_rank > 0){
    cells.push(`<span class="htstat"><span class="v">${(+ht.overall_rank).toLocaleString()}</span><span class="k">Overall rank</span></span>`);
  }
  return `<div class="houseteamrow">${cells.join('')}</div>`;
}

fetch('/api/armband-team', {credentials:'same-origin'})
  .then(r => { if(!r.ok) throw new Error(`the server answered ${r.status}`); return r.json(); })
  .then(st => {
    const ht = st.house_team;
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
    if(ht.result_event) document.title = `Gameweek ${ht.result_event} — our team — FPL Armband`;
    if(houseEl) houseEl.innerHTML = houseTeamHtml(ht);
    if(pitchEl) pitchEl.innerHTML = pitchHtml(ht);
  })
  .catch(err => {
    const el=document.getElementById('teampitch');
    if(el) el.innerHTML=`<div class="panel" style="padding:24px">
      <b>The team could not be loaded.</b>
      <div class="dim" style="margin-top:8px">${esc(err.message)}</div>
    </div>`;
  });
