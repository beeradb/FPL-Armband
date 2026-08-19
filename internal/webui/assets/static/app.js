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

   The rule is about the SINK, not the source: any ${...} inside a template that
   becomes innerHTML goes through esc() unless it is a number formatted by toFixed.

   Stating it the other way round -- "escape the player name, escape the club" -- is how
   three sites were missed on the first pass. Each of them had a name in it that did not
   LOOK like one at the point of use: renderShapes joins names into a sentence, so the
   interpolation reads ${missing.join}; the swap bar reads ${byId(...).n}. The one in
   renderShapes was reachable on first paint with no interaction at all. */
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
/* ⚠️ The keys are analysis.chipLabels' keys, exactly. They read "tcap" here for a while
   and "3xc" in Go, so the triple captain arrived with no icon and chipExplain returned
   undefined, which was interpolated into innerHTML as the literal word. One spelling. */
const CHIPKEY={
  'Wildcard':'wildcard', 'Free Hit':'freehit',
  'Bench Boost':'bboost', 'Triple Captain':'3xc'
};

/* A player as every panel here draws him. The server sends more than this; the
   fields below are the ones the prototype's renderers already read, kept under
   their original names so the rendering is untouched by the data swap. */
function player(p){
  return {
    id:p.id, code:p.code, n:p.name, club:p.club, pos:p.pos, pr:p.price,
    xp:p.xp, p90:p.per90, mn:p.minutes, rel:p.reliability, own:p.ownership,
    role:p.role, status:p.status, news:p.news, fixtures:p.fixtures||[],
    /* The multiplier FPL's availability flag produces, carried rather than inferred from
       status: re-deriving it would be a second copy of a table that already exists, and
       its most important value is 0 -- a ruled-out player, whose score is zero for that
       reason and no other. */
    availability:p.availability,
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
    /* The gameweek is KEPT. The server sends the next N upcoming fixtures, so position in
       this array is not gameweek -- it only looks like it at GW1, which is where the test
       fixture is pinned. Everything below asks for a gameweek by number. */
    FIX[p.club]=p.fixtures.map(f=>({gw:f.gw, opp:f.opp, ha:f.home?'H':'A', fdr:f.fdr}));
  }

  OV=(st.overrides.live||[]).map((o,i)=>({
    id:'o'+i, code:o.code, t:o.label, kind:o.kind, who:o.player, club:o.club,
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
    chip:CHIPKEY[g.chip]||null, live:!!g.current, projected:g.projected,
    /* Which chips the competition allows THIS week, decided by the model. Gameweek one
       offers only the bench boost and the triple captain -- a wildcard buys nothing when
       transfers are already unlimited, and the free hit is not offered either. The page
       must not work that out for itself: it is a rule about the competition, and a second
       copy of it here would disagree with the model the first time either changed. */
    /* The icon is design data and stays here; the model decides which chips EXIST this
       week and what they are called. Merging by key rather than sending an icon from Go
       keeps the glyph a design decision. */
    playable:(g.playable||[]).map(c=>{
      const known=CHIPS.find(x=>x.k===c.key);
      return {k:c.key, n:c.label, ic:known?known.ic:''};
    })
  }));

  /* The session, as the server holds it. Rebuilt from the document rather than kept
     across renders: the server is the one that knows what is stored. */
  const sess=st.session||{};
  S.locks=new Set((sess.locked||[]).map(codeToId).filter(Boolean));
  S.blocks=new Set((sess.blocked||[]).map(codeToId).filter(Boolean));
  S.optimised=!!sess.optimised;
  S.saved=!!sess.saved;
  PENDING={
    v:1,
    seed:undefined,          /* the server owns the seed; never sent back */
    opt:!!sess.optimised,
    lock:(sess.locked||[]).slice(),
    excl:(sess.blocked||[]).slice(),
    dis:(sess.dismissed||[]).slice(),
    chips:Object.assign({},sess.chips||{})
  };

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

  /* The team, by code, so a reload restores exactly this arrangement rather than the
     model's answer to the same fifteen. */
  PENDING.squad=(sq.players||[]).map(p=>p.code).filter(Boolean);
  syncArrangement();
}

/* syncArrangement copies the reader's current lineup into the pending session. */
function syncArrangement(){
  PENDING.xi=S.xi.map(codeOf).filter(Boolean);
  PENDING.bench=[S.benchGk].concat(S.bench).filter(Boolean).map(codeOf).filter(Boolean);
  PENDING.cap=codeOf(S.cap)||0;
  PENDING.vc=codeOf(S.vc)||0;
}

/* codeToId is the reverse of codeOf, for the codes the session carries. */
function codeToId(code){
  const p=P.concat(POOL).find(x=>x.code===code);
  return p?p.id:0;
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

   locks and blocks stay Sets because the code below reads them repeatedly and a Set is
   the right shape for that. Nothing serialises S: what goes over the wire is PENDING,
   which is the server's own session shape and is rebuilt from the served document on
   every load. That matters beyond tidiness -- JSON.stringify turns a Set into {} without
   complaining, so a client that sent S would drop every lock and block with a 200. */
let S={
  gw:1, view:'pitch',
  xi:[], bench:[], benchGk:null,
  cap:null, vc:null,
  locks:new Set(), blocks:new Set(), ovKind:'ALL',
  swapFrom:null,
  posFilter:'ALL', q:'', affordOnly:false, showAll:false,
  modelXi:[]
};

/* TOKEN gates every write. It is put on the page by the server and read once here.
   /api/state is a read and does not need it; changing stored state does. */
const TOKEN=(document.querySelector('meta[name="armband-token"]')||{}).content||'';

/* save sends the reader's team to the server and redraws from the answer.
 *
 * ⚠️ The ANSWER is the new state, not an acknowledgement. The reader blocks a player and
 * the model may then pick a different eleven; the reader dismisses an override and every
 * projection moves. A page that applied its own change optimistically would be showing a
 * squad the model has not agreed to, which is the whole failure this application exists to
 * avoid -- and it is how a client starts recomputing model quantities to keep up.
 *
 * While a save is in flight the page is marked busy rather than frozen. Losing a click is
 * better than applying it twice, and a spinner over a page that still reads correctly is
 * better than a page that goes blank. */
let saving=false;
/* Saves run one at a time, in the order they were asked for, and none is dropped.
 *
 * The mutation runs when its turn comes rather than when the click happens, and that is the
 * whole design. `hydrate` rebuilds PENDING from the server's answer, so a mutation applied
 * while an earlier save is in flight would be overwritten by that answer the moment it
 * lands — the click would appear to work and then quietly undo itself.
 *
 * The previous version returned early while a save was in flight, which discarded the
 * mutation too: change the captain during a bench drag and nothing happened, silently. */
let CHAIN=Promise.resolve();
function save(mutate){
  /* The .catch is what keeps the chain alive. A synchronous throw inside a mutate -- and
     saveArrangement calls syncArrangement() in there -- rejects CHAIN, and every later
     save on the page is then skipped for the lifetime of the document: silently, with
     nothing but an unhandled rejection in a console nobody has open. Catching here makes
     that structurally impossible rather than merely unlikely. */
  CHAIN=CHAIN.then(()=>sendSave(mutate)).catch(err=>{
    notify('That did not save: '+(err&&err.message?err.message:err));
    console.error(err);
  });
  return CHAIN;
}

function sendSave(mutate){
  mutate(PENDING);
  saving=true;
  document.body.classList.add('saving');
  return fetch('/api/session',{
    method:'PUT',
    credentials:'same-origin',
    headers:{'Content-Type':'application/json','X-Armband-Token':TOKEN},
    body:JSON.stringify(PENDING)
  })
    .then(r=>{
      if(!r.ok) return r.text().then(t=>{throw new Error(t||('the server answered '+r.status));});
      return r.json();
    })
    .then(st=>{ hydrate(st); renderAll(); })
    .catch(err=>{
      /* Said out loud, in place. A save that fails silently is the reader's work
         disappearing with the page still claiming it is there. */
      notify('That did not save: '+err.message);
      console.error(err);
    })
    .finally(()=>{ saving=false; document.body.classList.remove('saving'); });
}

/* PENDING is the session as the server stores it: permanent player CODES, never element
   ids, because ids are reassigned every summer and this outlives a deploy. hydrate rebuilds
   it from the document on every load, so it is never assembled from the page's own memory. */
let PENDING={};

function notify(text){
  const el=document.getElementById('toast');
  if(!el){ alert(text); return; }
  el.textContent=text;
  el.hidden=false;
  clearTimeout(notify.t);
  notify.t=setTimeout(()=>{el.hidden=true;},6000);
}

/* Toggle a standing correction on one player, and let the server decide what it means.
 *
 * Lock and block are mutually exclusive — config.Roster.Set has the same rule — so setting
 * one clears the other rather than leaving a player both built around and refused.
 *
 * Both the card's icon and the sheet's button come here. They used to disagree: the sheet
 * saved and the card only re-rendered, so the same action was durable from one surface and
 * imaginary from the other.
 *
 * Nothing is applied locally. A block changes which fifteen the optimiser returns, and
 * guessing at that on the client would draw a squad the model has not agreed to. */
function toggleCorrection(id, kind){
  const code=codeOf(id);
  if(!code){ notify('That player has no code, so the correction cannot be saved.'); return; }
  return save(pending=>{
    const had=((kind==='lock'?pending.lock:pending.excl)||[]).includes(code);
    pending.lock=(pending.lock||[]).filter(c=>c!==code);
    pending.excl=(pending.excl||[]).filter(c=>c!==code);
    /* Clearing a correction the reader made is a deletion, and the server settles it
       against the dismissal list — so the code goes there too, or a standing override of
       the same kind read from config.json would simply reappear. */
    pending.dis=(pending.dis||[]).filter(c=>c!==code);
    if(had){ pending.dis.push(code); }
    else { (kind==='lock'?pending.lock:pending.excl).push(code); }
  });
}

/* codeOf maps an element id to the permanent code the server keys on. */
const codeOf=id=>{const p=byId(id);return p?p.code:0;};

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

/* The strip for `n` gameweeks starting at `from`, which is a GAMEWEEK NUMBER and not an
   index into the fixture list. */
function fdrHtml(club,n=5,from=S.gw){
  const all=FIX[club]||[];
  const f=all.filter(x=>x.gw>=from && x.gw<from+n);
  // pad to a constant width so card rhythm survives the end of the horizon
  const pad=Array(Math.max(0,n-f.length)).fill(null);
  return '<span class="fdr">'+
    f.map(x=>`<i class="f${x.fdr}" title="${esc(x.opp)} (${esc(x.ha)}) difficulty ${x.fdr}">${x.fdr}</i>`).join('')+
    pad.map(()=>'<i class="blank" title="beyond the projected horizon">·</i>').join('')+
    '</span>';
}
/* The club's fixture in a given gameweek, or null. A blank week is a real answer and the
   callers say so, rather than falling back to a different week's opponent -- which is what
   this did, and which quietly attributed one week's fixture to another. */
function fixtureIn(club,gw){ return (FIX[club]||[]).find(x=>x.gw===gw)||null; }
function nextFix(club){
  const f=fixtureIn(club,S.gw);
  return f?`${esc(f.opp)} (${esc(f.ha)})`:'blank';
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
/* A player's projection, as the model produced it.
 *
 * ⚠️ This used to SCORE the player here, and it is the reason the rule at the top of this
 * file is worth taking seriously. It read:
 *
 *     const base = p.p90 * (p.mn/90);
 *     const adj  = 1 + (3 - fdr) * 0.055;
 *     return base * adj;
 *
 * Five things wrong with it at once. `p.p90` is FixtureAdjXP90, which is ALREADY averaged
 * over each fixture's own difficulty, so the hand-rolled 0.055 ladder -- a constant that
 * exists nowhere in the Go model -- counted fixtures twice. It dropped
 * AvailabilityFactor, whose most important value is 0, so a ruled-out player showed a
 * positive projection on his card and 0.00 in the market on the same page. It dropped
 * Congestion and RoleFactor. It dropped FixtureLoad, so a blanking club scored as though
 * it played and a doubling club as though it played once. And it used minutes/90 where
 * the model uses a reliability figure that is a different quantity.
 *
 * The number it produced drove the score bug, the captain's arithmetic, the formation
 * comparison and the armband picker -- every headline figure on the pitch -- while
 * squad.xi_score and squad.expected arrived from the model and were never read.
 *
 * There is no per-player, per-gameweek projection in the contract today; Score is an
 * average over the horizon. The honest answer is to show that number and to add a
 * per-week one to internal/viewmodel if the rail needs to move, NOT to invent one here.
 */
function xpFor(p){ return p.xp; }
const xiPts=()=>S.xi.reduce((s,id)=>s+xpFor(byId(id)),0);
const benchPts=()=>[...S.bench,S.benchGk].reduce((s,id)=>s+xpFor(byId(id)),0);
function totalPts(){
  const chip=gwState().chip;
  let t=xiPts();
  const cap=xpFor(byId(S.cap));
  t += chip==='3xc' ? cap*2 : cap;          // captain doubles (triples on TC)
  if(chip==='bboost') t += benchPts();
  return t;
}
const spend=()=>P.reduce((s,p)=>s+p.pr,0);
/* The money, as the model priced it.
 *
 * ⚠️ Not `100 - spend()`. That asserts the opening allowance as a literal and is correct
 * for one week of the season: a mid-season squad is worth whatever it is worth, a
 * wildcard budget is not 100, and the bank is the entry's, not a subtraction. The server
 * sends squad.bank and squad.cost off Engine.AssemblyBudget, which knows all three. */
const bankOf=()=>(STATE&&STATE.squad&&typeof STATE.squad.bank==='number')?STATE.squad.bank:0;
const squadCost=()=>(STATE&&STATE.squad&&typeof STATE.squad.cost==='number')?STATE.squad.cost:spend();
/* The bar a swap has to clear, from review_policy.min_gain_for_free_transfer. */
const gateOf=()=>(STATE&&STATE.market&&typeof STATE.market.gate==='number')?STATE.market.gate:0;
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
  const model=S.modelXi.reduce((s,id)=>s+xpFor(byId(id)),0);
  const mine=xiPts();
  const vsm=+(mine-model).toFixed(2);
  const total=totalPts();
  const capX=xpFor(byId(S.cap)), mult=chip==='3xc'?3:2;

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
     <div class="sub">${capX.toFixed(2)} → ${(capX*mult).toFixed(2)}${chip==='3xc'?' ×3':''}</div></div>
   <div class="sb-div"></div>
   <div class="sb-cell"><span class="k">Bench</span>
     <div class="v">${benchPts().toFixed(1)}<small>pts</small></div>
     <div class="sub">${chip==='bboost'?'counting':'not counting'}</div></div>
   <div class="sb-div"></div>
   <div class="sb-cell"><span class="k">In the bank</span>
     <div class="v">£${bankOf().toFixed(1)}m</div>
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
  const week=gwState()||{};
  const cur=week.chip;
  const used=new Set(GWS.filter(g=>g.gw!==S.gw&&g.chip).map(g=>g.chip));
  /* Only the chips the competition allows THIS week. Gameweek one offers the bench boost
     and the triple captain and nothing else; the model decides that, not this file. The
     fallback to the full catalogue is for a week the server said nothing about. */
  const offered=(week.playable&&week.playable.length)?week.playable:CHIPS;
  document.getElementById('chiprow').innerHTML=
    `<span class="k" style="margin-right:2px">Chip for GW${S.gw}</span>`+
    offered.map(c=>{
      const isUsed=used.has(c.k);
      const wk=GWS.find(g=>g.chip===c.k);
      return `<button class="chipbtn${isUsed?' used':''}" data-chip="${esc(c.k)}"
        aria-pressed="${cur===c.k}" ${isUsed?'disabled title="planned for GW'+wk.gw+'"':''}>
        <span class="dot"></span>${c.ic} ${esc(c.n)}${isUsed?` <span class="k">GW${wk.gw}</span>`:''}</button>`;
    }).join('')+
    (cur?`<span class="chipnote">${chipExplain(cur)}</span>`:
      `<span class="dim" style="font-size:12px;margin-left:4px">Pick one and the projection above re-runs under that chip's rules.</span>`);
  document.querySelectorAll('.chipbtn').forEach(b=>b.onclick=()=>{
    const g=gwState()||{}; g.chip = g.chip===b.dataset.chip ? null : b.dataset.chip;
    save(pending=>{
      pending.chips=Object.assign({},pending.chips||{});
      if(g.chip) pending.chips[String(g.gw)]=g.chip; else delete pending.chips[String(g.gw)];
    });
  });
}
function chipExplain(k){
  return {
    bboost:`Bench boost: all 15 score. Your bench adds ${benchPts().toFixed(1)} pts — order stops mattering, so pick for points not safety.`,
    '3xc':`Triple captain: ${esc(byId(S.cap).n)} scores ×3 (${(xpFor(byId(S.cap))*3).toFixed(1)} pts).`,
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
  const x=xpFor(p), mult=chip==='3xc'?3:2;
  return `<div class="card${lock?' haslock':''}${block?' hasblock':''}${S.swapFrom===p.id?' sel':''}${isC?' iscap':''}${isC&&chip==='3xc'?' tcap':''}${isV?' isvc':''}"
     draggable="true" data-id="${p.id}" style="--clubc:${CLUBC[p.club]||'#39506A'}">
    <div class="shirt">${isC?`<span class="bandc">${chip==='3xc'?'3×':'C'}</span>`:''}</div>
    ${isC?`<span class="armchip${chip==='3xc'?' tc':''}">${chip==='3xc'?'3×':'C'}</span>`:''}
    ${isV?`<span class="armchip v">V</span>`:''}
    <div class="chead">
      <span class="lhs"><span class="cl">${esc(p.club)}</span></span>
      <div class="acts">
        <button class="iconbtn arm-btn${isC?' isc':''}${isV?' isv':''}" data-act="arm" data-id="${p.id}"
          title="${isC?'Captain — click to make vice':isV?'Vice-captain — click to clear':'Give him the armband'}">
          ${isC?(chip==='3xc'?'3×':'C'):isV?'V':'<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><path d="M4 9h16v6H4z"/><path d="M9 3l-2 6M15 3l2 6"/></svg>'}
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
    ${fdrHtml(p.club,opts.fdr||3,S.gw)}
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
/* Every change to the lineup is saved. The arrangement is the reader's, so the server
   stores it verbatim and hands it back on the next load rather than re-deriving the
   model's answer -- otherwise a player dragged to the bench would be back in the eleven
   after a reload with nothing to explain it. */
function saveArrangement(){
  return save(pending=>{ syncArrangement(); Object.assign(pending,{
    xi:PENDING.xi, bench:PENDING.bench, cap:PENDING.cap, vc:PENDING.vc, squad:PENDING.squad
  }); });
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
  saveArrangement();
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
        toggleCorrection(id, b.dataset.act); return;
      }
      if(S.swapFrom!==null){ doSwap(S.swapFrom,id); S.swapFrom=null; setSwapbar(); return; }
      openSheet(id);
    };
  });
}
/* armband cycle: nothing → captain → vice → nothing. Starters only. */
function cycleArmband(id){
  if(!S.xi.includes(id)){ flashInvalid(); return; }
  const others=()=>S.xi.filter(x=>x!==id).sort((a,b)=>xpFor(byId(b))-xpFor(byId(a)));
  if(S.cap===id){                       // captain → vice
    S.cap = S.vc && S.vc!==id ? S.vc : others()[0];
    S.vc  = id;
  } else if(S.vc===id){                 // vice → nothing
    S.vc = null;
  } else {                              // nothing → captain, old captain drops to vice
    const prev=S.cap; S.cap=id; S.vc = prev===id ? S.vc : prev;
  }
  renderAll();
  saveArrangement();
}
function setSwapbar(){
  const bar=document.getElementById('swapbar');
  if(S.swapFrom===null){bar.classList.remove('on');}
  else{bar.classList.add('on');
    document.getElementById('swaptext').innerHTML=`Tap any player to swap with <b>${esc(byId(S.swapFrom).n)}</b>`;}
  renderPitch();
}
document.getElementById('swapcancel').onclick=()=>{S.swapFrom=null;setSwapbar();};

/* ============================================================
   RENDER — formation options
   Every legal shape, priced from the fifteen you already own.
   ============================================================ */
function bestFor(d,m,f){
  const pick=(pos,n)=>P.filter(p=>p.pos===pos&&!S.blocks.has(p.id))
    .sort((a,b)=>xpFor(b)-xpFor(a)).slice(0,n);
  const xi=[...pick('GKP',1),...pick('DEF',d),...pick('MID',m),...pick('FWD',f)];
  if(xi.length<11) return null;
  return {ids:xi.map(p=>p.id), pts:xi.reduce((s,p)=>s+xpFor(p),0), swing:xi};
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
      <span class="who">${is?'your shape now':missing.length?'brings in '+missing.map(esc).join(', '):'same eleven'}</span>
      <span class="dd ${is?'':diff>0?'pos':'neg'}">${is?o.pts.toFixed(1):(diff>0?'+':'')+diff.toFixed(2)}</span>
    </button>`;}).join('');
  document.querySelectorAll('[data-shape]').forEach(b=>b.onclick=()=>{
    const [d,m,f]=b.dataset.shape.split('-').map(Number);
    const best=bestFor(d,m,f); if(!best) return;
    S.xi=best.ids;
    if(!S.xi.includes(S.cap)) S.cap=S.xi.slice().sort((x,y)=>xpFor(byId(y))-xpFor(byId(x)))[0];
    if(S.vc&&!S.xi.includes(S.vc)) S.vc=null;
    S.bench=P.filter(p=>!S.xi.includes(p.id)&&p.pos!=='GKP').map(p=>p.id);
    S.benchGk=P.find(p=>p.pos==='GKP'&&!S.xi.includes(p.id)).id;
    renderAll();
    saveArrangement();
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
  const f=fixtureIn(p.club,S.gw);
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
       <div><div class="k">xPts per gameweek</div><div class="v acc">${xpFor(p).toFixed(2)}</div></div>
       <div><div class="k">Per £m</div><div class="v">${(xpFor(p)/p.pr).toFixed(2)}</div></div>
       <div><div class="k">Role</div><div class="v" style="font-size:12px;padding-top:3px">${roleChip(p.role)}</div>
            <div class="dim" style="font-family:var(--mono);font-size:10px;margin-top:3px">${p.mn.toFixed(0)} min/gw modelled</div></div>
       <div><div class="k">Reliability</div><div class="v">${p.rel.toFixed(2)}</div>
            <div class="dim" style="font-family:var(--mono);font-size:10px;margin-top:3px">how often that role held up</div></div>
     </div>

     <!-- ⚠️ These are the model's INPUTS, not a derivation, and the difference is
          deliberate. This panel used to print "points per 90" then "x minutes mn/90" and a
          total -- the exact expression that was removed from xpFor for being wrong. Score
          is (rate x reliability + appearance + clean sheet + defensive contribution) x
          congestion x role certainty x availability x fixture load; minutes/90 is not one
          of its terms, and the figure shown as Reliability above is the one it uses.
          The steps did not produce the total under them.

          Reproducing the real expression here would be a second implementation of it,
          which is what this whole change exists to remove. So the inputs are shown as
          inputs and the answer is the model's. -->
     <div class="k" style="margin-bottom:6px">What goes into the number</div>
     <div class="deriv panel" style="padding:10px 12px">
       <div class="step"><span class="muted">points per 90, after fixtures</span><b>${p.p90.toFixed(2)}</b></div>
       <div class="step"><span class="muted">${f?`fixture ${esc(f.opp)} (${esc(f.ha)}) FDR ${f.fdr}`:'no fixture this gameweek'}</span><b>${f?'':'blank'}</b></div>
       <div class="step"><span class="muted">minutes reliability</span><b>${p.rel.toFixed(2)}</b></div>
       <div class="step"><span class="muted">availability</span><b>${(p.availability===undefined?1:p.availability).toFixed(2)}</b></div>
       ${chip==='3xc'&&S.cap===id?`<div class="step"><span class="muted">× triple captain</span><b>×3</b></div>`:
         S.cap===id?`<div class="step"><span class="muted">× captain</span><b>×2</b></div>`:''}
       <div class="step total"><span>the model's figure, per gameweek</span>
         <b>${(xpFor(p)*(S.cap===id?(chip==='3xc'?3:2):1)).toFixed(2)}</b></div>
     </div>

     <div class="k" style="margin:14px 0 6px">Next five</div>
     ${fdrHtml(p.club,5,S.gw)}
     <div class="dim" style="font-family:var(--mono);font-size:11px;margin-top:6px">
       ${(FIX[p.club]||[]).filter(x=>x.gw>=S.gw&&x.gw<S.gw+5).map(x=>`${esc(x.opp)}(${esc(x.ha)},${x.fdr})`).join(' · ')||'no fixtures in the projected window'}
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
         <button class="btn" data-sact="replace">Replace him…</button>
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
    /* Locking and blocking are STANDING corrections -- they bind every build, not just
       this page -- so they go to the server and the answer is the squad the model picks
       under them. Applying them locally would show a fifteen the model has not agreed to. */
    if(a==='lock'||a==='block'){
      closeSheet();
      toggleCorrection(id, a);
      return;
    }
    if(a==='swap'){S.swapFrom=id;closeSheet();setSwapbar();return;}
    if(a==='replace'){openPicker(id);return;}
    if(a==='buy'){closeSheet();return;}
    closeSheet();renderAll();
  });
}
/* Captain picker — ranked by what the armband is actually worth this week */
function openArmbandPicker(which){
  const chip=gwState().chip, mult = chip==='3xc'?3:2;
  const rows=[...S.xi].map(id=>byId(id))
    .sort((a,b)=>xpFor(b)-xpFor(a));
  const best=xpFor(rows[0]), floor=xpFor(rows[rows.length-1]), span=Math.max(.01,best-floor);
  document.getElementById('sheet').innerHTML=`
   <header><div style="flex:1">
     <div class="nm">${which==='cap'?'Pick your captain':'Pick your vice-captain'}</div>
     <div class="sub">${which==='cap'
       ? `armband is worth ×${mult} this week${chip==='3xc'?' — triple captain is on':''}`
       : 'plays only if your captain does not start'}</div>
   </div><button class="btn icon ghost" id="sheetclose">✕</button></header>
   <div class="body" style="padding-top:8px">
     ${rows.map(p=>{
       const x=xpFor(p), gain=+(x*(mult-1)).toFixed(2), isC=S.cap===p.id, isV=S.vc===p.id;
       const f=fixtureIn(p.club,S.gw);
       return `<button class="caprow${isC||isV?' on':''}" data-pick="${p.id}">
         <span class="armslot">${isC?'C':isV?'V':''}</span>
         <span class="cn"><b>${esc(p.n)}</b> <span class="dim" style="font-family:var(--mono);font-size:10.5px">${esc(p.club)} ${esc(p.pos)}</span>
           <span style="display:block;margin-top:3px">${roleChip(p.role,true)}
             <span class="dim" style="font-family:var(--mono);font-size:10px">${f?`vs ${esc(f.opp)} (${esc(f.ha)})`:'blank gameweek'}</span>
             <i style="font-style:normal">${fdrHtml(p.club,1,S.gw)}</i></span></span>
         <span class="cx"><b>${x.toFixed(2)}</b><span class="dim">xPts</span>
           <span class="gain">${which==='cap'?`+${gain.toFixed(2)} from the armband`:'backup'}</span></span>
         <span class="mb"><span class="mbar"><span style="width:${Math.max(3,Math.round((x-floor)/span*100))}%"></span></span></span>
       </button>`;}).join('')}
     <div class="storenote" style="margin-top:12px">
       Ranked by projected points per gameweek, not by name recognition. Bars span your XI only — from ${floor.toFixed(2)} to ${best.toFixed(2)} — so a short bar is a small real gap, not a bad player.
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
  const bank=bankOf(), gate=gateOf();
  document.getElementById('bank').textContent='£'+bank.toFixed(1)+'m';
  /* The rest of this panel's header. Every one of these was a literal in the markup,
     which is how the squad came to be worth £99.5m on the pitch and £100.0m here. */
  const st=STATE||{squad:{},market:{}};
  const cost=st.squad.cost||0;
  document.getElementById('bankSub').textContent=`of £${(cost+bank).toFixed(1)}m budget`;
  document.getElementById('squadValue').textContent='£'+cost.toFixed(1)+'m';
  document.getElementById('squadValueSub').textContent=`${(st.squad.players||[]).length} players`;
  document.getElementById('gateValue').innerHTML=
    `+${gate.toFixed(2)}<small>xPts/gw</small>`;
  const upTo=document.getElementById('bankUpTo');
  if(upTo) upTo.textContent=(STATE&&STATE.policy&&STATE.policy.bank_up_to)||'—';
  document.getElementById('benchLegend').textContent=
    BENCHMARKS.map(b=>`${b.pos} vs ${b.name} ${b.score.toFixed(2)}`).join(' · ');
  let list=POOL.filter(p=>S.posFilter==='ALL'||p.pos===S.posFilter)
    .filter(p=>!S.q||((p.n+' '+p.club).toLowerCase().includes(S.q.toLowerCase())));
  // affordable = you can sell your weakest in that position and still cover him
  const weakest=pos=>P.filter(p=>p.pos===pos).sort((a,b)=>a.xp-b.xp)[0];
  list=list.map(p=>{
    const w=weakest(p.pos);
    /* d and clears come from the server: MarketRow.Delta and MarketRow.ClearsGate.
       Colouring the gap against a hardcoded bar was the page recommending in colour what
       the policy refuses in prose -- the same defect this once had against zero. */
    return {...p,d:p.delta,clears:p.clears,afford:bank+w.pr-p.pr, out:w};
  });
  const reachable=list.filter(p=>p.afford>=0).length, clears=list.filter(p=>p.clears).length;
  if(S.affordOnly) list=list.filter(p=>p.afford>=0);
  list.sort((a,b)=>b.d-a.d);
  document.getElementById('marketnote').innerHTML=`
    <span class="gate pass"></span><b>${clears}</b> of ${POOL.length} clear the +${gate.toFixed(2)} transfer gate
    <span class="sep">·</span>
    <b>${reachable}</b> are reachable with £${bank.toFixed(1)}m in the bank
    ${bank<0.5?`<span class="sep">·</span><span class="warnc">every other move needs you to sell first</span>`:''}`;
  document.getElementById('poolCount').textContent=POOL.length;

  const MOB_CAP=40, shown=list.slice(0,S.showAll?list.length:MOB_CAP);
  const emptyHtml=`<div class="empty">
      <div class="big">Nothing clears this filter</div>
      <p>No player matches ${S.q?`“${esc(S.q)}”`:'these settings'}${S.affordOnly?' inside your budget':''}.</p>
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
     <td><span class="gate${p.clears?' pass':''}" title="${p.clears?`clears the +${gate.toFixed(2)} transfer gate`:'below the transfer gate'}"></span></td>
     <td><span class="who">${esc(p.n)}</span><span class="club">${esc(p.club)}</span></td>
     <td class="k">${esc(p.pos)}</td>
     <td>${fdrHtml(p.club,5,S.gw)}</td>
     <td>${roleChip(p.role)}</td><td class="n">${p.own.toFixed(1)}%</td>
     <td class="n">£${p.pr.toFixed(1)}${p.afford<0?`<span class="short">needs +£${Math.abs(p.afford).toFixed(1)}m</span>`:''}</td>
     <td class="n" style="font-weight:700">${p.xp.toFixed(2)}</td>
     <td class="n ${p.clears?'dpos':'dneg'}">${p.d>0?'+':''}${p.d.toFixed(2)}</td>
     <td class="n"><button class="btn sm" data-buy="${p.id}" title="Transfer in ${esc(p.n)}, sell ${esc(p.out.n)}">${esc(p.out.n.length>10?p.out.n.slice(0,10)+'…':p.out.n)}</button></td>
   </tr>`).join('');

  document.getElementById('plist').innerHTML=shown.map(p=>`
   <div class="prow" data-id="${p.id}">
     <div>
       <div class="l1"><span class="gate${p.clears?' pass':''}"></span>
         <span class="nm">${esc(p.n)}</span><span class="k">${esc(p.pos)}</span>
         <span class="club" style="font-family:var(--mono);font-size:10px;color:var(--ink3)">${esc(p.club)}</span></div>
       <div class="l2">£${p.pr.toFixed(1)}m ${roleChip(p.role,true)} ${p.own.toFixed(1)}% ${fdrHtml(p.club,3,S.gw)}
         ${p.afford<0?`<span class="short">needs +£${Math.abs(p.afford).toFixed(1)}m</span>`:''}</div>
     </div>
     <div class="r">
       <div class="xp">${p.xp.toFixed(2)}</div>
       <div class="dd ${p.clears?'dpos':'dneg'}">${p.d>0?'+':''}${p.d.toFixed(2)}</div>
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
     <button class="btn sm ghost rm" data-del="${esc(o.id)}" data-code="${o.code||0}"
       title="Clear this correction for your session">✕</button>
   </div>`).join(''):`<div class="empty panel"><div class="big">No overrides of this kind</div>
     <p>The model is running unaided here — every number in this category is measured, not hand-set.</p></div>`;
  /* Dismissing an override is a change to what the MODEL is running under, so it goes to
     the server and the page redraws from the squad that comes back.
     It used to filter a JavaScript array and re-render: the row vanished, the model went on
     applying the correction, and the squad did not move -- which is what "nothing gets
     updated" looked like from the outside. The row disappearing was the only thing that had
     happened.
     The config file is untouched. A dismissal is this session's, and `serve -persist` is
     the deliberate way a correction leaves the standing record. */
  document.querySelectorAll('[data-del]').forEach(b=>b.onclick=()=>{
    const code=+b.dataset.code;
    if(!code){ notify('That override has no player to clear.'); return; }
    save(pending=>{
      pending.dis=(pending.dis||[]).concat([code]).filter((c,i,a)=>a.indexOf(c)===i);
    });
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
function renderChipSummary(){
  const el=document.getElementById('chipnow');
  if(!el) return;
  const g=gwState()||{};
  const c=(g.playable&&g.playable.length?g.playable:CHIPS).find(x=>x.k===g.chip);
  el.textContent=c?c.n:'none this gameweek';
}

/* Where this fifteen came from, said plainly.
 *
 * The opening squad is deliberately varied rather than the model's single best, so a reader
 * looking at it deserves to know which of the two they have -- otherwise the tool appears to
 * be recommending something it is not. */
function renderSquadSource(){
  const el=document.getElementById('squadsource');
  if(!el) return;
  el.textContent = S.saved ? 'your saved team'
    : S.optimised ? "the model's best fifteen"
    : 'a strong opening fifteen — press Optimize for the model’s best';
  const opt=document.getElementById('optimise');
  if(opt) opt.disabled = !!S.optimised && !S.saved;
}

function renderAll(){renderRail();renderReadout();renderChips();renderChipSummary();renderSquadSource();renderPitch();renderShapes();renderWhy();renderPlayers();renderOv();}

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
      const hash=location.hash.slice(1);
      const replace=/^replace-(\d+)$/.exec(hash);
      if(replace){ setView('pitch', false); openPicker(+replace[1]); }
      else if(hash){ setView(hash, false); }
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
/* Optimize discards the arrangement and asks the model for its best fifteen.
 *
 * It CLEARS the squad rather than sending one: the server is what knows what best means,
 * and a client that sent its own answer would be a second optimiser. */
const optimiseBtn=document.getElementById('optimise');
if(optimiseBtn) optimiseBtn.onclick=()=>save(pending=>{
  pending.opt=true;
  pending.squad=undefined;
  pending.xi=undefined;
  pending.bench=undefined;
  pending.cap=undefined;
  pending.vc=undefined;
});

boot();


/* ============================================================
   THE REPLACEMENT PICKER
   ============================================================

   Replacing is not swapping, and the two verbs are deliberately different words for
   deliberately different acts. SWAP rearranges the fifteen you own and keeps its
   pitch-to-pitch flow. REPLACE is a transfer -- sell one man, buy from the market -- and its
   targets are not on screen, so it opens the sheet, the same surface the captain picker uses
   for "pick one from a ranked list".

   ⚠️ Position is a RULE here, not a filter. An FPL squad is always 2 GKP, 5 DEF, 5 MID and
   3 FWD, so a single transfer is always like-for-like: a defender cannot replace a
   midfielder in one move, and a panel offering it would be offering an illegal transfer.
   The position control is kept, but widening it opens a BROWSE -- price and projection only,
   with the delta and the gate dot suppressed, because they would price a move that cannot be
   made -- and a tap on one of those rows is refused with the rule on screen.

   Everything interpolated below goes through esc(). The design prototype had no escaping,
   because its data was invented; this reads real names and FPL's own injury prose. */

let R={out:null,pos:null,within:true,sel:null};

/* The money. The sale funds the purchase, so the budget is what he raises plus the bank.
 *
 * ⚠️ The sale price is his LISTED price. FPL sells at the purchase price plus half of any
 * rise since, which the contract does not carry -- so for a squad the reader has just built
 * these agree exactly, and for a squad carried through price changes this is optimistic by
 * up to half the rise. Stated rather than hidden: the header says "sells for" and it is the
 * one number here the model has not checked. */
const sellPriceOf=p=>p.pr;
const pickerBudget=()=>{const o=byId(R.out);return o?sellPriceOf(o)+bankOf():bankOf();};
const affordGap=c=>+(pickerBudget()-c.pr).toFixed(1);

/* The club rule: never a fourth from one club. Counted from the fifteen with the outgoing
   man removed, because he is the one leaving. */
function overClub(c){
  const o=byId(R.out);
  let n=0;
  for(const p of P){ if(p.id!==R.out && p.club===c.club) n++; }
  void o;
  return n>=3;
}

/* The ONE model rule this client restates, and the reason it has to.
 *
 * Everything else on this page is computed in Go and sent -- see the package comment. The
 * market's rows carry `clears_gate` from the server and this function is not used for them.
 * The picker cannot: its delta is against the player being REPLACED, and the server sends
 * deltas against the weakest starter, so there is no server answer to mirror.
 *
 * It is therefore a deliberate copy of present.ClearsGate, kept identical by
 * TestTheGateIsDecidedTheSameWayInBothLanguages, which runs one table through both. Change
 * one and the test fails on the other; change the rule and change it in Go first.
 *
 * The rounding is the rule, not an implementation detail: a row displaying "+0.40" counting
 * as below a +0.40 gate is the page contradicting itself on one line. A gate of zero clears
 * nothing, because an unset threshold is a question nobody asked. */
function clearsGate(d){ const g=gateOf(); if(g<=0) return false;
  /* Math.round(x*100)/100, NOT toFixed(2). They are different functions: toFixed is
     specified on the exact value of the double, while Go's math.Round rounds the float
     product, which can sit on a .5 boundary the value itself is below. They disagree on
     ordinary numbers -- 0.015, 0.295, 0.495 -- and agreed at the shipped 0.40 gate, which
     is how a passing equivalence test came to pin the constant instead of the rule. */
  return Math.round(d*100)/100 >= Math.round(g*100)/100; }

function pickerCandidates(){
  const o=byId(R.out);
  if(!o) return [];
  const own=new Set(P.map(p=>p.id));
  /* No block filter here: the server drops a blocked player from the market entirely, so
     POOL never carries one. Filtering again would read as the panel accounting for blocks
     when it is the server that does. */
  return POOL.filter(c=>c.pos===R.pos && !own.has(c.id));
}

/* The picker is addressable: /app#replace-<id>.
 *
 * Not scaffolding. The panel is where a transfer is decided, so it is the thing a reader
 * wants to come back to -- a reload keeps it open, and a link to it is a link to the
 * decision rather than to the page it was taken on. It is also what lets the layout suite
 * screenshot a panel that otherwise only exists after a tap. */
function openPicker(id){
  const o=byId(id);
  if(!o) return;
  R={out:id,pos:o.pos,within:true,sel:null};
  renderPicker();
  document.getElementById('scrim').classList.add('open');
  if(location.hash!=='#replace-'+id) history.replaceState(null,'','#replace-'+id);
}

function renderPicker(){
  const o=byId(R.out);
  if(!o) return;
  const browse=R.pos!==o.pos;
  const all=pickerCandidates();
  let list=all.slice().sort((a,b)=>b.xp-a.xp);
  if(R.within&&!browse) list=list.filter(c=>affordGap(c)>=0);

  const B=pickerBudget();
  /* Split rather than nested. "N of M clear the gate" counts the ones the reader can
     actually buy; "K more clear it above your budget" counts the rest. Counting ALL
     clearers in the first figure and the unaffordable ones again in the second says
     "1 clear the gate · 1 more above your budget" when there is exactly one, which reads
     as two and is the panel contradicting itself on one line. */
  const clears=all.filter(c=>clearsGate(c.xp-o.xp));
  const clearing=clears.filter(c=>affordGap(c)>=0).length;
  const clearingAbove=clears.length-clearing;

  const note=browse
    ? `<div class="marketnote rule">A transfer is like-for-like — the squad is always two
       keepers, five defenders, five midfielders and three forwards, so ${esc(o.n)} can only be
       replaced by a ${esc(R.pos)}. These are shown for reference and cannot be bought here.
       To change a ${esc(R.pos)}, open <b>Replace him…</b> on one of yours.</div>`
    : `<div class="marketnote"><span class="gate pass"></span>
       <b>${clearing}</b> of ${all.length} clear the +${gateOf().toFixed(2)} gate
       ${clearingAbove?`<span class="sep">·</span> <b>${clearingAbove}</b> more clear it above your budget`:''}</div>`;

  const rows=list.map(c=>pickerRow(c,o,browse)).join('');
  const empty=list.length?'':pickerEmpty(o);

  document.getElementById('sheet').innerHTML=`
   <header>
     <div class="who"><b>Replace ${esc(o.n)}</b>
       <span class="dim">${esc(o.pos)} · ${esc(o.club)} · ${o.xp.toFixed(2)} xPts/gw · sells for £${sellPriceOf(o).toFixed(1)}m</span>
     </div>
     <button class="btn icon ghost" id="pkclose" aria-label="Close">✕</button>
   </header>
   <div class="body">
     <div class="repmath">
       sells <b>£${sellPriceOf(o).toFixed(1)}m</b> <span class="op">+</span> bank <b>£${bankOf().toFixed(1)}m</b>
       <span class="op">=</span> <b>£${B.toFixed(1)}m</b> to spend
       <span class="sep">·</span> gate +${gateOf().toFixed(2)} xPts/gw
       <span class="sep">·</span> Δ vs ${esc(o.n)}, per gameweek
     </div>
     <div class="toolbar" style="margin:10px 0 8px">
       <div class="seg" id="pkpos">
         ${['GKP','DEF','MID','FWD'].map(p=>`<button aria-pressed="${R.pos===p}" data-pos="${esc(p)}">${esc(p)}</button>`).join('')}
       </div>
       ${browse?'':`<button class="btn sm" id="pkafford" aria-pressed="${R.within}">
         <span class="gate${R.within?' pass':''}"></span> ${R.within?`Within £${B.toFixed(1)}m`:'All prices'}
       </button>`}
     </div>
     ${note}
     ${rows}${empty}
     ${list.length?`<div class="moreline" style="border-top:0;padding:8px">All ${list.length} shown</div>`:''}
     ${R.sel&&!browse?pickerStage(byId(R.sel),o):''}
   </div>`;
  wirePicker();
}

function pickerRow(c,o,browse){
  const d=+(c.xp-o.xp).toFixed(2), clears=clearsGate(d), gap=affordGap(c), oc=overClub(c);
  const av=(c.availability===undefined?1:c.availability);
  return `<button class="reprow${R.sel===c.id?' on':''}${browse?' browse':''}" data-id="${c.id}">
    <span class="g">${browse?'':`<span class="gate${clears?' pass':''}"
      title="${clears?'clears':'below'} the +${gateOf().toFixed(2)} transfer gate"></span>`}</span>
    <span class="n"><b>${esc(c.n)}</b><span class="club">${esc(c.club)}</span>
      ${c.ov?`<span class="pill ov">set: ${esc((c.ov.t||'').toLowerCase())}</span>`:''}
      ${av===0?`<span class="pill bad">ruled out</span>`:av<1?`<span class="pill warn">${Math.round(av*100)}% fit</span>`:''}
      ${oc?`<span class="pill bad">4th ${esc(c.club)} — over the club limit</span>`:''}</span>
    <span class="m">£${c.pr.toFixed(1)}m ${roleChip(c.role,true)} ${c.own.toFixed(1)}% ${fdrHtml(c.club,3,S.gw)}
      ${gap<0?`<span class="short">needs +£${Math.abs(gap).toFixed(1)}m</span>`:''}</span>
    <span class="x"><b class="xp">${c.xp.toFixed(2)}</b>
      ${browse?'':`<span class="dd ${clears?'dpos':'dneg'}">${d>0?'+':''}${d.toFixed(2)}</span>`}</span>
    ${c.news?`<span class="news">${esc(c.news)}</span>`:''}
  </button>`;
}

function pickerEmpty(o){
  const cheapest=pickerCandidates().slice().sort((a,b)=>a.pr-b.pr)[0];
  const gap=cheapest?Math.abs(affordGap(cheapest)).toFixed(1):null;
  return `<div class="empty">
    <div class="big">Nothing at £${pickerBudget().toFixed(1)}m</div>
    <p>Selling ${esc(o.n)} raises £${sellPriceOf(o).toFixed(1)}m and the bank adds £${bankOf().toFixed(1)}m.
    ${cheapest?`The cheapest ${esc(R.pos)} on the market is £${cheapest.pr.toFixed(1)}m — £${gap}m short.`:''}</p>
    <button class="btn sm" id="pkwiden">Show the ${esc(R.pos)}s you can’t afford</button>
  </div>`;
}

function pickerStage(c,o){
  if(!c) return '';
  const d=+(c.xp-o.xp).toFixed(2), clears=clearsGate(d), gap=affordGap(c);
  return `<div class="stagebar">
    <div class="move"><span class="out">${esc(o.n)} £${sellPriceOf(o).toFixed(1)}m</span> <span class="op">→</span>
      <span class="in"><b>${esc(c.n)}</b> £${c.pr.toFixed(1)}m</span>
      <span class="sep">·</span> ${gap>=0?`leaves <b>£${gap.toFixed(1)}m</b> in the bank`
        :`<span class="short" style="display:inline;margin:0">needs +£${Math.abs(gap).toFixed(1)}m — raise it by selling elsewhere first</span>`}</div>
    <div class="verdict ${clears?'pass':'miss'}">Δ ${d>0?'+':''}${d.toFixed(2)} xPts/gw —
      ${clears?`clears the +${gateOf().toFixed(2)} gate`:`below the +${gateOf().toFixed(2)} gate`}</div>
    <div class="acts">
      <button class="btn primary" id="pkgo" ${gap<0||overClub(c)?'disabled':''}>Make this transfer</button>
      <button class="btn ghost" id="pkcancel">Cancel</button>
    </div>
  </div>`;
}

function wirePicker(){
  const close=()=>{
    document.getElementById('scrim').classList.remove('open');
    if(location.hash.startsWith('#replace-')) history.replaceState(null,'','#pitch');
  };
  document.getElementById('pkclose').onclick=close;

  document.getElementById('pkpos').onclick=e=>{
    const b=e.target.closest('button'); if(!b) return;
    R.pos=b.dataset.pos; R.sel=null; renderPicker();
  };
  const aff=document.getElementById('pkafford');
  if(aff) aff.onclick=()=>{ R.within=!R.within; renderPicker(); };
  const widen=document.getElementById('pkwiden');
  if(widen) widen.onclick=()=>{ R.within=false; renderPicker(); };

  document.querySelectorAll('.reprow').forEach(b=>b.onclick=()=>{
    if(b.classList.contains('browse')){
      /* Refused, not undone -- the pitch's own idiom for an illegal move. */
      flashInvalid();
      return;
    }
    R.sel=+b.dataset.id; renderPicker();
  });

  const cancel=document.getElementById('pkcancel');
  if(cancel) cancel.onclick=()=>{ R.sel=null; renderPicker(); };

  const go=document.getElementById('pkgo');
  if(go) go.onclick=()=>{
    const out=R.out, into=R.sel;
    close();
    /* The squad changes, so the server decides what the eleven becomes. The client sends
       the fifteen with one man swapped and redraws from the answer. */
    save(pending=>{
      const outCode=codeOf(out), inCode=codeOf(into);
      pending.squad=(pending.squad||[]).map(c=>c===outCode?inCode:c);
      /* The lineup mentions a player who has left, so it is dropped rather than repaired
         here: the server falls back to the model's arrangement for the new fifteen, which
         is the right default for a squad the reader has just changed. */
      pending.xi=undefined; pending.bench=undefined;
      pending.cap=undefined; pending.vc=undefined;
    });
  };
}
