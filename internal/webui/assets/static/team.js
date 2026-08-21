/* FPL Armband — the spectator team page (/armband-team).
 *
 * Deliberately its own small script, not a mode of app.js. app.js is the interactive
 * builder: drag-and-drop, locks, leave-outs, the captain picker, a session. None of that
 * exists here -- this document has no reader input at all, so it has no state object and
 * no event wiring beyond one fetch. Keeping it separate is what keeps it separate: a
 * shared renderer would eventually grow one #if for "is this the read-only page" and then
 * a second, and the two surfaces would start silently drifting toward each other.
 *
 * Vanilla DOM, no framework, no build step, same as app.js. Computes no model quantity:
 * every number here arrives from /api/armband-team already decided. See
 * internal/viewmodel.HouseTeam and TeamPlayer.
 */
'use strict';

const CLUBC={TOT:'#132257',LIV:'#C8102E',NEW:'#241F20',MUN:'#DA291C',MCI:'#6CABDD',
  EVE:'#003399',CHE:'#034694',SUN:'#EB172B',BRE:'#E30613',BHA:'#0057B8',COV:'#78D0F3',
  FUL:'#CC0000',ARS:'#EF0107',AVL:'#95BFE5',BOU:'#DA291C',NFO:'#DD0000',CRY:'#1B458F',
  IPS:'#0044A9',LEE:'#FFCD00',WOL:'#FDB913'};

function esc(v){
  return String(v==null?'':v).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
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

/* statRow is this gameweek's counting stats: goals and assists for everyone, then a
   position-specific pair. A goalkeeper gets Clean sheet and Saves; every other position
   gets Defensive Contribution instead -- FPL does not score outfielders' clean sheets on
   this card and does not score keepers on CBIT at all (see
   internal/analysis.DefConThreshold's own comment), so the two never share a card. DC
   gets a green/red verdict once his match has started: green means he has cleared the bar
   FPL pays 2 points at, red means not yet -- reached is recomputed live and can flip green
   mid-match, never the other way. Before kickoff neither Saves nor DC render at all (both
   nil from the server): "the match has not started" is not the same fact as "he has zero",
   and drawing a red pill for a game that has not kicked off would say the wrong one. */
function statRow(p){
  const stats = [['G',p.goals],['A',p.assists]];
  let extra = '';
  if(p.pos === 'GKP'){
    // Clean sheet is the keeper's own defensive stat -- DC is an outfield-only channel
    // (see analysis.DefConThreshold's own comment, and Saves below), so the two never
    // both appear on one card. Every other position gets DC instead.
    stats.push(['CS', p.clean_sheets]);
  }
  if(p.saves != null){
    stats.push(['SV', p.saves]);
  } else if(p.def_con != null){
    const cls = p.def_con_reached ? 'ok' : 'no';
    extra = `<span class="tpstat tpdc ${cls}" title="${p.def_con_reached ? 'Cleared the defensive contribution bar' : 'Has not cleared the defensive contribution bar yet'}"><b>${p.def_con}</b>DC</span>`;
  }
  return `<div class="tpstats">${stats.map(([k,v]) =>
    `<span class="tpstat"><b>${v}</b>${k}</span>`).join('')}${extra}</div>`;
}

function teamCard(p, isCaptain, isVice){
  const band = CLUBC[p.club] || '#4C6072';
  const badge = isCaptain ? '<span class="tpband c">C</span>'
              : isVice ? '<span class="tpband v">V</span>' : '';
  return `
  <div class="tpcard">
    <div class="tpbar" style="background:${band}"></div>
    ${badge}
    <div class="tpbody">
      <div class="tpname">${esc(p.name)}</div>
      <div class="tpmeta">${esc(p.club)}</div>
      <div class="tpopp fdr">${fxrChip(p.opponent)}${liveDot(p.match_status)}</div>
      ${statRow(p)}
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
      ${players.map(p => teamCard(p, p.id===ht.captain, p.id===ht.vice)).join('')}
    </div>`;
  }).join('');
  const bench = (ht.bench||[]).map(p => teamCard(p, false, false)).join('');
  return `
    <div class="teamformation">
      <span class="k">Formation</span>
      <span class="form">${esc(ht.formation||'—')}</span>
    </div>
    <div class="pitch">${rows}</div>
    ${bench ? `<div class="teambench">
      <span class="k">Bench</span>
      <div class="teambenchrow">${bench}</div>
    </div>` : ''}
  `;
}

function houseTeamHtml(ht){
  const hist = ht.history||[];
  const last = hist.length ? hist[hist.length-1] : null;
  return `<div class="houseteamrow">
    <span class="htbadge">FPL Armband</span>
    ${last ? `<span class="htstat"><span class="v">${last.points}</span><span class="k">GW${last.event} actual</span></span>` : ''}
    ${ht.current_event ? `<span class="htstat"><span class="v acc">${(+ht.current_projected).toFixed(1)}</span><span class="k">GW${ht.current_event} projected</span></span>` : ''}
    ${ht.overall_rank ? `<span class="htstat"><span class="v">${(+ht.overall_rank).toLocaleString()}</span><span class="k">Overall rank</span></span>` : ''}
  </div>`;
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
