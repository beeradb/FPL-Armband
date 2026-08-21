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

/* statRow is the season-counting-stats line the interactive card does not have: goals,
   assists, clean sheets, defensive contribution. Bonus rides along in the API but is not
   shown here -- four numbers already fills the row at card width, and these four are the
   ones asked for. Zero is shown, not hidden: before a ball is kicked, "0" is the honest
   answer and a blank card reads as broken. */
function statRow(p){
  const stats = [['G',p.goals],['A',p.assists],['CS',p.clean_sheets],['DC',p.def_con]];
  return `<div class="tpstats">${stats.map(([k,v]) =>
    `<span class="tpstat"><b>${v}</b>${k}</span>`).join('')}</div>`;
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
      <div class="tpopp fdr">${fxrChip(p.opponent)}</div>
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
