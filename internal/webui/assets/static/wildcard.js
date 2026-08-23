/* FPL Armband — the wildcard/free-hit hypothetical page (/wildcard).
 *
 * The caller for window.ArmbandProjection: one fetch, a handful of element
 * lookups, and a tab toggle that switches which half of GET /api/wildcard is
 * passed to the shared component — the same shape team.js settled at for
 * /armband-team. Everything that differs between the wildcard tab and the
 * free-hit tab is decided here, in what gets passed in and what copy wraps
 * around it; projection.js itself never learns which tab called it.
 *
 * Vanilla DOM, no framework, no build step, same as team.js and app.js.
 */
'use strict';

let CT = null;
let tab = location.hash === '#free-hit' ? 'free_hit' : 'wildcard';

function render(){
  const t = tab === 'wildcard' ? CT.wildcard : CT.free_hit;
  const unavailable = tab === 'wildcard' ? CT.wildcard_unavailable : CT.free_hit_unavailable;
  const headerEl = document.getElementById('chipheader');
  const bodyEl = document.getElementById('chipbody');
  const outEl = document.getElementById('chipout');

  document.getElementById('tab-wildcard').classList.toggle('on', tab==='wildcard');
  document.getElementById('tab-freehit').classList.toggle('on', tab==='free_hit');

  const clauseEl = document.getElementById('chipclause');

  if(!t){
    headerEl.innerHTML = '';
    outEl.innerHTML = '';
    clauseEl.textContent = '';
    bodyEl.innerHTML = `<div class="panel" style="padding:24px">
      <b>Not open this gameweek.</b>
      <div class="dim" style="margin-top:8px">${unavailable || 'The competition does not allow this chip yet.'}</div>
    </div>`;
    return;
  }

  headerEl.innerHTML = ArmbandProjection.header(Object.assign({}, t, {budget: CT.budget}));

  // The objective-mismatch clause (see wildcard.html's own comment): the expected-
  // points figure below is a one-gameweek number on BOTH tabs and directly
  // comparable, but the SQUAD each one maximises is not -- a wildcard's fifteen is
  // kept, a free hit's is handed back the following week. Said here, every time,
  // rather than once at the top of the page where a reader who jumped straight to
  // a tab would miss it.
  clauseEl.textContent = tab === 'wildcard'
    ? 'This fifteen is built to be kept — it is the squad the model plays from next gameweek too, not just this one.'
    : 'This fifteen is built for this gameweek only — the permanent squad comes straight back afterwards.';

  const notes = [];
  if(CT.caveat) notes.push(`<div class="chipcaveat">${esc(CT.caveat)}</div>`);
  if(CT.budget_warning) notes.push(`<div class="chipcaveat bad">${esc(CT.budget_warning)}</div>`);
  bodyEl.innerHTML = notes.join('') + ArmbandProjection.pitch(t);

  outEl.innerHTML = (t.out && t.out.length)
    ? `<p class="chipout"><b>Not in this fifteen, and in ours today:</b> ${t.out.map(esc).join(', ')}</p>`
    : '';
}

function esc(v){
  return String(v==null?'':v).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
}

function selectTab(name){
  tab = name;
  history.replaceState(null, '', tab==='free_hit' ? '#free-hit' : (location.pathname));
  render();
}

fetch('/api/wildcard', {credentials:'same-origin'})
  .then(r => {
    if(r.status === 409) return r.text().then(msg => { throw {seasonOver:true, msg}; });
    if(!r.ok) throw new Error(`the server answered ${r.status}`);
    return r.json();
  })
  .then(st => {
    CT = st.chip_teams;
    if(!CT) throw new Error('no projection in the response');
    document.title = `Gameweek ${CT.event} — If we chipped — FPL Armband`;
    document.getElementById('chipsub').textContent =
      `This is the fifteen the model would buy with our money, this week — gameweek ${CT.event}.`;
    document.getElementById('tab-wildcard').addEventListener('click', () => selectTab('wildcard'));
    document.getElementById('tab-freehit').addEventListener('click', () => selectTab('free_hit'));
    render();
  })
  .catch(err => {
    const el = document.getElementById('chipbody');
    if(!el) return;
    document.getElementById('chipheader').innerHTML = '';
    document.getElementById('chipout').innerHTML = '';
    if(err && err.seasonOver){
      el.innerHTML = `<div class="panel" style="padding:24px"><b>The season is over.</b>
        <div class="dim" style="margin-top:8px">There is no gameweek left to project.
        See <a href="/armband-team">FPL Armband’s Team</a> for how it went.</div></div>`;
      return;
    }
    el.innerHTML = `<div class="panel" style="padding:24px">
      <b>The projection could not be loaded.</b>
      <div class="dim" style="margin-top:8px"></div>
    </div>`;
    el.querySelector('.dim').textContent = err.message || 'Unknown error';
  });
