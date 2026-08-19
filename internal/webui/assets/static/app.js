/* FPL Armband — the client application.
 *
 * Served from /assets/app.js and lifted out of app.html so the page can carry a
 * Content-Security-Policy that forbids inline script entirely. A policy that has to
 * allow 'unsafe-inline' to run this file would also allow whatever an injected string
 * managed to open, which is most of the value gone.
 *
 * Vanilla DOM, no framework, no build step. It computes no model quantity: every
 * number it draws arrives from /api/state, already decided. See internal/viewmodel.
 */

/* ============================================================
   DATA — lifted from the real GW1 output
   ============================================================ */
const CLUBC={TOT:'#132257',LIV:'#C8102E',NEW:'#241F20',MUN:'#DA291C',MCI:'#6CABDD',
  EVE:'#003399',CHE:'#034694',SUN:'#EB172B',BRE:'#E30613',BHA:'#0057B8',COV:'#78D0F3',
  FUL:'#CC0000',ARS:'#EF0107',AVL:'#95BFE5',BOU:'#DA291C',NFO:'#DD0000',CRY:'#1B458F',
  IPS:'#0044A9',LEE:'#FFCD00',WOL:'#FDB913'};

/* ============================================================
   ESCAPING — read this before adding a template literal
   ============================================================

   Every string below that comes from the server is interpolated into innerHTML.
   With mock data that was safe by luck. With real data it is not: player names,
   FPL's own `news` prose and, once overrides are writable, text the reader typed
   all arrive here, and a name containing markup would execute rather than render.

   The Go template this application replaces escaped everything for free, and is
   pinned by TestEveryNameIsEscapedInEveryView. Moving rendering to the client
   dropped that guarantee silently, which is the whole reason this helper exists.

   The rule: any ${...} holding a STRING goes through esc(). Numbers formatted by
   toFixed do not need it, but they do not suffer from it either. When in doubt,
   escape. */
function esc(v){
  if(v===null||v===undefined) return '';
  return String(v).replace(/[&<>"']/g, c => ({
    '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'
  })[c]);
}

/* ============================================================
   DATA — every figure arrives from /api/state, already decided
   ============================================================

   The prototype carried twelve hardcoded collections. They are all server-side
   now except two, which stay because they are design data rather than model
   output: CLUBC (club colours) and LIB (the suggested-override library, which is
   copy).

   Nothing here recomputes a model quantity. The role band, the delta against the
   weakest starter, whether a candidate clears the transfer gate and whether an
   override needs re-checking are all decided in Go and copied across. The design
   note believes the re-check rule is fourteen days; it is seven, and the client
   is not the place to discover that. */
let FIX={};        /* club -> [[opponent, 'H'|'A', fdr], ...] */
let P=[];          /* the fifteen */
let POOL=[];       /* the market */
let OV=[];         /* standing overrides */
let BLIND=[];      /* where the model says it is blind */
let GWS=[];        /* the rail: current + upcoming, length is data-driven */
let WEAKEST={};    /* position -> the weakest starter's score */
let BENCHMARKS=[]; /* the same, with the name and price the legend prints */
let WHY=[];        /* the marginal calls behind this eleven */
let TODAY=new Date();
let STATE=null;    /* the raw document, for anything not mapped below */

/* The chip spellings the engine uses, mapped to the keys this file already had.
   One direction only: the engine's spelling is authoritative and the client must
   not invent a chip the plan does not name. */
const CHIPKEY={
  'Wildcard':'wildcard', 'Free Hit':'freehit',
  'Bench Boost':'bboost', 'Triple Captain':'tcap'
};

/* A player as every panel here draws him. The server sends more than this; the
   fields below are the ones the prototype's renderers already read, kept under
   their original names so the rendering is untouched by the data swap. */
function player(p){
  return {
    id:p.id, code:p.code, n:p.name, club:p.club, pos:p.pos, pr:p.price,
    xp:p.xp, p90:p.per90, mn:p.minutes, rel:p.reliability, own:p.ownership,
    role:p.role, status:p.status, news:p.news, fixtures:p.fixtures||[],
    ov:p.override ? {
      t:p.override.label, why:p.override.reason, set:p.override.set_on,
      lapse:p.override.until, chk:p.override.checked,
      needsCheck:p.override.needs_check, age:p.override.check_age
    } : undefined
  };
}

/* hydrate maps the server document onto the shapes the renderers expect. It is
   the ONLY place that knows both vocabularies; everything downstream reads the
   prototype's names and is unchanged. */
function hydrate(st){
  STATE=st;
  TODAY=new Date(st.now);

  P=(st.squad.players||[]).map(player);
  POOL=(st.market.rows||[]).map(r=>{
    const p=player(r.player);
    p.delta=r.delta; p.clears=r.clears_gate;
    return p;
  });

  /* The FDR strip is per club, and the fixture list rides on every player, so
     the first player from each club settles it. They cannot disagree: a club
     plays who it plays. */
  FIX={};
  for(const p of P.concat(POOL)){
    if(FIX[p.club]||!p.fixtures.length) continue;
    FIX[p.club]=p.fixtures.map(f=>[f.opp, f.home?'H':'A', f.fdr]);
  }

  OV=(st.overrides.live||[]).map((o,i)=>({
    id:'o'+i, t:o.label, kind:o.kind, who:o.player, club:o.club,
    set:o.set_on, lapse:o.until, chk:o.checked, inSquad:o.in_squad,
    why:o.reason, needsCheck:o.needs_check, age:o.check_age, flag:o.flag,
    session:o.session, eff:''
  }));

  BLIND=st.blind||[];
  WHY=st.why||[];

  BENCHMARKS=st.market.benchmarks||[];
  WEAKEST={};
  for(const b of BENCHMARKS) WEAKEST[b.pos]=b.score;

  GWS=(st.gameweeks||[]).map(g=>({
    gw:g.gw, deadline:g.deadline ? new Date(g.deadline) : null,
    d:g.deadline ? fmtDeadline(new Date(g.deadline)) : '',
    chip:CHIPKEY[g.chip]||null, live:!!g.current, projected:g.projected
  }));

  const sq=st.squad;
  S.xi=(sq.xi||[]).slice();
  /* The prototype keeps the reserve keeper apart from the three outfield bench
     slots, because they are not interchangeable — the keeper covers exactly one
     player. The server sends the bench in substitution order, keeper first. */
  const bench=(sq.bench||[]).slice();
  const gkFirst=bench.findIndex(id=>{const p=P.find(x=>x.id===id);return p&&p.pos==='GKP';});
  S.benchGk = gkFirst>=0 ? bench.splice(gkFirst,1)[0] : null;
  S.bench=bench;
  S.cap=sq.captain||null;
  S.vc=sq.vice||null;
  S.modelXi=(sq.xi||[]).slice();
  S.gw=(GWS.find(g=>g.live)||GWS[0]||{gw:1}).gw;
}

/* fmtDeadline is the rail's short date. Deliberately not a countdown: the rail
   shows six or more weeks and a countdown is only meaningful for the next one. */
function fmtDeadline(d){
  return d.toLocaleDateString('en-GB',{weekday:'short',day:'numeric',month:'short'});
}

/* countdown is the top bar's figure for the gameweek being planned. The
   prototype hard-coded "1d 22h 41m", which was true for about an hour. */
function countdown(to){
  if(!to) return '';
  let ms=to-TODAY;
  if(ms<=0) return 'closed';
  const d=Math.floor(ms/86400000); ms-=d*86400000;
  const h=Math.floor(ms/3600000);  ms-=h*3600000;
  const m=Math.floor(ms/60000);
  return d>0 ? `${d}d ${h}h ${m}m` : `${h}h ${m}m`;
}

const LIB=[
 {t:'Promoted-club starter',d:'Zero PL minutes but a nailed starter in the Championship. Tells the optimiser he plays, so it can see him at all.',badge:'NAILED'},
 {t:'Long-term injury',d:'Excluded until a named return window. Carries an expiry so it lapses rather than rotting.',badge:'EXCL'},
 {t:'New No.1 keeper',d:'Keeper whose record is backup minutes but who has been confirmed as first choice.',badge:'NAILED'},
 {t:'Defence without its anchor',d:'Team-level goals-conceded multiplier when a defence loses the centre-back its numbers were earned with.',badge:'TEAM ×1.15'},
 {t:'Rotation risk',d:'Caps minutes for a player in a congested side who the model still treats as nailed.',badge:'ROTATION RISK'},
 {t:'Set-piece taker, unscored',d:'Bumps a player who has taken over corners or penalties that last season’s data attributes elsewhere.',badge:'MULT 1.20'}
];


/* ============================================================
   STATE
   ============================================================ */
const CHIPS=[
 {k:'wildcard', n:'Wildcard',       ic:'★'},
 {k:'freehit',  n:'Free Hit',       ic:'⇄'},
 {k:'bboost',   n:'Bench Boost',    ic:'▤'},
 {k:'tcap',     n:'Triple Captain', ic:'3×'}
];

/* S is the reader's state: what he has changed, and what he is looking at. Everything
   in it is empty until hydrate() fills it from the server -- the prototype seeded ids
   1..15 here, which drew a plausible pitch before any data arrived and would have
   masked a failed fetch as a working page.

   locks and blocks stay Sets because the code below reads them thirty times and a Set
   is the right shape for that. They are NOT serialisable as they stand -- JSON.stringify
   silently turns a Set into {} -- so anything sending S to the server goes through
   snapshot(), never S itself. */
let S={
  gw:1, view:'pitch',
  xi:[], bench:[], benchGk:null,
  cap:null, vc:null,
  locks:new Set(), blocks:new Set(), ovKind:'ALL',
  swapFrom:null,
  posFilter:'ALL', q:'', affordOnly:false, showAll:false,
  modelXi:[]
};

/* snapshot is S as something that survives JSON.stringify.
   HANDOFF.md section 6 says S "maps straight onto a Go struct for Wails bindings". That
   is true of this function and false of S, because two of its fields are Sets. Sending
   S directly would drop every lock and block the reader had set, silently and with a
   200. */
function snapshot(){
  return {
    gw:S.gw, xi:S.xi.slice(), bench:S.bench.slice(), benchGk:S.benchGk,
    captain:S.cap, vice:S.vc,
    locks:[...S.locks], blocks:[...S.blocks]
  };
}
const byId=id=>P.find(p=>p.id===id)||POOL.find(p=>p.id===id);
const gwState=()=>GWS.find(g=>g.gw===S.gw);

/* ============================================================
   HELPERS
   ============================================================ */
/* The band a player's minutes put him in, said in words -- this is how managers
   actually think, and the minute count stays in the detail sheet as the evidence.

   ⚠️ The band is DECIDED BY THE MODEL and arrives on the player. This function only
   chooses a colour and a short form for it.

   It used to compute the band here, from thresholds of 80/65/40 over four names. The
   model uses 75/60/40/20 over five, so a player on 78 expected minutes read "Likely
   starter" on this page and counted as "nailed" in the squad that had just been picked
   around him. One quantity, two implementations, disagreeing -- do not reintroduce a
   threshold in this file. */
const ROLES={
  'nailed':         {s:'Nailed',   c:'r1'},
  'likely starter': {s:'Likely',   c:'r2'},
  'rotation risk':  {s:'Rotation', c:'r3'},
  'squad player':   {s:'Squad',    c:'r4'},
  'fringe':         {s:'Fringe',   c:'r5'}
};
function role(band){
  const r=ROLES[band];
  /* An unknown band is rendered rather than swallowed. If the model grows a sixth, the
     page should say so plainly instead of quietly drawing it as a fringe player. */
  return r ? {l:band.charAt(0).toUpperCase()+band.slice(1), s:r.s, c:r.c}
           : {l:band||'unknown', s:band||'?', c:'r5'};
}
const roleChip=(band,sm)=>{const r=role(band);
  return `<span class="role ${r.c}${sm?' sm':''}">${esc(sm?r.s:r.l)}</span>`;};

function fdrHtml(club,n=5,from=0){
  const f=(FIX[club]||[]).slice(from,from+n);
  // pad to a constant width so card rhythm survives the end of the horizon
  const pad=Array(Math.max(0,n-f.length)).fill(null);
  return '<span class="fdr">'+
    f.map(x=>`<i class="f${x[2]}" title="${esc(x[0])} (${esc(x[1])}) difficulty ${x[2]}">${x[2]}</i>`).join('')+
    pad.map(()=>'<i class="blank" title="beyond the projected horizon">·</i>').join('')+
    '</span>';
}
function nextFix(club){
  const f=(FIX[club]||[])[S.gw-1]||(FIX[club]||[])[0];
  return f?`${esc(f[0])} (${esc(f[1])})`:'—';
}
function shape(){
  const c={GKP:0,DEF:0,MID:0,FWD:0};
  S.xi.forEach(id=>c[byId(id).pos]++);
  return c;
}
function formationStr(){const c=shape();return `${c.DEF}-${c.MID}-${c.FWD}`;}
function legal(){
  const c=shape();
  return c.GKP===1 && c.DEF>=3 && c.DEF<=5 && c.MID>=2 && c.MID<=5 && c.FWD>=1 && c.FWD<=3
    && S.xi.length===11;
}
/* gameweek-scaled projection so switching weeks moves real numbers */
function xpFor(p,gw){
  const f=(FIX[p.club]||[])[gw-1];
  if(!f) return p.xp;
  const base=p.p90*(p.mn/90);
  const adj=1+(3-f[2])*0.055;
  return +(base*adj).toFixed(2);
}
const xiPts=()=>S.xi.reduce((s,id)=>s+xpFor(byId(id),S.gw),0);
const benchPts=()=>[...S.bench,S.benchGk].reduce((s,id)=>s+xpFor(byId(id),S.gw),0);
function totalPts(){
  const chip=gwState().chip;
  let t=xiPts();
  const cap=xpFor(byId(S.cap),S.gw);
  t += chip==='tcap' ? cap*2 : cap;          // captain doubles (triples on TC)
  if(chip==='bboost') t += benchPts();
  return t;
}
const spend=()=>P.reduce((s,p)=>s+p.pr,0);
function clubCounts(){const m={};P.forEach(p=>m[p.club]=(m[p.club]||0)+1);return m;}

/* ============================================================
   RENDER — gameweek rail
   ============================================================ */
function renderRail(){
  const el=document.getElementById('gwrail');
  el.innerHTML=GWS.map(g=>{
    const c=CHIPS.find(c=>c.k===g.chip);
    return `<button class="gw${g.live?' live':''}" role="tab" data-gw="${g.gw}"
      aria-selected="${g.gw===S.gw}">
      <div class="n">GW${g.gw}${g.live?' <span class="k" style="letter-spacing:.1em">NOW</span>':''}</div>
      <div class="d">${g.d}</div>
      <div class="chipslot">${c?`<span class="pill on">${c.ic} ${c.n}</span>`:''}</div>
    </button>`;}).join('');
  el.querySelectorAll('.gw').forEach(b=>b.onclick=()=>{S.gw=+b.dataset.gw;renderAll();});
}

/* ============================================================
   RENDER — readout
   ============================================================ */
let lastTotal=null, deltaTimer=null;
function renderReadout(){
  const chip=gwState().chip, c=CHIPS.find(x=>x.k===chip);
  const model=S.modelXi.reduce((s,id)=>s+xpFor(byId(id),S.gw),0);
  const mine=xiPts();
  const vsm=+(mine-model).toFixed(2);
  const total=totalPts();
  const capX=xpFor(byId(S.cap),S.gw), mult=chip==='tcap'?3:2;

  document.getElementById('scorebug').innerHTML=`
   <div class="gwlz">GW${S.gw}<small>${gwState().live?'NOW':'PLANNED'}</small></div>
   <div class="sb-main">
     <span class="sb-big" id="sbTotal">${total.toFixed(1)}</span>
     <span class="sb-lbl"><span class="k">Projected</span>
       <span class="sub" style="display:block;font-family:var(--mono);font-size:10px;color:var(--ink3)">
         XI ${xiPts().toFixed(1)} + armband${chip==='bboost'?' + bench':''}</span></span>
     <span class="sb-delta" id="sbDelta"></span>
   </div>
   <div class="sb-div"></div>
   <div class="sb-cell"><span class="k">Captain</span>
     <div class="v">${esc(byId(S.cap).n)}</div>
     <div class="sub">${capX.toFixed(2)} → ${(capX*mult).toFixed(2)}${chip==='tcap'?' ×3':''}</div></div>
   <div class="sb-div"></div>
   <div class="sb-cell"><span class="k">Bench</span>
     <div class="v">${benchPts().toFixed(1)}<small>pts</small></div>
     <div class="sub">${chip==='bboost'?'counting':'not counting'}</div></div>
   <div class="sb-div"></div>
   <div class="sb-cell"><span class="k">In the bank</span>
     <div class="v">£${(100-spend()).toFixed(1)}m</div>
     <div class="sub">squad £${spend().toFixed(1)}m</div></div>
   <div class="sb-div"></div>
   <div class="sb-cell"><span class="k">Chip</span>
     <div class="v">${c?c.n:'None'}</div>
     <div class="sub">${c?'included above':(4-GWS.filter(g=>g.chip).length)+' of 4 left'}</div></div>
   <div class="sb-pad"></div>`;

  // the score reacts to your hand
  if(lastTotal!==null && Math.abs(total-lastTotal)>0.004){
    const d=total-lastTotal, el=document.getElementById('sbDelta');
    el.textContent=(d>0?'+':'')+d.toFixed(2);
    el.className='sb-delta show '+(d>0?'pos':'neg');
    clearTimeout(deltaTimer);
    deltaTimer=setTimeout(()=>el.className='sb-delta',1400);
  }
  lastTotal=total;

  const vs=document.getElementById('vsmodel');
  vs.innerHTML = vsm===0
    ? `<span class="dim">matches the model's XI</span>`
    : `<span class="${vsm>0?'acc':'badc'}">${vsm>0?'+':''}${vsm.toFixed(2)}</span> <span class="dim">vs the model's XI</span>`;

  // the deadline follows the gameweek you are planning, and only shouts when it should
  const t=document.getElementById('ddl'), g=gwState()||{};
  document.getElementById('ddlLabel').textContent=`GW${S.gw} deadline`;
  /* A countdown for the week actually being entered, a date for any later one. The
     prototype printed a literal "1d 22h 41m", which was true for about an hour. */
  const imminent=!!g.live;
  t.textContent = imminent ? countdown(g.deadline) : (g.d||'');
  t.className='t'+(imminent?' soon':'');
}

/* ============================================================
   RENDER — chip row
   ============================================================ */
function renderChips(){
  const cur=gwState().chip;
  const used=new Set(GWS.filter(g=>g.gw!==S.gw&&g.chip).map(g=>g.chip));
  document.getElementById('chiprow').innerHTML=
    `<span class="k" style="margin-right:2px">Chip for GW${S.gw}</span>`+
    CHIPS.map(c=>{
      const isUsed=used.has(c.k);
      const wk=GWS.find(g=>g.chip===c.k);
      return `<button class="chipbtn${isUsed?' used':''}" data-chip="${c.k}"
        aria-pressed="${cur===c.k}" ${isUsed?'disabled title="planned for GW'+wk.gw+'"':''}>
        <span class="dot"></span>${c.ic} ${c.n}${isUsed?` <span class="k">GW${wk.gw}</span>`:''}</button>`;
    }).join('')+
    (cur?`<span class="chipnote">${chipExplain(cur)}</span>`:
      `<span class="dim" style="font-size:12px;margin-left:4px">Pick one and the projection above re-runs under that chip's rules.</span>`);
  document.querySelectorAll('.chipbtn').forEach(b=>b.onclick=()=>{
    const g=gwState(); g.chip = g.chip===b.dataset.chip ? null : b.dataset.chip; renderAll();
  });
}
function chipExplain(k){
  return {
    bboost:`Bench boost: all 15 score. Your bench adds ${benchPts().toFixed(1)} pts — order stops mattering, so pick for points not safety.`,
    tcap:`Triple captain: ${esc(byId(S.cap).n)} scores ×3 (${(xpFor(byId(S.cap),S.gw)*3).toFixed(1)} pts).`,
    wildcard:`Wildcard: unlimited transfers, no hits. Budget rules still apply — the Players tab is now a full rebuild.`,
    freehit:`Free hit: this week's team only, reverts after GW${S.gw}. Nothing you buy here carries forward.`
  }[k];
}

/* ============================================================
   RENDER — the pitch
   ============================================================ */
function cardHtml(p,opts={}){
  const lock=S.locks.has(p.id), block=S.blocks.has(p.id);
  const isC=S.cap===p.id, isV=S.vc===p.id;
  const chip=gwState().chip;
  const x=xpFor(p,S.gw), mult=chip==='tcap'?3:2;
  return `<div class="card${lock?' haslock':''}${block?' hasblock':''}${S.swapFrom===p.id?' sel':''}${isC?' iscap':''}${isC&&chip==='tcap'?' tcap':''}${isV?' isvc':''}"
     draggable="true" data-id="${p.id}" style="--clubc:${CLUBC[p.club]||'#39506A'}">
    <div class="shirt">${isC?`<span class="bandc">${chip==='tcap'?'3×':'C'}</span>`:''}</div>
    ${isC?`<span class="armchip${chip==='tcap'?' tc':''}">${chip==='tcap'?'3×':'C'}</span>`:''}
    ${isV?`<span class="armchip v">V</span>`:''}
    <div class="chead">
      <span class="lhs"><span class="cl">${esc(p.club)}</span></span>
      <div class="acts">
        <button class="iconbtn arm-btn${isC?' isc':''}${isV?' isv':''}" data-act="arm" data-id="${p.id}"
          title="${isC?'Captain — click to make vice':isV?'Vice-captain — click to clear':'Give him the armband'}">
          ${isC?(chip==='tcap'?'3×':'C'):isV?'V':'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><path d="M4 9h16v6H4z"/><path d="M9 3l-2 6M15 3l2 6"/></svg>'}
        </button>
        <button class="iconbtn${lock?' on':''}" data-act="lock" data-id="${p.id}" title="Lock into the squad — auto-rebuilds keep him">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><rect x="4" y="10" width="16" height="11" rx="2"/><path d="M8 10V7a4 4 0 018 0v3"/></svg></button>
        <button class="iconbtn block${block?' on':''}" data-act="block" data-id="${p.id}" title="Block — never picked, even on a rebuild">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><circle cx="12" cy="12" r="9"/><path d="M6 6l12 12"/></svg></button>
      </div>
    </div>
    <div class="nm">${esc(p.n)}</div>
    <div class="meta"><span>£${p.pr.toFixed(1)}</span>${roleChip(p.role,true)}</div>
    <div class="xp">${isC
      ?`<span class="pre">${x.toFixed(2)}</span><span class="arw">→</span><b>${(x*mult).toFixed(2)}</b><span class="u">xPts</span>`
      :`<b>${x.toFixed(2)}</b><span class="u">xPts</span>`}</div>
    ${fdrHtml(p.club,opts.fdr||3,S.gw-1)}
    ${p.ov?`<div class="ovtag">set: ${esc(p.ov.t.toLowerCase())}</div>`:''}
  </div>`;
}
function renderPitch(){
  ['GKP','DEF','MID','FWD'].forEach(pos=>{
    const line=document.querySelector(`.line[data-pos="${pos}"]`);
    const ids=S.xi.filter(id=>byId(id).pos===pos);
    line.innerHTML=ids.map(id=>cardHtml(byId(id))).join('')||`<div class="slot">empty</div>`;
  });
  const br=document.getElementById('benchrow');
  br.innerHTML=
    `<div class="benchslot gk"><span class="ord">GK</span>${cardHtml(byId(S.benchGk),{fdr:2})}</div>`+
    S.bench.map((id,i)=>`<div class="benchslot"><span class="ord">${i+1}</span>${cardHtml(byId(id),{fdr:2})}</div>`).join('');

  document.getElementById('formation').textContent=formationStr();
  const ok=legal();
  const v=document.getElementById('validity');
  v.className='valid'+(ok?'':' no');
  v.innerHTML=ok
    ? `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><path d="M4 12l6 6L20 6"/></svg> legal`
    : `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3"><path d="M12 7v7M12 17.5v.5"/><circle cx="12" cy="12" r="9"/></svg> ${illegalReason()}`;
  document.getElementById('capName').textContent=byId(S.cap).n;
  document.getElementById('vcName').textContent=S.vc?byId(S.vc).n:'none set';
  const cc=clubCounts(), over=Object.entries(cc).filter(([,n])=>n>3);
  const cap=document.getElementById('clubcap');
  cap.className='pill'+(over.length?' bad':'');
  cap.textContent=over.length?`${over[0][0]} ${over[0][1]}/3 — over the club limit`:'max 3/club ✓';
  document.getElementById('benchval').innerHTML=benchPts().toFixed(1)+' <span class="unit">xPts</span>';
  document.querySelector('.bench').classList.toggle('boosted',gwState().chip==='bboost');
  wirePitch();
}
function illegalReason(){
  const c=shape();
  if(c.GKP!==1) return 'needs exactly 1 keeper';
  if(c.DEF<3) return 'needs at least 3 defenders';
  if(c.DEF>5) return 'max 5 defenders';
  if(c.MID<2) return 'needs at least 2 midfielders';
  if(c.MID>5) return 'max 5 midfielders';
  if(c.FWD<1) return 'needs at least 1 forward';
  if(c.FWD>3) return 'max 3 forwards';
  return 'needs 11 players';
}

/* ---- swap engine: drag on pointer, tap-tap on touch ---- */
function canSwap(a,b){
  const A=byId(a),B=byId(b);
  if(A.pos===B.pos) return true;
  // simulate
  const xi=new Set(S.xi);
  const aIn=xi.has(a), bIn=xi.has(b);
  if(aIn===bIn) return true;             // both on pitch or both on bench
  const next=new Set(S.xi);
  if(aIn){next.delete(a);next.add(b);} else {next.delete(b);next.add(a);}
  const c={GKP:0,DEF:0,MID:0,FWD:0};
  next.forEach(id=>c[byId(id).pos]++);
  return c.GKP===1&&c.DEF>=3&&c.DEF<=5&&c.MID>=2&&c.MID<=5&&c.FWD>=1&&c.FWD<=3;
}
function doSwap(a,b){
  if(a===b) return;
  if(!canSwap(a,b)){ flashInvalid(); return; }
  const slots=[['xi',S.xi],['bench',S.bench]];
  const posOf=id=>{
    if(S.xi.includes(id)) return ['xi',S.xi.indexOf(id)];
    if(S.bench.includes(id)) return ['bench',S.bench.indexOf(id)];
    if(S.benchGk===id) return ['gk',0];
    return null;
  };
  const pa=posOf(a), pb=posOf(b);
  const set=(p,id)=>{ if(p[0]==='gk') S.benchGk=id; else (p[0]==='xi'?S.xi:S.bench)[p[1]]=id; };
  set(pa,b); set(pb,a);
  if(!S.xi.includes(S.cap)) S.cap=S.xi.find(id=>byId(id).pos!=='GKP');
  renderAll();
}
function flashInvalid(){
  const hud=document.querySelector('.pitchhud');
  hud.style.background='rgba(255,77,106,.18)';
  setTimeout(()=>hud.style.background='',420);
}
function wirePitch(){
  document.querySelectorAll('.card').forEach(el=>{
    const id=+el.dataset.id;
    el.ondragstart=e=>{e.dataTransfer.setData('text/plain',id);el.classList.add('dragging');};
    el.ondragend=()=>el.classList.remove('dragging');
    el.ondragover=e=>e.preventDefault();
    el.ondrop=e=>{e.preventDefault();doSwap(+e.dataTransfer.getData('text/plain'),id);};
    el.onclick=e=>{
      const b=e.target.closest('.iconbtn');
      if(b){
        e.stopPropagation();
        if(b.dataset.act==='arm'){ cycleArmband(id); return; }
        if(!S.xi.includes(id) && false){}
        const set = b.dataset.act==='lock'?S.locks:S.blocks;
        set.has(id)?set.delete(id):set.add(id);
        if(b.dataset.act==='lock') S.blocks.delete(id); else S.locks.delete(id);
        renderAll(); return;
      }
      if(S.swapFrom!==null){ doSwap(S.swapFrom,id); S.swapFrom=null; setSwapbar(); return; }
      openSheet(id);
    };
  });
}
/* armband cycle: nothing → captain → vice → nothing. Starters only. */
function cycleArmband(id){
  if(!S.xi.includes(id)){ flashInvalid(); return; }
  const others=()=>S.xi.filter(x=>x!==id).sort((a,b)=>xpFor(byId(b),S.gw)-xpFor(byId(a),S.gw));
  if(S.cap===id){                       // captain → vice
    S.cap = S.vc && S.vc!==id ? S.vc : others()[0];
    S.vc  = id;
  } else if(S.vc===id){                 // vice → nothing
    S.vc = null;
  } else {                              // nothing → captain, old captain drops to vice
    const prev=S.cap; S.cap=id; S.vc = prev===id ? S.vc : prev;
  }
  renderAll();
}
function setSwapbar(){
  const bar=document.getElementById('swapbar');
  if(S.swapFrom===null){bar.classList.remove('on');}
  else{bar.classList.add('on');
    document.getElementById('swaptext').innerHTML=`Tap any player to swap with <b>${byId(S.swapFrom).n}</b>`;}
  renderPitch();
}
document.getElementById('swapcancel').onclick=()=>{S.swapFrom=null;setSwapbar();};

/* ============================================================
   RENDER — formation options
   Every legal shape, priced from the fifteen you already own.
   ============================================================ */
function bestFor(d,m,f){
  const pick=(pos,n)=>P.filter(p=>p.pos===pos&&!S.blocks.has(p.id))
    .sort((a,b)=>xpFor(b,S.gw)-xpFor(a,S.gw)).slice(0,n);
  const xi=[...pick('GKP',1),...pick('DEF',d),...pick('MID',m),...pick('FWD',f)];
  if(xi.length<11) return null;
  return {ids:xi.map(p=>p.id), pts:xi.reduce((s,p)=>s+xpFor(p,S.gw),0), swing:xi};
}
function renderShapes(){
  const cur=formationStr(), mine=xiPts(), out=[];
  for(let d=3;d<=5;d++) for(let m=2;m<=5;m++){
    const f=11-1-d-m; if(f<1||f>3) continue;
    const b=bestFor(d,m,f); if(!b) continue;
    out.push({k:`${d}-${m}-${f}`,d,m,f,pts:b.pts,ids:b.ids});
  }
  out.sort((a,b)=>b.pts-a.pts);
  document.getElementById('shapes').innerHTML=out.map(o=>{
    const diff=+(o.pts-mine).toFixed(2), is=o.k===cur;
    const missing=o.ids.filter(id=>!S.xi.includes(id)).map(id=>byId(id).n);
    return `<button class="shape-row" data-shape="${o.d}-${o.m}-${o.f}" aria-current="${is}">
      <span class="f">${o.k}</span>
      <span class="who">${is?'your shape now':missing.length?'brings in '+missing.join(', '):'same eleven'}</span>
      <span class="dd ${is?'':diff>0?'pos':'neg'}">${is?o.pts.toFixed(1):(diff>0?'+':'')+diff.toFixed(2)}</span>
    </button>`;}).join('');
  document.querySelectorAll('[data-shape]').forEach(b=>b.onclick=()=>{
    const [d,m,f]=b.dataset.shape.split('-').map(Number);
    const best=bestFor(d,m,f); if(!best) return;
    S.xi=best.ids;
    if(!S.xi.includes(S.cap)) S.cap=S.xi.slice().sort((x,y)=>xpFor(byId(y),S.gw)-xpFor(byId(x),S.gw))[0];
    if(S.vc&&!S.xi.includes(S.vc)) S.vc=null;
    S.bench=P.filter(p=>!S.xi.includes(p.id)&&p.pos!=='GKP').map(p=>p.id);
    S.benchGk=P.find(p=>p.pos==='GKP'&&!S.xi.includes(p.id)).id;
    renderAll();
  });
}

/* ============================================================
   RENDER — why rows
   ============================================================ */
function renderWhy(){
  /* The marginal calls -- "Ndiaye over Kadioglu, +0.94, and here is why" -- are the
     best thing on this panel and the model does not currently produce them. Naming the
     next-best alternative for each slot means re-running the optimiser with that slot
     forced, which belongs with the other optimiser-backed work.

     So the panel says it has nothing rather than showing the five worked examples the
     prototype shipped. Those were written by hand against one GW1 build; kept here they
     would read as this week's reasoning about this week's squad, and be neither. */
  document.getElementById('whyrows').innerHTML = WHY.length ? WHY.map(r=>`
    <div class="whyrow">
      <div class="swap"><span class="in">${esc(r.in)}</span> <span class="dim">over</span> <span class="out">${esc(r.out)}</span></div>
      <div class="rz">${esc(r.why)}</div>
      <div class="d ${r.delta>=0?'pos':'neg'}">${r.delta>0?'+':''}${r.delta.toFixed(2)}</div>
    </div>`).join('') : `
    <div class="whyrow"><div class="swap dim">—</div>
    <div class="rz dim">The marginal calls behind this eleven are not computed yet. The
    overrides below are what a human has changed; everything else is the model's own
    ordering.</div><div class="d dim"></div></div>`;
  /* The engine states its own blind spots as prose, one line each. The prototype split
     them into a headline, a count and a detail; the real source is a single sentence,
     and this package does not get to summarise the engine's explanation of itself. */
  document.getElementById('blindrows').innerHTML=BLIND.map(b=>`
    <div class="whyrow"><div class="swap"><span class="pill warn">blind</span></div>
    <div><b>${esc(b)}</b></div>
    <div class="d dim">→</div></div>`).join('');
}

/* ============================================================
   RENDER — player sheet (the "why" for one player)
   ============================================================ */
function openSheet(id){
  const p=byId(id), chip=gwState().chip;
  const f=(FIX[p.club]||[])[S.gw-1]||['—','',3];
  const adj=1+(3-f[2])*0.055;
  const inSquad=P.some(x=>x.id===id);
  const onPitch=S.xi.includes(id);
  const sheet=document.getElementById('sheet');
  sheet.innerHTML=`
   <header>
     <div style="width:6px;align-self:stretch;border-radius:3px;background:${CLUBC[p.club]||'#39506A'}"></div>
     <div style="flex:1">
       <div class="nm">${esc(p.n)}</div>
       <div class="sub">${esc(p.pos)} · ${esc(p.club)} · £${p.pr.toFixed(1)}m · ${p.own.toFixed(1)}% owned</div>
     </div>
     <button class="btn icon ghost" id="sheetclose" aria-label="Close">✕</button>
   </header>
   <div class="body">
     <div class="statgrid">
       <div><div class="k">xPts this GW</div><div class="v acc">${xpFor(p,S.gw).toFixed(2)}</div></div>
       <div><div class="k">Per £m</div><div class="v">${(xpFor(p,S.gw)/p.pr).toFixed(2)}</div></div>
       <div><div class="k">Role</div><div class="v" style="font-size:12px;padding-top:3px">${roleChip(p.role)}</div>
            <div class="dim" style="font-family:var(--mono);font-size:10px;margin-top:3px">${p.mn} min/gw modelled</div></div>
       <div><div class="k">Reliability</div><div class="v">${p.rel.toFixed(2)}</div>
            <div class="dim" style="font-family:var(--mono);font-size:10px;margin-top:3px">how often that role held up</div></div>
     </div>

     <div class="k" style="margin-bottom:6px">How the number is built</div>
     <div class="deriv panel" style="padding:10px 12px">
       <div class="step"><span class="muted">points per 90</span><b>${p.p90.toFixed(2)}</b></div>
       <div class="step"><span class="muted">× fixture ${esc(f[0])} (${esc(f[1])}) FDR ${f[2]}</span><b>×${adj.toFixed(3)}</b></div>
       <div class="step"><span class="muted">× minutes ${p.mn}/90</span><b>×${(p.mn/90).toFixed(3)}</b></div>
       ${chip==='tcap'&&S.cap===id?`<div class="step"><span class="muted">× triple captain</span><b>×3</b></div>`:
         S.cap===id?`<div class="step"><span class="muted">× captain</span><b>×2</b></div>`:''}
       <div class="step total"><span>projected GW${S.gw}</span>
         <b>${(xpFor(p,S.gw)*(S.cap===id?(chip==='tcap'?3:2):1)).toFixed(2)}</b></div>
     </div>

     <div class="k" style="margin:14px 0 6px">Next five</div>
     ${fdrHtml(p.club,5,S.gw-1)}
     <div class="dim" style="font-family:var(--mono);font-size:11px;margin-top:6px">
       ${(FIX[p.club]||[]).slice(S.gw-1,S.gw+4).map(x=>`${esc(x[0])}(${esc(x[1])},${x[2]})`).join(' · ')}
     </div>

     ${p.ov?`<div class="reason">
        <div class="h">Hand-set override — role set to ${esc(p.ov.t.toLowerCase())}</div>
        ${esc(p.ov.why)}
        <div style="margin-top:8px;font-family:var(--mono);font-size:10px;opacity:.75">
          set ${esc(p.ov.set)} · lapses after ${esc(p.ov.lapse)} · checked ${esc(p.ov.chk)}</div>
      </div>`:`<div class="panel" style="margin-top:12px;padding:10px 12px;font-size:12.5px;color:var(--ink2)">
        <b style="color:var(--ink)">No corrections.</b> This is the raw model number, measured from last season's returns.
      </div>`}

     <div class="sheetacts">
       ${inSquad?`
         <button class="btn primary" data-sact="swap">${onPitch?'Swap out of the XI':'Swap into the XI'}</button>
         <button class="btn" data-sact="cap">Make captain</button>
         <button class="btn" data-sact="vc">Make vice</button>
         <button class="btn ghost" data-sact="lock">${S.locks.has(id)?'Unlock':'Lock in'}</button>
         <button class="btn ghost" data-sact="block">${S.blocks.has(id)?'Unblock':'Block'}</button>`
       :`<button class="btn primary" data-sact="buy">Transfer in — £${p.pr.toFixed(1)}m</button>
         <button class="btn ghost" data-sact="block">${S.blocks.has(id)?'Unblock':'Block from every build'}</button>`}
     </div>
   </div>`;
  document.getElementById('scrim').classList.add('open');
  sheet.querySelector('#sheetclose').onclick=closeSheet;
  sheet.querySelectorAll('[data-sact]').forEach(b=>b.onclick=()=>{
    const a=b.dataset.sact;
    if(a==='cap'){S.cap=id; if(S.vc===id)S.vc=S.xi.find(x=>x!==id);}
    if(a==='vc') S.vc=id;
    if(a==='lock'){S.locks.has(id)?S.locks.delete(id):S.locks.add(id);S.blocks.delete(id);}
    if(a==='block'){S.blocks.has(id)?S.blocks.delete(id):S.blocks.add(id);S.locks.delete(id);}
    if(a==='swap'){S.swapFrom=id;closeSheet();setSwapbar();return;}
    if(a==='buy'){closeSheet();return;}
    closeSheet();renderAll();
  });
}
/* Captain picker — ranked by what the armband is actually worth this week */
function openArmbandPicker(which){
  const chip=gwState().chip, mult = chip==='tcap'?3:2;
  const rows=[...S.xi].map(id=>byId(id))
    .sort((a,b)=>xpFor(b,S.gw)-xpFor(a,S.gw));
  const best=xpFor(rows[0],S.gw), floor=xpFor(rows[rows.length-1],S.gw), span=Math.max(.01,best-floor);
  document.getElementById('sheet').innerHTML=`
   <header><div style="flex:1">
     <div class="nm">${which==='cap'?'Pick your captain':'Pick your vice-captain'}</div>
     <div class="sub">${which==='cap'
       ? `armband is worth ×${mult} this week${chip==='tcap'?' — triple captain is on':''}`
       : 'plays only if your captain does not start'}</div>
   </div><button class="btn icon ghost" id="sheetclose">✕</button></header>
   <div class="body" style="padding-top:8px">
     ${rows.map(p=>{
       const x=xpFor(p,S.gw), gain=+(x*(mult-1)).toFixed(2), isC=S.cap===p.id, isV=S.vc===p.id;
       const f=(FIX[p.club]||[])[S.gw-1]||['—','',3];
       return `<button class="caprow${isC||isV?' on':''}" data-pick="${p.id}">
         <span class="armslot">${isC?'C':isV?'V':''}</span>
         <span class="cn"><b>${esc(p.n)}</b> <span class="dim" style="font-family:var(--mono);font-size:10.5px">${esc(p.club)} ${esc(p.pos)}</span>
           <span style="display:block;margin-top:3px">${roleChip(p.role,true)}
             <span class="dim" style="font-family:var(--mono);font-size:10px">vs ${esc(f[0])} (${esc(f[1])})</span>
             <i style="font-style:normal">${fdrHtml(p.club,1,S.gw-1)}</i></span></span>
         <span class="cx"><b>${x.toFixed(2)}</b><span class="dim">xPts</span>
           <span class="gain">${which==='cap'?`+${gain.toFixed(2)} from the armband`:'backup'}</span></span>
         <span class="mb"><span class="mbar"><span style="width:${Math.max(3,Math.round((x-floor)/span*100))}%"></span></span></span>
       </button>`;}).join('')}
     <div class="storenote" style="margin-top:12px">
       Ranked by projected points for GW${S.gw}, not by name recognition. Bars span your XI only — from ${floor.toFixed(2)} to ${best.toFixed(2)} — so a short bar is a small real gap, not a bad player.
     </div>
   </div>`;
  document.getElementById('scrim').classList.add('open');
  document.getElementById('sheetclose').onclick=closeSheet;
  document.querySelectorAll('[data-pick]').forEach(b=>b.onclick=()=>{
    const id=+b.dataset.pick;
    if(which==='cap'){ if(S.vc===id) S.vc=S.cap; S.cap=id; }
    else { if(S.cap===id) S.cap=S.vc||S.xi.find(x=>x!==id); S.vc=id; }
    closeSheet(); renderAll();
  });
}
document.getElementById('capPick').onclick=()=>openArmbandPicker('cap');
document.getElementById('vcPick').onclick=()=>openArmbandPicker('vc');

function closeSheet(){document.getElementById('scrim').classList.remove('open');}
document.getElementById('scrim').onclick=e=>{if(e.target.id==='scrim')closeSheet();};

/* ============================================================
   RENDER — players market
   ============================================================ */
function renderPlayers(){
  const bank=100-spend();
  document.getElementById('bank').textContent='£'+bank.toFixed(1)+'m';
  /* The rest of this panel's header. Every one of these was a literal in the markup,
     which is how the squad came to be worth £99.5m on the pitch and £100.0m here. */
  const st=STATE||{squad:{},market:{}};
  const cost=st.squad.cost||0;
  document.getElementById('bankSub').textContent=`of £${(cost+bank).toFixed(1)}m budget`;
  document.getElementById('squadValue').textContent='£'+cost.toFixed(1)+'m';
  document.getElementById('squadValueSub').textContent=`${(st.squad.players||[]).length} players`;
  const gate=st.market.gate||0;
  document.getElementById('gateValue').innerHTML=
    `+${gate.toFixed(2)}<small>xPts/gw</small>`;
  document.getElementById('benchLegend').textContent=
    BENCHMARKS.map(b=>`${b.pos} vs ${b.name} ${b.score.toFixed(2)}`).join(' · ');
  let list=POOL.filter(p=>S.posFilter==='ALL'||p.pos===S.posFilter)
    .filter(p=>!S.q||((p.n+' '+p.club).toLowerCase().includes(S.q.toLowerCase())));
  // affordable = you can sell your weakest in that position and still cover him
  const weakest=pos=>P.filter(p=>p.pos===pos).sort((a,b)=>a.xp-b.xp)[0];
  list=list.map(p=>{
    const w=weakest(p.pos);
    return {...p,d:+(p.xp-WEAKEST[p.pos]).toFixed(2),afford:bank+w.pr-p.pr, out:w};
  });
  const reachable=list.filter(p=>p.afford>=0).length, clears=list.filter(p=>p.d>=0.4).length;
  if(S.affordOnly) list=list.filter(p=>p.afford>=0);
  list.sort((a,b)=>b.d-a.d);
  document.getElementById('marketnote').innerHTML=`
    <span class="gate pass"></span><b>${clears}</b> of ${POOL.length} clear the +0.40 transfer gate
    <span class="sep">·</span>
    <b>${reachable}</b> are reachable with £${bank.toFixed(1)}m in the bank
    ${bank<0.5?`<span class="sep">·</span><span class="warnc">every other move needs you to sell first</span>`:''}`;
  document.getElementById('poolCount').textContent=POOL.length;

  const MOB_CAP=40, shown=list.slice(0,S.showAll?list.length:MOB_CAP);
  const emptyHtml=`<div class="empty">
      <div class="big">Nothing clears this filter</div>
      <p>No player matches ${S.q?`“${S.q}”`:'these settings'}${S.affordOnly?' inside your budget':''}.</p>
      <button class="btn sm" id="clearFilters">Show all ${POOL.length} players</button>
    </div>`;
  document.getElementById('emptyState').innerHTML=list.length?'':emptyHtml;
  document.getElementById('ptable').style.display=list.length?'':'none';
  document.getElementById('moreline').innerHTML =
    list.length>shown.length
      ? `Showing ${shown.length} of ${list.length} · <button class="btn sm" id="showMore">Load the rest</button>`
      : list.length ? `<span>All ${list.length} shown</span>` : '';

  document.getElementById('ptbody').innerHTML=list.map(p=>`
   <tr data-id="${p.id}">
     <td><span class="gate${p.d>=0.4?' pass':''}" title="${p.d>=0.4?'clears the +0.40 transfer gate':'below the transfer gate'}"></span></td>
     <td><span class="who">${esc(p.n)}</span><span class="club">${esc(p.club)}</span>${S.blocks.has(p.id)?' <span class="pill bad">blocked</span>':''}</td>
     <td class="k">${esc(p.pos)}</td>
     <td>${fdrHtml(p.club,5,S.gw-1)}</td>
     <td>${roleChip(p.role)}</td><td class="n">${p.own.toFixed(1)}%</td>
     <td class="n">£${p.pr.toFixed(1)}${p.afford<0?`<span class="short">needs +£${Math.abs(p.afford).toFixed(1)}m</span>`:''}</td>
     <td class="n" style="font-weight:700">${p.xp.toFixed(2)}</td>
     <td class="n ${p.d>=0.4?'dpos':'dneg'}">${p.d>0?'+':''}${p.d.toFixed(2)}</td>
     <td class="n"><button class="btn sm" data-buy="${p.id}" title="Transfer in ${esc(p.n)}, sell ${esc(p.out.n)}">${esc(p.out.n.length>10?p.out.n.slice(0,10)+'…':p.out.n)}</button></td>
   </tr>`).join('');

  document.getElementById('plist').innerHTML=shown.map(p=>`
   <div class="prow" data-id="${p.id}">
     <div>
       <div class="l1"><span class="gate${p.d>=0.4?' pass':''}"></span>
         <span class="nm">${esc(p.n)}</span><span class="k">${esc(p.pos)}</span>
         <span class="club" style="font-family:var(--mono);font-size:10px;color:var(--ink3)">${esc(p.club)}</span></div>
       <div class="l2">£${p.pr.toFixed(1)}m ${roleChip(p.role,true)} ${p.own.toFixed(1)}% ${fdrHtml(p.club,3,S.gw-1)}
         ${p.afford<0?`<span class="short">needs +£${Math.abs(p.afford).toFixed(1)}m</span>`:''}</div>
     </div>
     <div class="r">
       <div class="xp">${p.xp.toFixed(2)}</div>
       <div class="dd ${p.d>=0.4?'dpos':'dneg'}">${p.d>0?'+':''}${p.d.toFixed(2)}</div>
       <span class="vs">vs ${esc(p.out.n)}</span>
     </div>
   </div>`).join('');

  const cf=document.getElementById('clearFilters');
  if(cf) cf.onclick=()=>{S.q='';S.affordOnly=false;S.posFilter='ALL';
    document.getElementById('psearch').value='';
    document.querySelectorAll('#posfilter button').forEach(x=>x.setAttribute('aria-pressed',x.dataset.pos==='ALL'));
    document.getElementById('affordToggle').setAttribute('aria-pressed','false');
    document.getElementById('affordToggle').innerHTML='<span class="gate"></span> Affordable only';
    renderPlayers();};
  const sm=document.getElementById('showMore');
  if(sm) sm.onclick=()=>{S.showAll=true;renderPlayers();};

  document.querySelectorAll('#ptbody tr,.prow').forEach(r=>r.onclick=e=>{
    if(e.target.closest('[data-buy]')) return;
    openSheet(+r.dataset.id);
  });
}
document.getElementById('posfilter').onclick=e=>{
  const b=e.target.closest('button'); if(!b)return;
  S.posFilter=b.dataset.pos;
  document.querySelectorAll('#posfilter button').forEach(x=>x.setAttribute('aria-pressed',x===b));
  renderPlayers();
};
document.getElementById('psearch').oninput=e=>{S.q=e.target.value;renderPlayers();};
document.getElementById('affordToggle').onclick=e=>{
  S.affordOnly=!S.affordOnly;
  e.currentTarget.setAttribute('aria-pressed',S.affordOnly);
  e.currentTarget.innerHTML=`<span class="gate${S.affordOnly?' pass':''}"></span> ${S.affordOnly?'Affordable only':'All players'}`;
  renderPlayers();
};

/* ============================================================
   RENDER — overrides
   ============================================================ */
/* ⚠️ There is no daysSince() here, and there must not be one.

   The prototype computed staleness as `daysSince(o.chk) > 14`. The rule actually in
   force is in internal/config: stale at SEVEN days, or within two gameweeks of lapsing
   -- a second condition the arithmetic above cannot express at all. So an override the
   model considered overdue rendered as fine for another week.

   The server decides it and sends `needsCheck`, the age in days, and the badge text. */
function renderOv(){
  const list=OV.filter(o=>S.ovKind==='ALL'||o.kind===S.ovKind);
  const stale=OV.filter(o=>o.needsCheck);
  document.getElementById('ovAll').textContent=OV.length;
  /* The two nav badges and the "binding" pill all count the same thing, so they are set
     from one place. The prototype hard-coded 9 into the markup, which stayed 9. */
  const ovCount=document.getElementById('ovCount');
  if(ovCount) ovCount.textContent=OV.length;
  const binding=document.getElementById('ovBinding');
  if(binding) binding.textContent=OV.length
    ? `${OV.length} override${OV.length===1?'':'s'} binding`
    : 'no overrides binding';
  const sc=document.getElementById('staleCount');
  sc.style.display=stale.length?'':'none';
  sc.textContent=stale.length?`${stale.length} due a re-check`:'';
  document.getElementById('ovlist').innerHTML=list.length?list.map(o=>`
   <div class="ovcard${o.needsCheck?' stale':''}">
     <span class="badge ${o.kind==='exclude'?'excl':o.kind==='club'?'team':''}">${esc(o.t)}</span>
     <div>
       <div class="who">${esc(o.who)}<span class="club">${esc(o.club)}</span>
         ${o.inSquad?'<span class="pill acc" style="margin-left:8px">in the fifteen</span>':''}</div>
       <!-- The three dates are printed exactly as the server phrased them. Until is
            already a clause ("lapses after GW10", or "indefinite — review"), and Checked
            already carries its age ("2026-07-10 (40d)"). Adding a "lapses after" prefix
            and a "(40d)" suffix here produced "lapses after indefinite — review" and
            "checked 2026-07-10 (40d) (40d)" -- the client re-deriving presentation the
            server had already decided. -->
       <div class="dates">set ${esc(o.set)}${o.lapse?` · ${esc(o.lapse)}`:''} · checked ${esc(o.chk||'never')}
         ${o.needsCheck?`<span class="pill warn" style="margin-left:6px">${esc(o.flag||'recheck')}</span>`:''}</div>
       <div class="txt clamp">${esc(o.why)}</div>
       ${o.eff?`<div class="effect">→ ${esc(o.eff)}</div>`:''}
     </div>
     <button class="btn sm ghost rm" data-del="${esc(o.id)}" title="Delete this override">✕</button>
   </div>`).join(''):`<div class="empty panel"><div class="big">No overrides of this kind</div>
     <p>The model is running unaided here — every number in this category is measured, not hand-set.</p></div>`;
  document.querySelectorAll('[data-del]').forEach(b=>b.onclick=()=>{
    OV=OV.filter(o=>o.id!==b.dataset.del);renderOv();
  });
  document.querySelectorAll('.txt.clamp').forEach(t=>t.onclick=()=>t.classList.toggle('clamp'));
  document.getElementById('liblist').innerHTML=LIB.map(l=>`
   <div class="libcard">
     <span class="pill ov">${esc(l.badge)}</span>
     <div class="t" style="margin-top:7px">${esc(l.t)}</div>
     <div class="d">${esc(l.d)}</div>
     <button class="btn sm">+ Add</button>
   </div>`).join('');
}
document.getElementById('ovfilter').onclick=e=>{
  const b=e.target.closest('button'); if(!b)return;
  S.ovKind=b.dataset.kind;
  document.querySelectorAll('#ovfilter button').forEach(x=>x.setAttribute('aria-pressed',x===b));
  renderOv();
};
document.getElementById('addOv').onclick=()=>{
  const sheet=document.getElementById('sheet');
  sheet.innerHTML=`
   <header><div style="flex:1"><div class="nm">New override</div>
     <div class="sub">binds every build until it lapses</div></div>
     <button class="btn icon ghost" id="sheetclose">✕</button></header>
   <div class="body">
     <div class="frow">
       <div class="field"><label class="k">Kind</label>
         <select><option>Set his role</option><option>Exclude player</option>
         <option>Score multiplier</option><option>Team-level adjustment</option></select></div>
       <div class="field"><label class="k">Role</label>
         <select><option>Nailed</option><option selected>Likely starter</option>
         <option>Rotation risk</option><option>Fringe player</option></select></div>
     </div>
     <div class="field"><label class="k">Player or club</label><input placeholder="Start typing a name…"></div>
     <div class="frow">
       <div class="field"><label class="k">Lapses after</label>
         <select><option>GW6</option><option>GW8</option><option selected>GW10</option><option>GW12</option><option>never</option></select></div>
       <div class="field"><label class="k">Set on</label><input value="2026-08-19" readonly></div>
     </div>
     <div class="field"><label class="k">Reasoning — what does the model not see?</label>
       <textarea placeholder="Name the source and the condition that would clear it. This is what you'll read in six weeks when you've forgotten why."></textarea>
       <div class="hint">Required. An override without a stated condition to clear it is the one that rots.</div></div>
     <div class="panel" style="padding:10px 12px;font-size:12.5px;color:var(--ink2)">
       <span class="k">Projected effect</span><div style="margin-top:5px"><b class="acc">no PL data → likely starter · 0.00 → 3.24 xPts/gw</b>
       <span class="dim">· would enter the XI at DEF, pushing out F.Kadıoğlu (−0.03)</span></div>
     </div>
     <div class="sheetacts"><button class="btn primary" id="sheetclose2">Save override</button>
       <button class="btn ghost" id="sheetclose3">Cancel</button></div>
   </div>`;
  document.getElementById('scrim').classList.add('open');
  ['sheetclose','sheetclose2','sheetclose3'].forEach(i=>{const e=document.getElementById(i);if(e)e.onclick=closeSheet;});
};
document.getElementById('showLib').onclick=()=>
  document.getElementById('libpanel').scrollIntoView({behavior:'smooth',block:'start'});

/* ============================================================
   VIEW SWITCHING
   ============================================================ */
const VIEWS=['pitch','players','overrides','brief'];

function setView(v, push){
  if(!VIEWS.includes(v)) v='pitch';
  S.view=v;
  VIEWS.forEach(x=> document.getElementById('view-'+x).hidden = x!==v);
  document.querySelectorAll('[data-view]').forEach(b=>
    b.setAttribute('aria-selected', b.dataset.view===v));
  /* The panel is in the URL, so a view can be linked to, reloaded into, and reached by
     anything driving a browser. Replace rather than push: the tabs are one screen, and
     filling the back button with them would make Back mean "previous tab" instead of
     "the page I came from". */
  if(push!==false && location.hash!=='#'+v) history.replaceState(null,'','#'+v);
  window.scrollTo({top:0});
}
document.querySelectorAll('[data-view]').forEach(b=>b.onclick=()=>setView(b.dataset.view));
addEventListener('hashchange',()=>setView(location.hash.slice(1), false));

/* ============================================================
   BOOT
   ============================================================ */
function renderAll(){renderRail();renderReadout();renderChips();renderPitch();renderShapes();renderWhy();renderPlayers();renderOv();}

/* boot fetches the state and draws once.

   It deliberately renders NOTHING before the fetch returns. An empty pitch drawn first
   and filled in afterwards looks identical to a pitch that failed to load, and this page
   exists to show its working -- a blank surface claiming to be a squad is the one thing
   it must not do.

   A failure says so, in place, with the reason. The old page reloaded on error, which is
   the right answer when the server renders the document and the wrong one here: the
   reload would fetch the same shell and fail the same way, forever. */
function boot(){
  fetch('/api/state', {credentials:'same-origin'})
    .then(r=>{
      if(!r.ok) throw new Error(`the server answered ${r.status}`);
      return r.json();
    })
    .then(st=>{
      hydrate(st);
      renderAll();
      /* Honour a panel named in the URL, so a reload lands where the reader was. */
      if(location.hash) setView(location.hash.slice(1), false);
    })
    .catch(err=>{
      const el=document.getElementById('view-pitch');
      if(el) el.innerHTML=`<div class="panel" style="padding:24px">
        <b>The squad could not be loaded.</b>
        <div class="dim" style="margin-top:8px">${esc(err.message)}</div>
        <div class="dim" style="margin-top:8px">The page is served by <code>armband serve</code>;
        if it is still running, its output will say more.</div></div>`;
      console.error(err);
    });
}
boot();
