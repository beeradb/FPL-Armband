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

   The prototype carried twelve hardcoded collections. They are all server-side now
   except one, which stays because it is design data rather than model output: CLUBC (club
   colours). The suggested-override library that used to live here went with authoring —
   "no custom overrides at this point".

   Nothing here recomputes a model quantity. The role band, the delta against the
   weakest starter, whether a candidate clears the transfer gate and whether an
   override needs re-checking are all decided in Go and copied across. The design
   note believes the re-check rule is fourteen days; it is seven, and the client
   is not the place to discover that. */
let FIX={};        /* club -> [[opponent, 'H'|'A', fdr], ...] */
let P=[];          /* the fifteen */
let POOL=[];       /* the market */
let OV=[];         /* standing overrides live in THIS document */
/* EXCL is Market.Excluded -- players a standing override keeps out of the market entirely,
   already sent alongside POOL and until now thrown away. Same Override shape as OV, mapped
   the same way, so the left-out strip and "Your instructions" never disagree about what a
   field means. */
let EXCL=[];
/* OV_CACHE remembers every config-sourced override this page has ever seen, by code, across
   reloads. It exists for exactly one reason: a dismissed override is removed from
   cfg.Roster BEFORE the server builds Reasoning, so it is not merely filtered out of the
   next document -- it is entirely ABSENT from it. News has to show that row greyed out
   with "Use it again", and the only place left holding what it said is this cache. Session
   locks and blocks are never cached here; they are the reader's own and "removing" one is a
   real deletion, not a suppression.

   ⚠️ It MUST be backed by localStorage, not just a module-level variable. The dismissal
   itself lives in the fpl_session cookie and survives a reload; a plain JS object does not
   -- it re-initialises to {} on every page load, which is before hydrate() has a chance to
   see the code again (the server already stopped sending it). A first version of this cache
   was in-memory only, which meant the row this comment promises stays visible actually
   vanished on the very first F5 after an Ignore, silently. Wrapped in try/catch: a reader
   with storage disabled or in a locked-down private-browsing mode gets the old in-memory-only
   behaviour rather than a thrown error breaking the page. */
const OV_CACHE_KEY='fpl_ov_cache';
function loadOvCache(){
  try{ return JSON.parse(localStorage.getItem(OV_CACHE_KEY)||'{}')||{}; }
  catch(e){ return {}; }
}
function saveOvCache(){
  try{ localStorage.setItem(OV_CACHE_KEY, JSON.stringify(OV_CACHE)); }
  catch(e){ /* storage unavailable -- degrade to in-memory only, silently */ }
}
let OV_CACHE=loadOvCache();
let BLIND=[];      /* where the model says it is blind */
let GWS=[];        /* the rail: current + upcoming, length is data-driven */
let WEAKEST={};    /* position -> the weakest starter's score */
let BENCHMARKS=[]; /* the same, with the name and price the legend prints */
let TODAY=new Date();
let STATE=null;    /* the raw document, for anything not mapped below */

/* NEWS carries the two freshness strings the News tab prints. Both are formatted BY THE
   SERVER, for the same reason app.js:1388 (see renderNews) has always given for staleness:
   a relative "3 minutes ago" is a clock reading, and the client does not own the clock.
   checked covers FPL's own status feed; readChecked is a second, independent cadence for
   the team news a person reads and passes to the model -- LAST READ ONLY, no "next read
   at", because there is no scheduler behind it yet (NOTES.md §3). */
let NEWS={checked:'', readChecked:''};

/* CHIPWIN is the two chip-window facts the client is not allowed to invent: the last
   gameweek of the current window, and how many chips are genuinely unspent IN it --
   counting ones already played, which GWS (current + upcoming only) cannot see. The one
   arithmetic the client performs on these is a subtraction of two gameweek integers, in
   chipWeeksLeft() below -- a calendar fact, not a competition rule. See app.js:625's old
   defect, fixed by this pair. */
let CHIPWIN={endsGw:null, remaining:null};

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
    /* mn is Player.Minutes -- the numeric expected-minutes figure every arithmetic
       consumer (sort keys, Math.round in the sheet, renderNews's effect line) needs.
       It used to read modelled_minutes, Player.ModelledMinutes's PRE-FORMATTED STRING
       ("90 → 54 modelled") -- p.mn silently became a string and every arithmetic use
       of it produced NaN.

       ⚠️ Fixed independently on two branches at once, against the same ambiguous line
       in the brief -- the server was told a formatted string was acceptable, the client
       was told the field was the number. Neither half was wrong alone. The branch this
       was merged with went further and deleted ModelledMinutes server-side entirely
       ("one quantity in two shapes" -- the client's own row template already draws the
       arrow), so there is no formatted string to carry any more; every consumer reads
       p.mn and formats its own sentence. */
    xp:p.xp, p90:p.per90, mn:p.minutes,
    rel:p.reliability, own:p.ownership,
    role:p.role, status:p.status, news:p.news, fixtures:p.fixtures||[],
    /* XP per million, from analysis.PlayerMetrics.ValueScore. Used to be xpFor(p)/p.pr in
       the sheet -- one of three client surfaces computing a model quantity; this closes it. */
    value:p.value_score,
    /* The multiplier FPL's availability flag produces, carried rather than inferred from
       status: re-deriving it would be a second copy of a table that already exists, and
       its most important value is 0 -- a ruled-out player, whose score is zero for that
       reason and no other. */
    availability:p.availability,
    /* Sort keys must be numbers the server already sent -- see the sortable-tables
       comment near renderPlayers. avgFdr is Player.AvgDifficulty, already decided. */
    avgFdr:p.avg_fdr,
    /* xg90/xa90 are FPL's own expected figures, not the points scoring makes of them.
       dc is Player.DefConChance, 0-1 or undefined -- undefined means the model does not
       price the term for this player (goalkeepers), which is a different fact from zero
       and must render as "—", never "0%". */
    xg90:p.xg_per_90, xa90:p.xa_per_90, dc:p.defensive_contribution_chance,
    /* ov is what v1 draws as news, not as a hand-set override -- see the News tab and the
       player card's .pcnews band. `eff` is NewsItem.effect, {label,was,now,direction},
       pre-formatted by the server; undefined until the field ships, and undefined means
       "do not render a before/after", never a fabricated one. */
    ov:p.override ? {
      t:p.override.label, why:p.override.reason, set:p.override.set_on,
      lapse:p.override.until, chk:p.override.checked,
      needsCheck:p.override.needs_check, age:p.override.check_age,
      eff:p.override.effect
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
    session:o.session,
    /* NewsItem.effect -- {label,was,now,direction}, pre-formatted by the server. Absent
       until the field ships; absent means the News tab's row renders no before/after
       rather than faking one from xpFor(). */
    eff:o.effect
  }));
  /* Upsert into the cache -- never clear it here. A code that has dropped out of `live`
     this time might be one the reader just ignored, and the cache is what lets News keep
     drawing that row. Only a config-sourced (non-session) entry is worth remembering: a
     session lock or block that disappears is genuinely gone.

     Persisted to localStorage on every hydrate, not only when something changes -- writing
     unconditionally is simpler than tracking whether the loop actually touched anything, and
     the payload is a handful of override records, not something worth optimising a write out
     of. See OV_CACHE_KEY's own comment for why persistence is not optional here. */
  for(const o of OV) if(!o.session) OV_CACHE[o.code]=o;
  saveOvCache();

  EXCL=(st.market.excluded||[]).map((o,i)=>({
    id:'x'+i, code:o.code, t:o.label, kind:o.kind, who:o.player, club:o.club,
    set:o.set_on, why:o.reason, session:o.session
  }));

  BLIND=st.blind||[];

  NEWS={checked:(st.news&&st.news.checked)||'', readChecked:(st.news&&st.news.read_checked)||''};
  CHIPWIN={
    endsGw:(st.chips&&st.chips.window_ends_gw!=null)?st.chips.window_ends_gw:null,
    remaining:(st.chips&&st.chips.remaining_in_window!=null)?st.chips.remaining_in_window:null
  };

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
  /* entry/noimp live on st.import, not st.session -- see viewmodel.Import -- but they are
     still session-cookie fields under the hood, and PUT /api/session is a full replace
     (saveSession decodes straight into a fresh session{}). Any save that omitted them would
     silently reset an already-imported entry id, or a reader's earlier "start fresh" choice,
     back to zero on the very next unrelated save (locking a player, toggling a chip, ...). */
  const imp=st.import||{};
  PENDING={
    v:1,
    seed:undefined,          /* the server owns the seed; never sent back */
    opt:!!sess.optimised,
    lock:(sess.locked||[]).slice(),
    excl:(sess.blocked||[]).slice(),
    dis:(sess.dismissed||[]).slice(),
    chips:Object.assign({},sess.chips||{}),
    entry:imp.entry||0,
    noimp:!!imp.skipped
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

  /* Import state: handle the team ID import affordance. */
  const card=document.getElementById('importCard');
  if(card){
    if(!imp.open || imp.skipped || imp.entry) card.hidden=true;
    else card.hidden=false;
  }
  /* Fill in the gameweek placeholders in the import card. */
  const nextGwEl=document.getElementById('importNextGw');
  if(nextGwEl) nextGwEl.textContent=imp.next||'—';
  const exampleGwEl=document.getElementById('importExampleGw');
  if(exampleGwEl) exampleGwEl.textContent=imp.next||'—';

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

/* ============================================================
   STATE
   ============================================================ */
const CHIPS=[
 {k:'wildcard', n:'Wildcard',       ic:'★'},
 {k:'freehit',  n:'Free Hit',       ic:'⇄'},
 {k:'bboost',   n:'Bench Boost',    ic:'▤'},
 {k:'3xc',      n:'Triple Captain', ic:'3×'}
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
  locks:new Set(), blocks:new Set(),
  swapFrom:null,
  /* Whether the chip catalogue is open. Kept here, not local to renderChips(), so it
     survives the re-render every save() triggers -- picking a chip should not close the
     popover the reader just picked it from. moveConfirm names the chip whose "move it
     here?" strip is open, or null. */
  chipOpen:false, moveConfirm:null,
  /* armLock names the market row currently armed for a Lock in confirm, or null. Locking a
     player you do not own rebuilds your whole fifteen around him -- too large a consequence
     for one click on a small button -- so the row arms first, the same two-step pattern
     moveConfirm already uses for a planned chip. Any other click disarms it. */
  armLock:null,
  posFilter:'ALL', q:'', affordOnly:false, showAll:false,
  modelXi:[],
  /* Sort and filter are independent axes and sort survives a filter change -- one state
     field for both the desktop headers and the phone's sort pill. */
  sort:{col:'delta',dir:'desc'}
};

/* ============================================================
   SORTABLE TABLES — every key here is a number the server already sent.
   Sorting is arranging. A key the client works out (e.g. "afford", assembled from bank and
   a weakest starter) is the governing-rule violation arriving through a new door, so it is
   not offered as a column. avg_fdr is the one exception worth naming: it looks derived but
   Player.AvgDifficulty is already decided in Go and merely carried across.
   ============================================================ */
const SORT_DEFAULT_DIR={name:'asc',pos:'asc',avg_fdr:'asc',minutes:'desc',ownership:'desc',price:'desc',xp:'desc',delta:'desc',xg90:'desc',xa90:'desc',defcon:'desc'};
function sortVal(p,col){
  switch(col){
    case 'name': return (p.n||'').toLowerCase();
    case 'pos': return p.pos||'';
    case 'avg_fdr': return p.avgFdr===undefined?99:p.avgFdr;
    case 'minutes': return p.mn||0;
    case 'ownership': return p.own||0;
    case 'price': return p.pr||0;
    case 'xp': return p.xp||0;
    case 'delta': return p.d||0;
    case 'xg90': return p.xg90||0;
    case 'xa90': return p.xa90||0;
    /* dc is undefined for a goalkeeper (the model does not price the term for him) --
       sorted to the bottom on a descending sort, which is where "not priced" belongs. */
    case 'defcon': return p.dc===undefined||p.dc===null?-1:p.dc;
    default: return 0;
  }
}
function applySort(list){
  const {col,dir}=S.sort, mult=dir==='asc'?1:-1;
  return list.slice().sort((a,b)=>{
    const av=sortVal(a,col), bv=sortVal(b,col);
    if(av<bv) return -1*mult; if(av>bv) return 1*mult; return 0;
  });
}
/* Click → sort by that column in its own first direction. Click again → reverse. A third
   click does not return to unsorted -- one way to do things; a hidden third state is how a
   table starts feeling arbitrary. */
function setSort(col){
  if(S.sort.col===col) S.sort.dir = S.sort.dir==='asc'?'desc':'asc';
  else S.sort={col, dir:SORT_DEFAULT_DIR[col]||'desc'};
  renderPlayers();
}
document.querySelectorAll('#ptable [data-sort]').forEach(b=>{
  b.onclick=()=>setSort(b.dataset.sort);
});
const sortPill=document.getElementById('sortPill');
if(sortPill) sortPill.onchange=()=>{
  const [col,dir]=sortPill.value.split(':');
  S.sort={col,dir};
  renderPlayers();
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
  return toggleCorrectionByCode(codeOf(id), kind);
}

/* toggleCorrectionByCode is the code-keyed core toggleCorrection wraps. A caller that
   already holds the permanent code -- the Left-out panel and Your-instructions undo
   buttons, both rendering from server-sent Override records -- calls this directly rather
   than routing through codeOf(codeToId(code)). That round trip depends on the player still
   being findable in P or POOL, and a session-excluded market player is in neither: the
   market row strips him out (see cmd/armband/page.go's watchlistFor) the moment the
   exclusion takes effect, which is exactly when Undo needs to reach him. */
function toggleCorrectionByCode(code, kind){
  if(!code){ notify('That player has no code, so we can’t save that.'); return; }
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
/* variant: undefined/false for the full word, true (legacy) or 'sm' for the short word, or
   'dot' for a bare colour dot with no label -- the market table's Role column, which sheds
   the word entirely and leans on the `.rolekey.always` legend above it instead. `.role.bare`
   (armband.css) is the same declaration the pitch card's own narrow-width rule uses. */
const roleChip=(band,variant)=>{const r=role(band);
  const sm=variant===true||variant==='sm', dot=variant==='dot';
  const cls=`role ${r.c}${sm?' sm':''}${dot?' bare':''}`;
  const attrs=dot?` title="${esc(r.l)}" aria-label="Role: ${esc(band||'unknown')}"`:'';
  return `<span class="${cls}"${attrs}>${esc(sm?r.s:r.l)}</span>`;};

/* The same band, as a number -- 1 (nailed) through 5 (fringe) -- for riskRows() below,
   which has to filter and sort the fifteen rather than just colour a chip. Reads the same
   ROLES table role() does, so the two can never disagree about which band a string names. */
const ROLE_NUM={'nailed':1,'likely starter':2,'rotation risk':3,'squad player':4,'fringe':5};
const roleNum=band=>ROLE_NUM[band]||5;

/* The strip for `n` gameweeks starting at `from`, which is a GAMEWEEK NUMBER and not an
   index into the fixture list. */
function fdrHtml(club,n=5,from=S.gw){
  const all=FIX[club]||[];
  const f=all.filter(x=>x.gw>=from && x.gw<from+n);
  // pad to a constant width so card rhythm survives the end of the horizon
  const pad=Array(Math.max(0,n-f.length)).fill(null);
  return '<span class="fdr">'+
    f.map(x=>`<i class="f${x.fdr}" title="${esc(x.opp)} (${esc(x.ha)}) difficulty ${x.fdr}">${x.fdr}</i>`).join('')+
    pad.map(()=>'<i class="blank" title="no fixture scheduled this far out">·</i>').join('')+
    '</span>';
}

/* One figure, one formatting, two consumers: the market table's cell (the column header
   already states the label, and `.ptable .stat` hides the inline one -- armband.css) and
   the mobile card's meta line (no header, so the label stays inline). v===undefined/null
   means the model does not price the term for this player -- rendered as "—", not a
   fabricated zero, which is a different claim (see PlayerMetrics.DefConChance). */
function statHtml(label,v,dp=2){
  const val=(v===undefined||v===null)?'—':(+v).toFixed(dp);
  return `<span class="stat"><i class="k">${esc(label)}</i><b>${val}</b></span>`;
}
function pctHtml(label,v){
  const val=(v===undefined||v===null)?'—':Math.round(v*100)+'%';
  return `<span class="stat"><i class="k">${esc(label)}</i><b>${val}</b></span>`;
}
/* The fixture ribbon for the player sheet's header -- "Coming up", drawn once, replacing
   both the old .fdr chip strip AND the mono line that repeated the same five fixtures
   underneath it (P-14, deleted). Kept apart from fdrHtml(), which the pitch cards still
   use unchanged: this is a wider cell carrying the opponent's own code, not a bare digit.

   HOME AND AWAY ARE ONE GLYPH -- away is @MCI, home is bare MCI, and there is no legend.
   Colour is difficulty and nothing else; the previous solid-vs-outlined construction
   spent colour on a second meaning and is deleted, not merely superseded (NOTES.md §1).
   Padded to a constant cell count with .blank, exactly as fdrHtml() does. */
function ribbon(club,n,from){
  n=n||5; from=from===undefined?S.gw:from;
  const all=FIX[club]||[];
  const f=all.filter(x=>x.gw>=from && x.gw<from+n);
  const pad=Array(Math.max(0,n-f.length)).fill(null);
  return '<span class="fxr">'+
    f.map(x=>{
      const away=x.ha==='A';
      return `<i class="f${x.fdr}" title="${esc(x.opp)} ${away?'away':'home'}, difficulty ${x.fdr}">`
        +`${away?'<span class="at">@</span>':''}${esc(x.opp)}</i>`;
    }).join('')+
    pad.map(()=>'<i class="blank" aria-hidden="true">·</i>').join('')+
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

/* One bank figure, one over-budget rule, every place the committed total renders -- the
   Players tab readout, the Pitch tab's score bug, and the pitch HUD's pill. The server
   never refuses an over-budget fifteen (validateSession, webroutes.go, deliberately does
   not check budget), so a negative bank is a real, legitimate state and not a bug to hide;
   this is the one place that decides how it reads rather than three copies agreeing by
   accident. */
function bankHtml(){
  const b=bankOf(), bad=b<0;
  return {bad, text:(bad?'−':'')+'£'+Math.abs(b).toFixed(1)+'m'};
}
function clubCounts(){const m={};P.forEach(p=>m[p.club]=(m[p.club]||0)+1);return m;}

/* ============================================================
   RENDER — gameweek rail
   ============================================================ */
/* weeksLeft/urgency are the chip control's clock: pressure = weeks left in the window
   divided by chips still unspent in it. A fixed gameweek cannot do this job -- "week 10"
   is early for a reader who has spent three chips and late for one who has spent none.
   Both inputs are CHIPWIN, server-sent (see hydrate()); the only arithmetic here is a
   subtraction of two gameweek integers, a calendar fact and not a competition rule. */
function chipWeeksLeft(){ return CHIPWIN.endsGw!=null ? CHIPWIN.endsGw-S.gw+1 : null; }
function chipUrgency(){
  const left=CHIPWIN.remaining, w=chipWeeksLeft();
  if(!left || w===null) return '';                 /* nothing to lose, or no window yet known: stay silent */
  const p=w/left;
  if(p>=3) return '';                               /* quiet */
  if(p>=1.5) return 'due';                          /* an outline */
  return 'due last';                                /* the fill */
}
function renderRail(){
  const el=document.getElementById('gwrail');
  const due=!!chipUrgency();
  el.innerHTML=GWS.map(g=>{
    const c=CHIPS.find(c=>c.k===g.chip);
    /* The window boundary is a fact about the calendar, so it lives on the calendar: the
       19px .chipslot already reserves per week, on the window's last week, quiet ink while
       the pill is quiet and amber when the pill is amber. Because the rail only shows the
       current week and the ones ahead, GW{endsGw} walks into view on its own about five
       weeks out -- the reader meets the deadline before the alarm (NOTES.md §4). */
    const slot=c?`<span class="pill on">${c.ic} ${c.n}</span>`
      :(g.gw===CHIPWIN.endsGw?`<span class="wend${due?' due':''}">Chips end</span>`:'');
    return `<button class="gw${g.live?' live':''}" role="tab" data-gw="${g.gw}"
      aria-selected="${g.gw===S.gw}">
      <div class="n">GW${g.gw}${g.live?' <span class="k" style="letter-spacing:.1em">NOW</span>':''}</div>
      <div class="d">${g.d}</div>
      <div class="chipslot">${slot}</div>
    </button>`;}).join('');
  el.querySelectorAll('.gw').forEach(b=>b.onclick=()=>{S.gw=+b.dataset.gw;renderAll();});
}

/* ============================================================
   RENDER — the chip control
   ============================================================
   Nothing is the default and a chip is the exception, so the resting state is one ghost
   pill in .pitchhud -- no slab, no row of four buttons, no space taken from the team. It
   escalates as its window closes (chipUrgency() above) rather than looking the same in
   GW3, when there is nothing to do, as in GW17, when two chips are about to evaporate.
   Replaces the deleted <details class="chipfold"> and its #chiprow.
   ============================================================ */
/* Every chip this week, in the state the row has to draw. `on` is running now; `planned`
   is the READER'S plan for a different week -- not disabled, selecting it moves the plan
   here (see wireChips' .cmrow handler); `closed` is everything the reader cannot act on,
   which is either FPL's own week.playable refusal or a chip already spent. The client
   cannot always tell those two apart (there is no "spent in GWn" field on the wire), so a
   closed row states only what it knows for certain: this chip is not offered this week. */
function chipListFor(week){
  const cur=week.chip;
  const offered=new Set((week.playable||[]).map(c=>c.k));
  return CHIPS.map(c=>{
    const on=cur===c.k;
    const plannedGw=on?null:((GWS.find(g=>g.gw!==S.gw && g.chip===c.k)||{}).gw||null);
    return {k:c.k, n:c.n, ic:c.ic, on, planned:!!plannedGw, plannedGw, playable:offered.has(c.k)};
  });
}
function chipExplain(k){
  return {
    bboost:`Bench Boost: all fifteen score. Your bench adds ${benchPts().toFixed(1)} pts and the order stops mattering, so pick for points, not safety.`,
    '3xc':`Triple Captain: ${esc(byId(S.cap).n)} scores three times over — ${(xpFor(byId(S.cap))*3).toFixed(1)} pts.`,
    wildcard:`Wildcard: change as many players as you like, with no points deducted. You still have to afford them. The Players tab becomes a full rebuild.`,
    freehit:`Free Hit: a team for one week only. After GW${S.gw} you get your old fifteen back — nothing you buy here sticks.`
  }[k];
}
function cmRowHtml(c,week){
  const closed=c.on?false:(c.planned?false:!c.playable);
  const state=c.on?'running this week'
    : c.planned?`planned for GW${c.plannedGw}`
    : !c.playable?`FPL doesn’t offer this in GW${S.gw}`
    : 'available';
  const cls=closed?' closed':c.planned?' placed':'';
  const confirmOpen=S.moveConfirm===c.k;
  return `<button class="cmrow${cls}" type="button" role="menuitemradio" data-chip="${c.k}"
      ${c.planned?`data-move="${c.plannedGw}"`:''}
      aria-pressed="${c.on}" ${closed?'disabled':''}>
      <span class="ic" aria-hidden="true">${c.on?'✓':c.ic}</span>
      <span><span class="t-row cmname">${esc(c.n)}</span><span class="t-meta">${esc(state)}</span></span>
      ${c.planned?`<span class="gwtag">Move here</span>`:`<span class="tick">${c.on?'✓':''}</span>`}
    </button>
    ${c.planned?`<div class="cmconfirm"${confirmOpen?'':' hidden'} data-for="${c.k}">
      <span class="t-body">Move your ${esc(c.n)} from GW${c.plannedGw} to GW${S.gw}?</span>
      <button class="btn sm ghost" type="button" data-keep="${c.k}">Keep GW${c.plannedGw}</button>
      <button class="btn sm" type="button" data-moveok="${c.k}">Move it</button>
    </div>`:''}`;
}
function chipMenuHtml(week){
  const list=chipListFor(week), cur=week.chip;
  const u=chipUrgency(), w=chipWeeksLeft();
  const left=CHIPWIN.remaining, size=4;
  return `<div class="chipmenu"${S.chipOpen?'':' hidden'} role="menu">
    <div class="cmhead">
      <span class="t-label">Play a chip in GW${S.gw}</span>
      <span class="sp"></span>
      <span class="t-meta">${left==null?'—':left} of ${size} left</span>
      <span class="t-meta cmwindow">${CHIPWIN.endsGw==null?'The chip window has not loaded yet.'
        :`This window ends after GW${CHIPWIN.endsGw}. Unused chips do not carry over.`}</span>
    </div>
    ${u&&left!=null&&w!=null?`<div class="cmwarn"><span class="g" aria-hidden="true">!</span>
      <span class="t-body">${left} unspent, ${w} gameweek${w===1?'':'s'} left in this window.</span></div>`:''}
    ${list.map(c=>cmRowHtml(c,week)).join('')}
    ${cur?`<p class="cmnote t-body acc">${esc(chipExplain(cur))}</p>`:''}
    <div class="cmfoot"><span class="t-meta">You get a few of these for the whole season, so most weeks the answer is none. Pick one and your points re-run with it on.</span></div>
  </div>`;
}
function chipPillHtml(week){
  const cur=week.chip, c=CHIPS.find(x=>x.k===cur);
  const u=chipUrgency(), left=CHIPWIN.remaining, w=chipWeeksLeft();
  return `<button class="chippill${c?' set':u?' '+u:''}" type="button"
      aria-expanded="${S.chipOpen}" aria-haspopup="menu">
      <span>Chip</span>
      ${c?`<b>${esc(c.n)}</b>`
        :u&&left!=null?`<span class="lft">${left} left</span><span class="by">· by GW${CHIPWIN.endsGw}</span>`
        :`<span class="dash">none</span>`}
      <span class="car" aria-hidden="true"></span>
    </button>`;
}
function renderChips(){
  const week=gwState()||{};
  const el=document.getElementById('chipctl');
  if(!el) return;
  el.innerHTML=chipPillHtml(week)+chipMenuHtml(week);
  wireChips(el,week);
}
function wireChips(el,week){
  const pill=el.querySelector('.chippill'), menu=el.querySelector('.chipmenu');
  const scrim=document.getElementById('chipscrim');
  pill.onclick=e=>{
    e.stopPropagation();
    S.chipOpen=!S.chipOpen;
    if(!S.chipOpen) S.moveConfirm=null;
    renderChips();
  };
  if(scrim){ scrim.classList.toggle('on',S.chipOpen); scrim.onclick=()=>{S.chipOpen=false;S.moveConfirm=null;renderChips();}; }
  el.querySelectorAll('.cmrow').forEach(r=>r.onclick=e=>{
    e.stopPropagation();
    if(r.classList.contains('placed')){
      /* The reader's own plan, so it may move -- but a wildcard is a season-defining
         decision and it may not relocate on a mis-tap. One in-place confirm, naming both
         gameweeks (NOTES.md §4). */
      S.moveConfirm = S.moveConfirm===r.dataset.chip ? null : r.dataset.chip;
      renderChips();
      return;
    }
    if(r.disabled) return;
    const chip = r.getAttribute('aria-pressed')==='true' ? null : r.dataset.chip;
    save(pending=>{
      pending.chips=Object.assign({},pending.chips||{});
      if(chip) pending.chips[String(S.gw)]=chip; else delete pending.chips[String(S.gw)];
    });
  });
  el.querySelectorAll('[data-keep]').forEach(b=>b.onclick=e=>{
    e.stopPropagation(); S.moveConfirm=null; renderChips();
  });
  el.querySelectorAll('[data-moveok]').forEach(b=>b.onclick=e=>{
    e.stopPropagation();
    const chip=b.dataset.moveok;
    const from=(GWS.find(g=>g.chip===chip)||{}).gw;
    S.moveConfirm=null;
    /* One write, not two: the plan leaves the week it was in and lands in the current
       one. */
    save(pending=>{
      pending.chips=Object.assign({},pending.chips||{});
      if(from) delete pending.chips[String(from)];
      pending.chips[String(S.gw)]=chip;
    });
  });
}
/* Registered once, not per render (wireChips runs on every save) -- a listener added again
   on every redraw would fire the same Escape press once per render since boot. */
document.addEventListener('keydown',e=>{
  if(e.key==='Escape' && S.chipOpen){ S.chipOpen=false; S.moveConfirm=null; renderChips(); }
});

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

  // Captain is off the score bug now -- his arithmetic lives on the pitch HUD's armband
  // pill (renderPitch), which renders at every width, so nothing is lost on a phone: the
  // bug used to hide the pre-doubled figure under 720px and show only the total.
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
   <div class="sb-cell"><span class="k">Bench</span>
     <div class="v">${benchPts().toFixed(1)}<small>pts</small></div>
     <div class="sub">${chip==='bboost'?'counting':'not counting'}</div></div>
   <div class="sb-div"></div>
   <div class="sb-cell"><span class="k">In the bank</span>
     <div class="v${bankHtml().bad?' badc':''}">${bankHtml().text}</div>
     <div class="sub${bankHtml().bad?' badc':''}">${bankHtml().bad?'over budget':'squad £'+spend().toFixed(1)+'m'}</div></div>
   <div class="sb-div"></div>
   <div class="sb-cell"><span class="k">Chip</span>
     <div class="v">${c?c.n:'None'}</div>
     <!-- ⚠️ Was (4 - GWS.filter(g=>g.chip).length) + ' of 4 left' -- GWS is the rail,
          current + upcoming (app.js:93), so a chip already spent earlier in the window
          was never counted and the figure overstated what the reader had; the 4 was
          hardcoded with no concept of the two windows; and it was the client deciding a
          competition rule. CHIPWIN.remaining is server-sent and scoped to the window
          (NOTES.md §6). -->
     <div class="sub">${c?'in the number above':(CHIPWIN.remaining==null?'—':CHIPWIN.remaining+' left this season')}</div></div>
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
    ? `<span class="dim">matches our pick</span>`
    : `<span class="${vsm>0?'acc':'badc'}">${vsm>0?'+':''}${vsm.toFixed(2)}</span> <span class="dim">vs our pick</span>`;

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
   RENDER — the pitch
   ============================================================ */
function cardHtml(p,opts={}){
  const lock=S.locks.has(p.id), block=S.blocks.has(p.id);
  const isC=S.cap===p.id, isV=S.vc===p.id;
  const chip=gwState().chip;
  const x=xpFor(p), mult=chip==='3xc'?3:2;
  /* The FPL-news glyph: an injured or suspended player looks identical to a healthy one
     everywhere else on the pitch. availability's most important value is 0 -- a ruled-out
     player, whose score is zero for that reason and no other -- so a card below 1 gets a
     corner marker rather than just a smaller number and no reason. */
  const av=p.availability===undefined?1:p.availability;
  return `<div class="card${lock?' haslock':''}${block?' hasblock':''}${S.swapFrom===p.id?' sel':''}${isC?' iscap':''}${isC&&chip==='3xc'?' tcap':''}${isV?' isvc':''}"
     draggable="true" data-id="${p.id}" style="--clubc:${CLUBC[p.club]||'#39506A'}">
    <div class="shirt">${isC?`<span class="bandc">${chip==='3xc'?'3×':'C'}</span>`:''}</div>
    ${isC?`<span class="armchip${chip==='3xc'?' tc':''}">${chip==='3xc'?'3×':'C'}</span>`:''}
    ${isV?`<span class="armchip v">V</span>`:''}
    ${av<1?`<span class="newsflag${av===0?' bad':''}" title="${av===0?'Ruled out':Math.round(av*100)+'% fit'} — see News">!</span>`:''}
    <div class="chead">
      <span class="lhs"><span class="cl">${esc(p.club)}</span></span>
      <div class="acts">
        <button class="iconbtn${lock?' on':''}" data-act="lock" data-id="${p.id}" title="Lock into the squad — auto-rebuilds keep him">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><rect x="4" y="10" width="16" height="11" rx="2"/><path d="M8 10V7a4 4 0 018 0v3"/></svg></button>
        <button class="iconbtn block${block?' on':''}" data-act="block" data-id="${p.id}" title="Block — never picked, even on a rebuild">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><circle cx="12" cy="12" r="9"/><path d="M6 6l12 12"/></svg></button>
      </div>
    </div>
    <div class="nm">${esc(p.n)}</div>
    <div class="meta"><span>£${p.pr.toFixed(1)}</span>${roleChip(p.role,true)}</div>
    <div class="xp">${isC
      ?`<span class="pre">${x.toFixed(2)}</span><span class="arw">→</span><b>${(x*mult).toFixed(2)}</b><span class="u">pts</span>`
      :`<b>${x.toFixed(2)}</b><span class="u">pts</span>`}</div>
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
  // The "legal / no" badge is gone. Validity is a REFUSAL, not a status: a bad drag is
  // flashed by flashInvalid(), stating the rule; the readout only carries an error state
  // when the eleven is PERSISTENTLY illegal (a stored arrangement can round-trip into that
  // state with no drag involved — see bench-slots-ignore-formation-legality).
  const ok=legal(), fe=document.getElementById('formErr');
  fe.hidden=ok;
  if(!ok) fe.textContent='· '+illegalReason();
  document.getElementById('pitchhud').classList.toggle('illegal',!ok);
  document.getElementById('capName').textContent=byId(S.cap).n;
  const chip=gwState().chip, mult=chip==='3xc'?3:2;
  const capX=xpFor(byId(S.cap));
  document.getElementById('capMath').textContent=`${capX.toFixed(2)} → ${(capX*mult).toFixed(2)}${chip==='3xc'?' ×3':''}`;
  document.getElementById('vcName').textContent=S.vc?byId(S.vc).n:'none set';
  // Rendered only when violated -- like the formation error above, this pill can only ever
  // say ✓, because the picker refuses a fourth from one club at the point of action.
  const cc=clubCounts(), over=Object.entries(cc).filter(([,n])=>n>3);
  const cap=document.getElementById('clubcap');
  cap.hidden=over.length===0;
  if(over.length){ cap.className='pill bad'; cap.textContent=`${over[0][0]} ${over[0][1]}/3 — over the club limit`; }
  // The squad is where an over-budget state actually lives, so the pill sits here beside
  // the club-limit one -- not refused (the server never refuses this), just marked.
  const bh=bankHtml(), budgetPill=document.getElementById('budgetPill');
  budgetPill.hidden=!bh.bad;
  if(bh.bad) budgetPill.textContent=`over budget by £${Math.abs(bankOf()).toFixed(1)}m`;
  document.getElementById('benchval').innerHTML=benchPts().toFixed(1)+' <span class="unit">pts</span>';
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
/* The refusal itself: never silently reorder the reader's team. Washes the HUD red AND
   states the rule -- HANDOFF.md's own spec for this was half-implemented (the flash with
   no words), which stranded illegalReason()'s good strings behind a badge that no longer
   exists. Borrows the score bug's 1.4s rhythm so the page keeps one idiom for "something
   just happened". */
let invalidTimer=null;
function flashInvalid(){
  const hud=document.getElementById('pitchhud'), fe=document.getElementById('formErr');
  hud.classList.add('refused');
  fe.hidden=false; fe.textContent='· refused — '+illegalReason();
  clearTimeout(invalidTimer);
  invalidTimer=setTimeout(()=>{
    hud.classList.remove('refused');
    // A persistently illegal arrangement keeps its own error state rather than the flash
    // erasing it; a legal one clears entirely.
    if(legal()) fe.hidden=true;
    else fe.textContent='· '+illegalReason();
  },1400);
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
        toggleCorrection(id, b.dataset.act); return;
      }
      if(S.swapFrom!==null){ doSwap(S.swapFrom,id); S.swapFrom=null; setSwapbar(); return; }
      openSheet(id);
    };
  });
}
/* There is no card-level armband cycling any more. `cycleArmband` and its `.arm-btn` used
   `S.xi.filter(...).sort((a,b)=>xpFor(byId(b))-xpFor(byId(a)))[0]` to pick a fallback
   captain -- the client RANKING PLAYERS BY PROJECTION, one of the surfaces this whole pass
   exists to close. The armband picker (openArmbandPicker, below) is now the only route,
   at every width, and it is server-priced. */
function setSwapbar(){
  const bar=document.getElementById('swapbar');
  if(S.swapFrom===null){bar.classList.remove('on');}
  else{bar.classList.add('on');
    document.getElementById('swaptext').innerHTML=`Tap any player to swap with <b>${esc(byId(S.swapFrom).n)}</b>`;}
  renderPitch();
}
document.getElementById('swapcancel').onclick=()=>{S.swapFrom=null;setSwapbar();};

/* ============================================================
   RENDER — Your instructions
   Replaces the formations rail. Everything acting on the fifteen that a human decided,
   removable in place. `renderShapes()` is deleted, not replaced by a client computation --
   see docs at the head of this file and the design record: it was a second implementation
   of internal/analysis's bestFormation, wrong in a specific checkable way (a plain sum,
   ignoring locks and the three-per-club rule), and a server-computed replacement is filed
   as deferred rather than rebuilt here under a new name.
   ============================================================ */
/* ⚠️ Only the reader's own locks and leave-outs appear here, and that is the whole point of
   the panel now. It used to carry a second group -- config-sourced entries for a player in
   the fifteen, under "Set in your settings file", each with an Ignore button. Those are NEWS,
   provided by the system, and they live on the News tab; listing them under a heading that
   says "Your instructions" asserted the one thing about them that is not true.

   Dropping that group removes the last caller of ignoreOverride() and useAgain(), so both are
   deleted with it, and the suppression capability leaves the product in v1. That is deliberate
   and was taken on the product owner's ruling, not inferred: news is not something the reader
   adjudicates. If it returns, it returns on the News tab where the item is, and the violet
   channel this design reserves is what it should wear. */
function renderInstructions(){
  const mine=OV.filter(o=>o.session);                       // this session's own locks and leave-outs
  const el=document.getElementById('instrbody');
  const count=document.getElementById('instrCount');
  count.textContent=mine.length?`${mine.length} active`:'';
  if(!mine.length){
    el.innerHTML=`<div class="empty" style="padding:22px 16px">
      <div class="big">You haven't told us anything yet.</div>
      <p>This eleven is our own pick. Open any player and <b>Lock in</b> to keep him, or
      <b>Leave out</b> to make sure we never pick him.</p>
    </div>`;
    return;
  }
  /* Past tense, because these label a state that already exists rather than an action -- and
     they are the same two words as the buttons that created it. */
  const verb=o=>o.kind==='exclude'?'Left out':'Locked in';
  el.innerHTML=mine.map(o=>`
    <div class="instrrow">
      <span class="v">${esc(verb(o))}</span>
      <span class="who">${esc(o.who)}<span class="club">${esc(o.club)}</span></span>
      <button class="btn sm ghost" data-instr="undo" data-code="${o.code||0}">Undo</button>
    </div>`).join('');
  el.querySelectorAll('[data-instr]').forEach(b=>b.onclick=()=>{
    const code=+b.dataset.code;
    if(!code){ notify('That player has no code, so we can’t save that.'); return; }
    const o=mine.find(x=>x.code===code);
    if(o) toggleCorrectionByCode(code, o.kind==='exclude'?'block':'lock');
  });
}

/* Where a left-out player goes. He vanishes from the market row list entirely (the server
   drops him rather than the client filtering him), so this strip -- directly under the
   table he left -- is the only place he is still visible. EXCL is Market.Excluded, sent
   alongside POOL and thrown away until now.

   Session entries (this reader's own Leave out) get Undo; config-sourced ones (the
   standing exclude list) show their reason and date instead, because this session did not
   set them and cannot clear them -- the same split renderInstructions makes above. */
function renderLeftOut(){
  const el=document.getElementById('leftout');
  if(!el) return;
  if(!EXCL.length){ el.innerHTML=''; return; }
  el.innerHTML=`
    <h3>Left out of this market <span class="dim">${EXCL.length}</span></h3>
    ${EXCL.map(o=>o.session ? `
      <div class="instrrow">
        <span class="v ovc">Left out</span>
        <span class="who">${esc(o.who)}<span class="club">${esc(o.club)}</span></span>
        <button class="btn sm ghost" data-excl="undo" data-code="${o.code||0}">Undo</button>
      </div>` : `
      <div class="instrrow cfg">
        <span class="v ovc">${esc(o.t||'EXCL')}</span>
        <span class="who">${esc(o.who)}<span class="club">${esc(o.club)}</span>
          ${o.why?`<span class="rz">${esc(o.why)}</span>`:''}</span>
        <span class="k dim">${o.set?`set ${esc(o.set)}`:''}</span>
      </div>`).join('')}`;
  el.querySelectorAll('[data-excl]').forEach(b=>b.onclick=()=>{
    const code=+b.dataset.code;
    if(!code){ notify('That player has no code, so we can’t save that.'); return; }
    toggleCorrectionByCode(code, 'block');
  });
}

/* ignoreOverride() and useAgain() were here and are DELETED, with their last caller.
   They put a player's code on `pending.dis`, which suppressed a config-sourced entry for the
   visit and had the server rebuild the squad without it.

   Nothing suppresses news in v1. The News tab carries no per-row control at all, and the
   instructions panel no longer lists config entries -- see renderInstructions.

   ⚠️ `pending.dis` is still in the wire format and the server still honours it. It is only
   the client that stopped writing to it, so restoring the capability is a UI change and not
   a protocol one. Do not remove it from the contract on the strength of this deletion. */

/* ⚠️ renderBlind() and the Brief tab are DELETED, on the product owner's instruction.
   The tab held two panels: a verdict that was never wired through -- it said so, and
   pointed at the CLI -- and the engine's own list of where it cannot see. A tab whose
   headline feature announces its own absence is not a tab.

   State.blind is still sent and is still read into BLIND here; the contract is untouched,
   so a surface that wants the model's blind spots can render them without a server change.
   Nothing draws them today. */

/* ============================================================
   The player card's depth — /api/player/{code}

   A SECOND endpoint, keyed on the player's permanent code, fetched only when his card
   opens. It carries HISTORY (finished football), never a projection, so it cannot disagree
   with anything State says -- see viewmodel.PlayerDetail's doc comment. Every number below
   is a passthrough or a plain reordering of what the server sent; nothing here computes a
   model quantity, and nothing here needs to -- there is no model quantity in a match log.
   ============================================================ */

/* lastSeasonHtml draws band 3. ls is viewmodel.SeasonSummary or undefined -- undefined
   covers both "still loading" (never reached, the caller shows a skeleton instead) and "FPL
   has no Premier League season on record for him", which is the ordinary case for a
   debutant or a player promoted from a division this feed does not cover. */
/* Eight cells, one grid, plus a meta footer -- not a 2x2 statgrid and two floating lines.
   Goals, assists, xG and xA were always the same KIND of thing as points, minutes, starts
   and pts/90 -- a measured season count -- and printing half of them as grid cells and half
   as loose sentences is exactly why they used to float. Row one is the rate, row two is the
   return, on a slightly darker ground so it reads as the underlying layer. Clean sheets,
   bonus and the price move are not counts of that kind (position-dependent, a bonus ledger,
   money) and sit in one meta footer line under the grid (NOTES.md §2). */
function lastSeasonHtml(ls){
  if(!ls) return `<p class="pcnil"><span class="g" aria-hidden="true">–</span>
    <span class="t-meta">He didn’t play in the Premier League last season.</span></p>`;
  /* ⚠️ ABSENT is not zero, and the difference is the whole reason the server sends nothing
     here for a midfielder or a forward. SeasonSummary.CleanSheets is a *int precisely so
     that "not his stat" and "none all season" cannot collapse into one value -- and this
     line collapsed them anyway, defaulting an absent count to 0 and printing "0 clean
     sheets" against players the figure does not describe. It shipped, and it read as a
     claim about a player rather than as an absence of one.

     Omit the clause entirely when the server omitted the number. */
  const cs = ls.clean_sheets===undefined||ls.clean_sheets===null ? null : ls.clean_sheets;
  return `
    <div class="msgrid">
      <div><span class="t-label">points</span><span class="t-stat">${ls.points}</span></div>
      <div><span class="t-label">minutes</span><span class="t-stat">${ls.minutes}</span></div>
      <div><span class="t-label">starts</span><span class="t-stat">${ls.starts}</span></div>
      <div><span class="t-label">pts/90</span><span class="t-stat">${ls.points_per_90.toFixed(2)}</span></div>
      <div class="und"><span class="t-label">goals</span><span class="t-stat">${ls.goals}</span></div>
      <div class="und"><span class="t-label">assists</span><span class="t-stat">${ls.assists}</span></div>
      <div class="und"><span class="t-label">xG</span><span class="t-stat">${ls.xg.toFixed(1)}</span></div>
      <div class="und"><span class="t-label">xA</span><span class="t-stat">${ls.xa.toFixed(1)}</span></div>
    </div>
    <p class="t-meta msfoot"><span>${esc(ls.season)}</span><span>·</span>
      ${cs===null?'':`<span>${cs} clean sheet${cs===1?'':'s'}</span><span>·</span>`}<span>${ls.bonus} bonus</span><span>·</span>
      <span>£${ls.price_start.toFixed(1)}m → £${ls.price_end.toFixed(1)}m</span></p>`;
}

/* gameweeksHtml draws band 4. gws is viewmodel.PlayerGameweek[] or undefined/empty -- empty
   is the ordinary state for every player in the game at GW1, and the copy says so rather
   than rendering a blank table. The server sends oldest first; the client reverses it for
   display only -- reordering an already-complete list, not computing anything -- because
   "most recent first" is right here even though it inverts the market table's own default.

   Ten columns will not fit at 390px, so six (G/A/CS/BPS/xG/xA, the .pdmore cells) hide under
   the same 720px breakpoint the rest of this design uses and reappear, per row, on tap --
   see .pdgwdetail in armband.css. Not horizontal scroll: the bench already owns that
   gesture on this page. */
/* Wrapped in .gwscroll so a long log (up to 38 rows by the end of a season) does not push
   the sheet's actions out of reach -- the table scrolls inside a fixed-height box with a
   sticky header, rather than the sheet growing without limit. The .pdmore column-hiding
   mechanism and wireGwTaps() below it are otherwise unchanged. */
function gameweeksHtml(gws){
  if(!gws || !gws.length) return `<p class="pcnil"><span class="g" aria-hidden="true">–</span>
    <span class="t-meta">No games played yet this season.</span></p>`;
  const rows=gws.slice().reverse();
  return `<div class="gwscroll"><table class="gwtable"><thead><tr>
      <th>GW</th><th>Opp</th><th class="n">Min</th><th class="n">Pts</th>
      <th class="n pdmore">G</th><th class="n pdmore">A</th><th class="n pdmore">CS</th>
      <th class="n pdmore">BPS</th><th class="n pdmore">xG</th><th class="n pdmore">xA</th>
    </tr></thead><tbody>${rows.map(r=>`
      <tr class="pdgwrow" tabindex="0">
        <td class="n">${r.gw}</td>
        <td>${r.home?'':'@'}${esc(r.opponent)}</td>
        <td class="n">${r.minutes}</td>
        <td class="n">${r.points}</td>
        <td class="n pdmore">${r.goals}</td>
        <td class="n pdmore">${r.assists}</td>
        <td class="n pdmore">${r.clean_sheet}</td>
        <td class="n pdmore">${r.bps}</td>
        <td class="n pdmore">${r.xg.toFixed(2)}</td>
        <td class="n pdmore">${r.xa.toFixed(2)}</td>
      </tr>
      <tr class="pdgwdetail"><td colspan="4">
        G ${r.goals} · A ${r.assists} · CS ${r.clean_sheet} · BPS ${r.bps} · xG ${r.xg.toFixed(2)} · xA ${r.xa.toFixed(2)}
      </td></tr>`).join('')}</tbody></table></div>`;
}

/* Tapping a row on a narrow screen reveals its .pdgwdetail sibling -- the mobile answer to
   the six columns .pdmore hides. Harmless at desktop: those columns are already visible
   there, and the media query in armband.css keeps .pdgwdetail.open closed above 720px
   regardless of this handler firing. */
function wireGwTaps(root){
  root.querySelectorAll('.pdgwrow').forEach(row=>{
    const toggle=()=>{
      const detail=row.nextElementSibling;
      if(detail && detail.classList.contains('pdgwdetail')) detail.classList.toggle('open');
    };
    row.onclick=toggle;
    row.onkeydown=e=>{ if(e.key==='Enter'||e.key===' '){ e.preventDefault(); toggle(); } };
  });
}

/* loadPlayerDetail fetches the depth behind the open card and paints it into the two
   skeleton bands openSheet already drew. Guarded on sheet.dataset.pdcode at both the
   success and failure exits, because the reader can close the sheet -- or open a DIFFERENT
   player -- before this resolves, and a late response must not paint over whoever is
   showing by then. */
async function loadPlayerDetail(code){
  const sheet=document.getElementById('sheet');
  const stillOpen=()=>sheet.dataset.pdcode===String(code);
  try{
    const res=await fetch('/api/player/'+encodeURIComponent(code));
    if(!res.ok) throw new Error('http '+res.status);
    const d=await res.json();
    if(!stillOpen()) return;
    const ls=document.getElementById('pd-lastseason'), gw=document.getElementById('pd-gameweeks');
    if(ls) ls.outerHTML=`<div id="pd-lastseason">${lastSeasonHtml(d.last_season)}</div>`;
    if(gw) gw.outerHTML=`<div id="pd-gameweeks">${gameweeksHtml(d.gameweeks)}</div>`;
    const wired=document.getElementById('pd-gameweeks');
    if(wired) wireGwTaps(wired);
  }catch(e){
    if(!stillOpen()) return;
    const msg=`<p class="pcnil"><span class="g" aria-hidden="true">–</span>
      <span class="t-meta">We couldn't load his history. Close the card and open it again.</span></p>`;
    const ls=document.getElementById('pd-lastseason'), gw=document.getElementById('pd-gameweeks');
    if(ls) ls.outerHTML=`<div id="pd-lastseason">${msg}</div>`;
    if(gw) gw.outerHTML=`<div id="pd-gameweeks"></div>`;
  }
}

/* The five-word difficulty scale (P-08): FDR as a rank a reader has to look up is the
   worst jargon on the card. Used in the derivation popover's "Who's he playing" row and
   nowhere else -- the header ribbon and the by-gameweek table carry difficulty as colour,
   not as a word, exactly as .fdr already does on the pitch. */
const FDR_WORD={1:'very easy',2:'easy',3:'even',4:'hard',5:'very hard'};

/* Every piece of news on this player, in the News tab's own row shape, minus the third
   column: nothing here is the reader's to adjudicate (NOTES.md §2). Two sources, both
   system-provided -- FPL's own flag, first, and whatever team news was read and passed to
   the model for this specific player. Absent on both counts renders nothing at all. */
function playerNewsRows(p){
  const rows=[];
  const av=p.availability===undefined?1:p.availability;
  if(p.news || av<1){
    rows.push({
      chip: av===0?'OUT':(av<1?`${Math.round(av*100)}% FIT`:'FPL'),
      cls: av===0?'out':(av<1?'fpl':''),
      when:'FPL',
      text: p.news?p.news:'FPL hasn’t said why.',
      pill: av===0?'<span class="pill bad">He scores nothing this week</span>'
          : av<1?`<span class="pill warn">We're counting ${Math.round(av*100)}% of his points</span>`:''
    });
  }
  if(p.ov){
    rows.push({
      chip:'Reported', cls:'read', when:'Read '+esc(p.ov.set||''),
      text:p.ov.why, pill:'',
      /* NewsItem.effect -- render only once the server sends it (NOTES.md §6). */
      effect:p.ov.eff
    });
  }
  return rows;
}
function effectClass(dir){ return dir==='acc'||dir==='warnc'||dir==='badc' ? dir : ''; }
function pcNewsRowHtml(o){
  return `<div class="nrow">
    <div class="nsrc">
      <span class="chip${o.cls?' '+o.cls:''}">${esc(o.chip)}</span>
      <span class="when t-meta">${esc(o.when)}</span>
    </div>
    <div class="nbody">
      ${o.pill?`<div class="nwho">${o.pill}</div>`:''}
      <p class="t-body">${esc(o.text)}</p>
      ${o.effect?`<span class="neffect">
        <span class="t-label">${esc(o.effect.label)}</span>
        <span class="t-meta">${esc(o.effect.was)}</span>
        <span class="arw" aria-hidden="true">→</span>
        <span class="t-stat ${effectClass(o.effect.direction)}">${esc(o.effect.now)}</span>
      </span>`:''}
    </div>
  </div>`;
}

/* The derivation popover -- desktop hover only, gone outright under 720px (wireWhy /
   the media query in armband.css). These rows are the model's INPUTS, not an expression
   that produces the total under them, and the footer says so rather than implying an
   arithmetic this file does not perform (NOTES.md §2). */
function derivPopHtml(p,f,mult,isCap){
  const figure=(xpFor(p)*(isCap?mult:1)).toFixed(2);
  const av=p.availability===undefined?1:p.availability;
  return `<div class="derivpop" role="tooltip" id="deriv-${p.id}">
    <div class="t-label dhead">What goes into the number</div>
    <div class="dstep"><span class="t-body">How much he scores</span>
      <span class="dv">${p.p90.toFixed(2)} per 90</span></div>
    <div class="dstep"><span class="t-body">Who's he playing</span>
      <span class="dv">${f?`${f.ha==='A'?'@':''}${esc(f.opp)}, ${f.ha==='A'?'away':'home'} — ${FDR_WORD[f.fdr]||f.fdr}`:'No game this week'}</span></div>
    <div class="dstep"><span class="t-body">Will he play</span><span class="dv">${p.rel.toFixed(2)}</span></div>
    <div class="dstep"><span class="t-body">Is he fit</span><span class="dv">${av.toFixed(2)}</span></div>
    ${isCap?`<div class="dstep"><span class="t-body">${mult===3?'Your armband, tripled':'Your armband'}</span><span class="dv">×${mult}</span></div>`:''}
    <div class="dstep total"><span class="t-row">His score this week</span><span class="dv">${figure}</span></div>
    <p class="t-meta derivfoot">These are the inputs. The figure is the model's own — we don't re-do its sums here.</p>
  </div>`;
}

/* ============================================================
   RENDER — player sheet (the "why" for one player)
   ============================================================
   Deliberately shaped as design-assets/v2/player-card.html's sheetHtml(): same data, same
   guards, new bands. Band order: header with the fixture ribbon opposite the name; .pcnews
   (every piece of news, absent = nothing); .pchero (the figure, its hover derivation, Will
   he start?, Per £m); Last season; This season by gameweek; Lock in / Leave out on the
   band channel. "No corrections" and the old .statgrid/.deriv panels are gone with the
   override concept they explained -- see NOTES.md §2.
   ============================================================ */
function openSheet(id){
  const p=byId(id), chip=gwState().chip;
  const f=fixtureIn(p.club,S.gw);
  const inSquad=P.some(x=>x.id===id);
  const onPitch=S.xi.includes(id);
  const isCap=S.cap===id, mult=chip==='3xc'?3:2;
  const locked=S.locks.has(id), leftOut=S.blocks.has(id);
  // Opening a DIFFERENT player's sheet disarms a pending Lock in confirm; re-opening the
  // same one (which the arm/cancel handlers below do, to redraw the confirm strip in
  // place) must not lose it.
  if(S.armLock!==null && S.armLock!==id) S.armLock=null;
  const armed=S.armLock===id;
  const news=playerNewsRows(p);
  const sheet=document.getElementById('sheet');
  sheet.innerHTML=`
<div class="sheet pc">
  <header>
    <div class="pcbar" style="background:${CLUBC[p.club]||'#39506A'}"></div>
    <div class="pcid">
      <h2 class="t-title">${esc(p.n)}</h2>
      <p class="t-meta">${esc(p.pos)} · ${esc(p.club)} · £${p.pr.toFixed(1)}m · ${p.own.toFixed(1)}% owned</p>
    </div>
    <div class="pcnext">
      <span class="t-label">Next five</span>
      ${ribbon(p.club,5,S.gw)}
    </div>
    <button class="btn icon ghost pcclose" aria-label="Close">✕</button>
  </header>

  <div class="body">
    ${news.length?`<div class="pcnews">${news.map(pcNewsRowHtml).join('')}</div>`:''}

    <div class="pchero">
      <div class="pcfig">
        <span class="t-label">pts a week</span>
        <button class="pcwhy" type="button" aria-describedby="deriv-${p.id}">
          <span class="t-figure">${(xpFor(p)*(isCap?mult:1)).toFixed(2)}</span>
          <span class="cue" aria-hidden="true">?</span>
        </button>
        ${derivPopHtml(p,f,mult,isCap)}
      </div>
      <div class="pcstats">
        <div class="pcstart">
          <span class="t-label">Will he start?</span>
          <span class="srow">
            ${roleChip(p.role)}
            <span class="t-stat">${Math.round(p.mn||0)}</span>
            <span class="t-meta">we expect ${Math.round(p.mn||0)} mins a game</span>
          </span>
          <span class="mbar" aria-hidden="true"><span style="width:${Math.round((p.rel||0)*100)}%"></span></span>
          <span class="t-meta">1.00 is nailed on</span>
        </div>
        <div>
          <span class="t-label">points per £m</span>
          <span class="t-stat">${p.value.toFixed(2)}</span>
          <span class="t-meta">points for the money</span>
        </div>
      </div>
    </div>

    <div class="pcsec"><span class="t-label">Last season</span></div>
    <div id="pd-lastseason"><p class="pcnil"><span class="t-meta">Loading…</span></p></div>

    <div class="pcsec"><span class="t-label">Week by week</span></div>
    <div id="pd-gameweeks"><p class="pcnil"><span class="t-meta">Loading…</span></p></div>

    <div class="sheetacts">
      ${inSquad?`
        <button class="btn primary" data-sact="swap">${onPitch?'Swap him out':'Swap him in'}</button>
        <button class="btn" data-sact="replace">Replace him…</button>
        <button class="btn own" type="button" aria-pressed="${locked}" data-sact="lock">${locked?'Locked in':'Lock in'}</button>
        <button class="btn own" type="button" aria-pressed="${leftOut}" data-sact="block">${leftOut?'Left out':'Leave out'}</button>`
      :`<button class="btn primary" data-sact="buy">Transfer in — £${p.pr.toFixed(1)}m</button>
        <button class="btn own" type="button" aria-pressed="${locked}" data-sact="lock">${locked?'Locked in':'Lock in'}</button>
        <button class="btn own" type="button" aria-pressed="${leftOut}" data-sact="block">${leftOut?'Left out':'Leave out'}</button>
        ${armed?`
        <div class="marketnote rule" style="margin-top:2px;flex-basis:100%">
          <b>Rebuild around ${esc(p.n)}?</b> We'll replace your fifteen with the best squad the model
          can build that contains him. Your current line-up and bench order are lost.
          <span class="spacer"></span>
          <button class="btn sm ghost warn" data-sact="armgo">Yes, rebuild</button>
          <button class="btn sm ghost" data-sact="armcancel">Cancel</button>
        </div>`:''}`}
    </div>
  </div>
</div>`;
  document.getElementById('scrim').classList.add('open');
  /* Marks which player the two skeleton bands above belong to, checked when the fetch below
     resolves -- the reader may close the sheet, or open a DIFFERENT player, before then, and
     a stale response must not paint over whoever is showing now. */
  sheet.dataset.pdcode=String(p.code);
  loadPlayerDetail(p.code);
  sheet.querySelector('.pcclose').onclick=closeSheet;
  wireWhy(sheet);
  /* No "Make captain" / "Make vice" here any more -- the armband picker
     (openArmbandPicker) is the one route, priced and ranked, at every width. */
  sheet.querySelectorAll('[data-sact]').forEach(b=>b.onclick=()=>{
    const a=b.dataset.sact;
    /* Locking and leaving out are STANDING corrections -- they bind every build, not just
       this page -- so they go to the server and the answer is the squad the model picks
       under them. Applying them locally would show a fifteen the model has not agreed to. */
    if(a==='lock'){
      if(inSquad){
        // Already yours: locking him in pins the arrangement, it does not rebuild it.
        closeSheet(); toggleCorrection(id,'lock'); return;
      }
      // Not yours: the model would rebuild your whole fifteen around him, so this needs
      // the same arm-then-confirm step the market row's own Lock in uses (see armgo below
      // and .ptable tr.arming/.armnote). Re-opening the same sheet redraws it armed.
      if(armed) return;
      S.armLock=id; openSheet(id); return;
    }
    if(a==='armgo'){ S.armLock=null; closeSheet(); toggleCorrection(id,'lock'); return; }
    if(a==='armcancel'){ S.armLock=null; openSheet(id); return; }
    if(a==='block'){
      S.armLock=null; closeSheet(); toggleCorrection(id,'block'); return;
    }
    if(a==='swap'){S.swapFrom=id;closeSheet();setSwapbar();return;}
    if(a==='replace'){openPicker(id);return;}
    if(a==='buy'){
      // He is not in the fifteen, so there is no single player this sheet already knows
      // he would replace -- openBuyPicker defaults the outgoing slot to the weakest
      // starter in his position, the same suggestion the market table makes for every
      // other candidate, and the tray lets the reader pick a different one of their own
      // fifteen before anything is bought.
      closeSheet();
      openBuyPicker(id);
      return;
    }
    closeSheet();renderAll();
  });
}
/* Click as well as hover, so a desktop touch screen can reach the derivation. Under 720px
   .pcwhy is pointer-events:none (armband.css), so this never fires there. */
function wireWhy(root){
  root.querySelectorAll('.pcwhy').forEach(b=>{
    b.onclick=()=>b.closest('.pcfig').classList.toggle('open');
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
             <span class="dim" style="font-family:var(--mono);font-size:10px">${f?`vs ${esc(f.opp)} (${esc(f.ha)})`:'no fixture'}</span>
             <i style="font-style:normal">${fdrHtml(p.club,1,S.gw)}</i></span></span>
         <span class="cx"><b>${x.toFixed(2)}</b><span class="dim">pts</span>
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

function closeSheet(){
  document.getElementById('scrim').classList.remove('open');
  /* Clears the guard loadPlayerDetail checks, so a fetch already in flight for the player
     who was just closed paints nothing if it resolves afterwards -- belt to the id lookups
     already failing gracefully once the sheet's markup is replaced by whatever opens next. */
  document.getElementById('sheet').dataset.pdcode='';
  S.armLock=null;
}
document.getElementById('scrim').onclick=e=>{if(e.target.id==='scrim')closeSheet();};

/* ============================================================
   RENDER — players market
   ============================================================ */
function renderPlayers(){
  const bank=bankOf(), gate=gateOf(), bh=bankHtml();
  const bankEl=document.getElementById('bank');
  bankEl.textContent=bh.text;
  bankEl.className='v'+(bh.bad?' badc':'');
  /* The rest of this panel's header. Every one of these was a literal in the markup,
     which is how the squad came to be worth £99.5m on the pitch and £100.0m here. */
  const st=STATE||{squad:{},market:{}};
  const cost=st.squad.cost||0;
  const bankSubEl=document.getElementById('bankSub');
  bankSubEl.textContent=bh.bad?'over budget — sell someone to fund it':`of £${(cost+bank).toFixed(1)}m budget`;
  bankSubEl.className='sub'+(bh.bad?' badc':'');
  document.getElementById('squadValue').textContent='£'+cost.toFixed(1)+'m';
  document.getElementById('squadValueSub').textContent=`${(st.squad.players||[]).length} players`;
  document.getElementById('gateValue').innerHTML=
    `+${gate.toFixed(2)}<small>pts a week</small>`;
  const upTo=document.getElementById('bankUpTo');
  if(upTo) upTo.textContent=(STATE&&STATE.policy&&STATE.policy.bank_up_to)||'—';
  document.getElementById('benchLegend').textContent=
    BENCHMARKS.map(b=>`${b.pos} vs ${b.name} ${b.score.toFixed(2)}`).join(' · ');
  let list=POOL.filter(p=>S.posFilter==='ALL'||p.pos===S.posFilter)
    .filter(p=>!S.q||((p.n+' '+p.club).toLowerCase().includes(S.q.toLowerCase())));
  /* affordable = you can sell your weakest in that position and still cover him. This is
     no longer "who it would replace" -- the tray (openBuyPicker) lets the reader choose
     that -- so only the weakest player's PRICE survives here, for the row's quiet "needs
     +£X" marking; the weakest player himself is not carried on the row. */
  const weakest=pos=>P.filter(p=>p.pos===pos).sort((a,b)=>a.xp-b.xp)[0];
  list=list.map(p=>{
    const w=weakest(p.pos);
    /* d and clears come from the server: MarketRow.Delta and MarketRow.ClearsGate.
       Colouring the gap against a hardcoded bar was the page recommending in colour what
       the policy refuses in prose -- the same defect this once had against zero. */
    return {...p,d:p.delta,clears:p.clears,afford:bank+w.pr-p.pr};
  });
  const reachable=list.filter(p=>p.afford>=0).length, clears=list.filter(p=>p.clears).length;
  if(S.affordOnly) list=list.filter(p=>p.afford>=0);
  list=applySort(list);
  document.querySelectorAll('#ptable [data-sort]').forEach(b=>{
    const th=b.closest('th'), active=b.dataset.sort===S.sort.col;
    th.setAttribute('aria-sort',active?(S.sort.dir==='asc'?'ascending':'descending'):'none');
    b.classList.toggle('active',active);
  });
  const pillVal=`${S.sort.col}:${S.sort.dir}`;
  if(sortPill && sortPill.value!==pillVal){
    // The pill only carries the eleven canonical combinations; an off-menu sort (reached
    // from a desktop header, e.g. Player ascending vs the pill's descending list) leaves
    // it on its nearest option rather than silently failing to match anything.
    if([...sortPill.options].some(o=>o.value===pillVal)) sortPill.value=pillVal;
  }
  document.getElementById('marketnote').innerHTML=`
    <span class="gate pass"></span><b>${clears}</b> of ${POOL.length} clear the +${gate.toFixed(2)} gate
    <span class="sep">·</span>
    <b>${reachable}</b> are reachable with £${bank.toFixed(1)}m in the bank
    <span class="sep">·</span><span class="dim">anything short of the money is marked, not withheld</span>`;
  document.getElementById('poolCount').textContent=POOL.length;

  const MOB_CAP=40, shown=list.slice(0,S.showAll?list.length:MOB_CAP);
  const emptyHtml=`<div class="empty">
      <div class="big">Nothing matches</div>
      <p>No player matches ${S.q?`“${esc(S.q)}”`:'these settings'}${S.affordOnly?' inside your budget':''}.</p>
      <button class="btn sm" id="clearFilters">Show all ${POOL.length} players</button>
    </div>`;
  document.getElementById('emptyState').innerHTML=list.length?'':emptyHtml;
  document.getElementById('ptable').style.display=list.length?'':'none';
  document.getElementById('moreline').innerHTML =
    list.length>shown.length
      ? `Showing ${shown.length} of ${list.length} · <button class="btn sm" id="showMore">Load the rest</button>`
      : list.length ? `<span>All ${list.length} shown</span>` : '';

  document.getElementById('ptbody').innerHTML=shown.map(p=>{
    const armed=S.armLock===p.id;
    return `
   <tr data-id="${p.id}"${armed?' class="arming"':''}>
     <td class="c-gate"><span class="gate${p.clears?' pass':''}" title="${p.clears?`clears the +${gate.toFixed(2)} gate`:'below the gate'}"></span></td>
     <td class="c-name"><span class="who">${esc(p.n)}</span><span class="club">${esc(p.club)}</span></td>
     <td class="k c-pos">${esc(p.pos)}</td>
     <td class="c-fdr">${fdrHtml(p.club,5,S.gw)}</td>
     <td class="c-role">${roleChip(p.role,'dot')}</td>
     <td class="n c-own">${p.own.toFixed(1)}%</td>
     <td class="n c-xg">${statHtml('xG',p.xg90)}</td>
     <td class="n c-xa">${statHtml('xA',p.xa90)}</td>
     <td class="n c-dc">${pctHtml('DC',p.dc)}</td>
     <td class="n c-price">£${p.pr.toFixed(1)}${p.afford<0?`<span class="short">needs +£${Math.abs(p.afford).toFixed(1)}m</span>`:''}</td>
     <td class="n c-xp">${p.xp.toFixed(2)}</td>
     <td class="n c-delta ${p.clears?'dpos':'dneg'}">${p.d>0?'+':''}${p.d.toFixed(2)}</td>
     <td class="c-acts">${rowActsHtml(p)}</td>
   </tr>${armed?armNoteRowHtml(p):''}`;}).join('');

  document.getElementById('plist').innerHTML=shown.map(p=>`
   <div class="prow" data-id="${p.id}">
     <div>
       <div class="l1"><span class="gate${p.clears?' pass':''}"></span>
         <span class="nm">${esc(p.n)}</span><span class="k">${esc(p.pos)}</span>
         <span class="club" style="font-family:var(--mono);font-size:10px;color:var(--ink3)">${esc(p.club)}</span>
         ${roleChip(p.role,'dot')}</div>
       <div class="l2">£${p.pr.toFixed(1)}m
         ${statHtml('xG',p.xg90)} ${statHtml('xA',p.xa90)} ${pctHtml('DC',p.dc)}
         ${fdrHtml(p.club,3,S.gw)}
         ${p.afford<0?`<span class="short">needs +£${Math.abs(p.afford).toFixed(1)}m</span>`:''}</div>
     </div>
     <div class="r">
       <div class="xp">${p.xp.toFixed(2)}</div>
       <div class="dd ${p.clears?'dpos':'dneg'}">${p.d>0?'+':''}${p.d.toFixed(2)}</div>
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

  wireMarketRows();
}

/* Own (a colour dot with no label -- .role.bare) plus Leave out (immediate, harmless: an
   unowned player leaving the market pool touches nothing stored) and the neutral Transfer
   action that opens the buy-mode tray. Desktop only -- mobile puts these into the row's
   detail sheet instead, the same "controls move into the sheet below 720px" rule the pitch
   card already follows (armband.css, .card .acts under @media(max-width:720px)). */
function rowActsHtml(p){
  const locked=S.locks.has(p.id), leftOut=S.blocks.has(p.id);
  return `<div class="rowacts">
    <button class="btn sm own" type="button" aria-pressed="${locked}" data-act="lock" data-id="${p.id}">${locked?'Locked in':'Lock in'}</button>
    <button class="btn sm own" type="button" aria-pressed="${leftOut}" data-act="block" data-id="${p.id}">${leftOut?'Left out':'Leave out'}</button>
    <button class="btn sm xfer" type="button" data-act="buy" data-id="${p.id}" title="Choose who goes for ${esc(p.n)}">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" aria-hidden="true"><path d="M7 16V4m0 0L3 8m4-4l4 4M17 8v12m0 0l4-4m-4 4l-4-4"/></svg>Transfer
    </button>
  </div>`;
}

/* The arm-then-confirm strip a Lock in click opens under the row it belongs to. Locking an
   unowned player rebuilds the whole fifteen around him -- too large a consequence for one
   click on a small button -- so the row arms first (S.armLock) and this second row is what
   the second click actually confirms. Any other click disarms it (wireMarketRows). */
function armNoteRowHtml(p){
  return `<tr class="armnote"><td colspan="13">
    <div class="marketnote rule">
      <b>Rebuild around ${esc(p.n)}?</b> We'll replace your fifteen with the best squad the model
      can build that contains him. Your current line-up and bench order are lost.
      <span class="spacer"></span>
      <button class="btn sm ghost warn" data-armgo="${p.id}">Yes, rebuild</button>
      <button class="btn sm ghost" data-armcancel="1">Cancel</button>
    </div>
  </td></tr>`;
}

function wireMarketRows(){
  document.querySelectorAll('#ptbody [data-act]').forEach(b=>b.onclick=e=>{
    e.stopPropagation();
    const id=+b.dataset.id, act=b.dataset.act;
    if(act==='lock'){
      if(S.armLock===id) return; // already armed -- the strip below is what confirms it
      S.armLock=id; renderPlayers(); return;
    }
    if(act==='block'){ S.armLock=null; toggleCorrection(id,'block'); return; }
    if(act==='buy'){ S.armLock=null; openBuyPicker(id); return; }
  });
  document.querySelectorAll('[data-armgo]').forEach(b=>b.onclick=e=>{
    e.stopPropagation();
    const id=+b.dataset.armgo; S.armLock=null; toggleCorrection(id,'lock');
  });
  document.querySelectorAll('[data-armcancel]').forEach(b=>b.onclick=e=>{
    e.stopPropagation(); S.armLock=null; renderPlayers();
  });
  document.querySelectorAll('#ptbody tr:not(.armnote),.prow').forEach(r=>r.onclick=e=>{
    if(e.target.closest('[data-act]')) return;
    if(S.armLock!==null){ S.armLock=null; renderPlayers(); return; }
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
   RENDER — News
   ============================================================
   Three sources, all provided by the system: FPL's own status; team news we've read; and
   the standing "who may not start" band, drawn from role bands the model already produces.
   The old kind filter (All / Minutes / Scoring / Selection, newsKindMatch, SELECTION_KINDS)
   is deleted with the "your settings" framing it served -- on a news page the reader's
   question is who says so, not which config key, and the groups now do the work a filter
   did (NOTES.md §3).

   No row here has a button. Ignore / Use it again went with the override concept: framed
   as news rather than a setting, "don't count this" is a strange offer, and it is the last
   thing on the page implying the reader owns any of it. ignoreOverride() and useAgain() are
   deleted outright -- Your instructions stopped listing config entries in the same change,
   which took their last caller with it. The reader's own locks and leave-outs stay there and
   keep their control. */

/* riskRows filters the owned fifteen to a rotation risk or worse and sorts by role, then
   by modelled minutes within a role -- FILTERING AND SORTING a list the server already
   produced. No new model quantity: it is Player.role, reordered. */
function riskRows(){
  return P.filter(p=>roleNum(p.role)>=3)
    .sort((a,b)=>(roleNum(a.role)-roleNum(b.role))||((b.mn||0)-(a.mn||0)));
}

/* One row shape for every source: [ who says so, and when ][ what happened ]. The third
   column (.nact) is documented in v2.css but has no caller in v1 -- nothing on this tab is
   the reader's to adjudicate -- so it is never emitted here. */
function nrow(o){
  return `
  <div class="nrow${o.stale?' stale':''}">
    <div class="nsrc">
      <span class="chip ${o.chipClass||''}">${esc(o.chip)}</span>
      <span class="when t-meta">${esc(o.when||'')}</span>
    </div>
    <div class="nbody">
      <div class="nwho">
        <span class="t-row">${esc(o.who)}</span>
        <span class="t-meta">${esc(o.club)}</span>
        ${o.roleBand?roleChip(o.roleBand):''}
        ${o.pill||''}
      </div>
      ${o.text?`<p class="t-body ntext${o.clamp?' clamp':''}">${esc(o.text)}</p>`:''}
      ${o.meta?`<p class="t-meta">${esc(o.meta)}</p>`:''}
      ${o.effect?`<span class="neffect">
        <span class="t-label">${esc(o.effect.label)}</span>
        <span class="t-meta">${esc(o.effect.was)}</span>
        <span class="arw" aria-hidden="true">→</span>
        <span class="t-stat ${effectClass(o.effect.direction)}">${esc(o.effect.now)}</span>
      </span>`:''}
    </div>
  </div>`;
}
function newsGroupHtml(title,countHtml,rows,nilHtml,freshHtml){
  return `<div class="ngroup">
    <div class="ngrouphead">
      <span class="t-label">${esc(title)}</span>
      <span class="sp"></span>
      ${countHtml}
      ${freshHtml||''}
    </div>
    ${rows.length?rows.map(nrow).join(''):nilHtml}
  </div>`;
}
const nilRow=txt=>`<p class="nempty"><span class="g" aria-hidden="true">
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M5 12h14"/></svg></span>
  <span class="txt"><span class="t-row muted">${esc(txt)}</span></span></p>`;
const allGoodHtml=`<p class="nallgood"><span class="g" aria-hidden="true">
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4"><path d="M4 12.5l5 5L20 6.5"/></svg></span>
  <span class="txt">
    <span class="t-row">All fifteen are nailed starters this week.</span>
    <span class="t-meta">Rare. Enjoy it.</span></span></p>`;
/* The press feed's shape, one specimen row so it can be judged -- not the live thing. One
   badge, no prose about what is or is not wired up (NOTES.md §3). */
const pressPanelHtml=`<div class="notyet nypanel">
  <div class="nyhead">
    <span class="pill soon">Coming soon</span>
    <span class="t-row">Team news, press conferences and leaked lineups, read as they land</span>
  </div>
  <div class="specimen" aria-hidden="true">
    ${nrow({chip:'Press',chipClass:'press',when:'',who:'A player in your fifteen',club:'',
      text:'Who said it, when they said it, and what it did to his number.'})}
  </div>
</div>`;

function renderNews(){
  // Config-sourced only ("team news we've read"). A session lock or leave-out is the
  // reader's own and is drawn on the pitch page's Your instructions panel instead.
  const read=OV.filter(o=>!o.session).sort((a,b)=>(b.needsCheck-a.needsCheck)||(b.age-a.age));
  const stale=read.filter(o=>o.needsCheck);
  const flagged=P.filter(p=>(p.availability!==undefined&&p.availability<1)||p.news);
  const risk=riskRows();

  const checkedEl=document.getElementById('newsChecked');
  if(checkedEl) checkedEl.textContent=NEWS.checked;

  const flaggedEl=document.getElementById('news-flagged');
  if(flaggedEl) flaggedEl.innerHTML = flagged.length ? newsGroupHtml('Hurt, suspended or doubtful',
    `<span class="pill ${flagged.some(p=>p.availability===0)?'bad':'warn'}">${flagged.length}</span>
     <span class="t-meta">FPL's own ruling — nothing to decide here</span>`,
    flagged.map(p=>{
      const av=p.availability===undefined?1:p.availability;
      return {
        chip: av===0?'OUT':`${Math.round(av*100)}% FIT`, chipClass: av===0?'out':'fpl',
        who:p.n, club:p.club,
        text:p.news?p.news:'FPL hasn’t said why.',
        pill: av===0?'<span class="pill bad">He scores nothing this week</span>'
            : `<span class="pill warn">We're counting ${Math.round(av*100)}% of his points</span>`
      };
    }), '', '') : '';

  const readEl=document.getElementById('news-read');
  if(readEl) readEl.innerHTML = newsGroupHtml('Team news we’ve read',
    read.length
      ? `<span class="pill">${read.length}</span>
         ${stale.length?`<span class="pill warn">${stale.length} need a look</span>`:''}
         <span class="t-meta">Reported elsewhere, read by us, and fed to the model</span>`
      : `<span class="t-meta">Reported elsewhere, read by us, and fed to the model</span>`,
    read.map(o=>({
      chip:'Reported', chipClass:'read', stale:o.needsCheck, who:o.who, club:o.club,
      text:o.why, clamp:true,
      meta:`Read ${o.set}${o.lapse?` · ${o.lapse}`:''} · last checked ${o.chk||'never'}`,
      pill:o.needsCheck?'<span class="pill warn">Due a re-check</span>':'',
      effect:o.eff
    })),
    nilRow('Nothing reported on your fifteen this week.'),
    `<span class="nfresh gfresh">
     <span class="t-meta">${esc(NEWS.readChecked)}</span></span>`);

  const riskEl=document.getElementById('news-risk');
  if(riskEl) riskEl.innerHTML = newsGroupHtml('Who may not start',
    risk.length
      ? `<span class="pill warn">${risk.length}</span><span class="t-meta">Fit, but not certain to be picked</span>`
      : `<span class="t-meta">Fit, but not certain to be picked</span>`,
    risk.map(p=>({
      chip:'Model', chipClass:'model', when:'this gameweek', who:p.n, club:p.club, roleBand:p.role,
      /* was is a fixed fact of football (a full match), not anything modelled -- 90 is
         safe to state directly, unlike the modelled figure beside it. Player.ModelledMinutes
         (a pre-formatted "90 → 54 modelled" string) used to carry this pair as one field;
         it was removed as one quantity in two shapes -- the row template already draws the
         arrow, so p.mn is the only number this needs. */
      effect:{label:'minutes', was:'90', now:`${Math.round(p.mn||0)} modelled`}
    })),
    allGoodHtml);

  const pressEl=document.getElementById('news-press');
  if(pressEl) pressEl.innerHTML=pressPanelHtml;

  document.querySelectorAll('.ntext.clamp').forEach(t=>t.onclick=()=>t.classList.toggle('clamp'));

  // The nav badge counts what needs attention: reported items overdue for a re-check --
  // staleness the server still decides, the same rule as before on a renamed thing -- plus
  // owned players FPL has ruled below full fitness. A count that is on for every visit is a
  // badge the eye learns to skip.
  const need=stale.length+flagged.filter(p=>(p.availability===undefined?1:p.availability)<1).length;
  const newsCount=document.getElementById('newsCount');
  if(newsCount){ newsCount.hidden=need===0; newsCount.textContent=need; }
}

/* ============================================================
   VIEW SWITCHING
   ============================================================ */
const VIEWS=['pitch','players','news'];

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
/* Where this fifteen came from, said plainly.
 *
 * The opening squad is deliberately varied rather than the model's single best, so a reader
 * looking at it deserves to know which of the two they have -- otherwise the tool appears to
 * be recommending something it is not. */
function renderSquadSource(){
  const el=document.getElementById('squadsource');
  if(!el) return;
  el.textContent = S.saved ? 'your saved team'
    : S.optimised ? "our best fifteen"
    : 'a strong opening fifteen — press Optimise for the model’s best';
  const opt=document.getElementById('optimise');
  if(opt) opt.disabled = !!S.optimised && !S.saved;
}

function renderAll(){renderRail();renderReadout();renderChips();renderSquadSource();renderPitch();renderInstructions();renderPlayers();renderLeftOut();renderNews();}

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
      const buy=/^buy-(\d+)$/.exec(hash);
      if(replace){ setView('pitch', false); openPicker(+replace[1]); }
      else if(buy){ setView('pitch', false); openBuyPicker(+buy[1]); }
      else if(hash){ setView(hash, false); }
    })
    .catch(err=>{
      const el=document.getElementById('view-pitch');
      if(el) el.innerHTML=`<div class="panel" style="padding:24px">
        <b>The squad could not be loaded.</b>
        <div class="dim" style="margin-top:8px">${esc(err.message)}</div>
        <div class="dim" style="margin-top:8px">Try reloading the page. If it keeps
        happening, the preview may be down for a moment — come back shortly.</div></div>`;
      console.error(err);
    });
}
/* Optimize discards the arrangement and asks the model for its best fifteen, WITHIN your
 * standing locks and blocks -- it respects them, same as every other build on this page.
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

/* Reset is Optimize's destructive sibling: the model's honest best with NONE of your
 * locks or blocks in force, and it DISCARDS them rather than merely ignoring them for one
 * build -- pending.lock and pending.excl go to empty, so Your instructions empties too.
 * That is the whole distinction the ask names ("optimize respects locks, reset does not"),
 * and it needs no server change: effectiveCfgFrom already builds every squad from
 * whatever the session's own lock/excl lists say, so an empty pair of lists is a build
 * under no session corrections at all. A confirm guards it because there is no undo once
 * the save round-trips -- but only when there is something to lose. */
const resetBtn=document.getElementById('resetBtn');
if(resetBtn) resetBtn.onclick=()=>{
  const n=(PENDING.lock||[]).length+(PENDING.excl||[]).length;
  if(n && !confirm(`Reset asks the model for its honest best and forgets what you told it: `+
    `${n} instruction${n===1?'':'s'} will be discarded. Optimise keeps them; Reset does not. Continue?`)) return;
  save(pending=>{
    pending.opt=true;
    pending.squad=undefined;
    pending.xi=undefined;
    pending.bench=undefined;
    pending.cap=undefined;
    pending.vc=undefined;
    pending.lock=[];
    pending.excl=[];
  });
};

/* Import card: wire up the import and skip buttons */
const importSubmitBtn=document.getElementById('importSubmit');
const importSkipBtn=document.getElementById('importSkip');
const importTeamIdInput=document.getElementById('importTeamId');
const importErrorDiv=document.getElementById('importError');

if(importSubmitBtn){
  importSubmitBtn.onclick=()=>importTeam();
}
if(importSkipBtn){
  importSkipBtn.onclick=()=>skipImport();
}
if(importTeamIdInput){
  importTeamIdInput.addEventListener('keypress', e=>{
    if(e.key==='Enter') importTeam();
  });
}

function importTeam(){
  const teamId=document.getElementById('importTeamId').value.trim();
  if(!teamId){
    showImportError('Please enter your Team ID.');
    return;
  }

  importSubmitBtn.disabled=true;
  const originalLabel=importSubmitBtn.textContent;
  importSubmitBtn.textContent='Reading your team…';

  fetch('/api/import',{
    method:'PUT',
    credentials:'same-origin',
    headers:{'Content-Type':'application/json','X-Armband-Token':TOKEN},
    body:JSON.stringify({entry:teamId})
  })
    .then(r=>{
      if(!r.ok){
        return r.text().then(text=>{
          throw new Error(text||`the server answered ${r.status}`);
        });
      }
      return r.json();
    })
    .then(st=>{
      hydrate(st);
      renderAll();
      document.getElementById('importCard').hidden=true;
      const eventNum=st.import?.event||'your';
      notify(`Imported your Gameweek ${eventNum} fifteen.`);
    })
    .catch(err=>{
      showImportError(err.message);
      importSubmitBtn.disabled=false;
      importSubmitBtn.textContent=originalLabel;
    });
}

function skipImport(){
  /* PUT /api/session is a full replace (saveSession decodes straight into a fresh
     session{}), so this goes through the same save()/PENDING chain as every other session
     mutation rather than posting a bespoke body -- a bespoke body would omit the squad,
     locks, excludes, dismissed overrides and chip placements the reader already has, and
     saveSession would store their absence as "gone" rather than "unchanged". sendSave's own
     hydrate(st) call hides the card once the server confirms noimp stuck; it must not be
     hidden here first, or a failed save would still look like it took. */
  save(p=>{ p.noimp=true; });
}

function showImportError(message){
  const errorDiv=document.getElementById('importError');
  if(errorDiv){
    errorDiv.textContent=message;
    errorDiv.style.display='block';
  }
}

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

/* mode is 'sell' (the original picker: outgoing man fixed, browse the market for who comes
   in) or 'buy' (the market row's own tray: incoming man fixed, browse your own fifteen for
   who goes). Everything below R.mode's own branches -- pickerBudget, affordGap, overClub,
   pickerStage, and the confirm/save handler in wirePicker -- reads R.out and R.sel by role
   (outgoing / incoming) and does not care which one the reader actually clicked, so it is
   unchanged by the mode that set them. */
let R={mode:'sell', out:null,pos:null,within:true,sel:null};

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
  if(R.mode==='buy'){
    /* The candidates are the reader's OWN fifteen in the target's position -- who could
       go, not who could come in. A player already marked Leave out is excluded: he is
       already leaving the squad on the next rebuild, so offering him as today's specific
       swap partner would be a second, conflicting instruction. buyHiddenCount() below
       counts what this filter removes, for the note. */
    return P.filter(p=>p.pos===R.pos && !S.blocks.has(p.id));
  }
  const o=byId(R.out);
  if(!o) return [];
  const own=new Set(P.map(p=>p.id));
  /* No block filter here: the server drops a blocked player from the market entirely, so
     POOL never carries one. Filtering again would read as the panel accounting for blocks
     when it is the server that does. */
  return POOL.filter(c=>c.pos===R.pos && !own.has(c.id));
}
/* How many of the reader's owned players in this position are hidden from the buy-mode
   candidate list because they are already marked Leave out -- named in the count line so
   nothing is dropped silently (NOTES.md's count-line convention). */
function buyHiddenCount(pos){
  return P.filter(p=>p.pos===pos && S.blocks.has(p.id)).length;
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
  R={mode:'sell', out:id,pos:o.pos,within:true,sel:null};
  renderPicker();
  document.getElementById('scrim').classList.add('open');
  if(location.hash!=='#replace-'+id) history.replaceState(null,'','#replace-'+id);
}

/* The transfer tray: the reverse of openPicker. Opened from a market row (or the sheet's
   own Transfer in button), it fixes the INCOMING man and lets the reader choose which of
   their fifteen goes -- the mirror of openPicker's fixed outgoing man and browsable market.
   Defaults the outgoing slot to the weakest starter in the target's position, the same
   suggestion the market table's own gate note makes for every candidate; the reader can
   still pick a different one of their five before confirming. */
function openBuyPicker(targetId){
  const t=byId(targetId);
  if(!t) return;
  // Same population pickerCandidates() offers as rows -- a player already marked Leave
  // out is not a sensible default outgoing man either.
  const w=P.filter(p=>p.pos===t.pos && !S.blocks.has(p.id)).sort((a,b)=>a.xp-b.xp)[0];
  R={mode:'buy', out:w?w.id:null, pos:t.pos, within:false, sel:targetId};
  renderPicker();
  document.getElementById('scrim').classList.add('open');
  if(location.hash!=='#buy-'+targetId) history.replaceState(null,'','#buy-'+targetId);
}

/* Dispatcher: R.mode decides which half of the tray this is. Everything past the fixed
   pair (pickerBudget, affordGap, overClub, pickerStage, wirePicker's confirm handler)
   reads R.out/R.sel by ROLE and does not know or care which render function set them. */
function renderPicker(){
  if(R.mode==='buy') renderBuyPicker(); else renderSellPicker();
}

function renderSellPicker(){
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
       <span class="dim">${esc(o.pos)} · ${esc(o.club)} · ${o.xp.toFixed(2)} pts a week · sells for £${sellPriceOf(o).toFixed(1)}m</span>
     </div>
     <button class="btn icon ghost" id="pkclose" aria-label="Close">✕</button>
   </header>
   <div class="body">
     <div class="repmath">
       sells <b>£${sellPriceOf(o).toFixed(1)}m</b> <span class="op">+</span> bank <b>£${bankOf().toFixed(1)}m</b>
       <span class="op">=</span> <b>£${B.toFixed(1)}m</b> to spend
       <span class="sep">·</span> gate +${gateOf().toFixed(2)} pts a week
       <span class="sep">·</span> Gain vs ${esc(o.n)}, per gameweek
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

/* The plural position word for the buy-mode count line -- "2 of your 5 midfielders", not
   "2 of your 5 mid". */
const POS_PLURAL={GKP:'goalkeepers',DEF:'defenders',MID:'midfielders',FWD:'forwards'};

function renderBuyPicker(){
  const t=byId(R.sel);
  if(!t) return;
  const list=pickerCandidates().slice().sort((a,b)=>b.xp-a.xp);
  const hidden=buyHiddenCount(R.pos);
  const beats=list.filter(p=>clearsGate(t.xp-p.xp)).length;
  const overBudget=list.filter(p=>sellPriceOf(p)+bankOf()<t.pr).length;

  const note=`<div class="marketnote"><span class="gate pass"></span>
    <b>${beats}</b> of your ${list.length} ${POS_PLURAL[R.pos]||R.pos} he beats by the +${gateOf().toFixed(2)} gate
    <span class="sep">·</span> <b>${overBudget}</b> leave you over budget
    ${hidden?`<span class="sep">·</span> <span class="ovc">${hidden} hidden by your Leave out</span>`:''}</div>`;

  const rows=list.map(p=>pickerRowBuy(p,t)).join('');
  const out=R.out?byId(R.out):null;

  document.getElementById('sheet').innerHTML=`
   <header>
     <div class="who"><b>Bring in ${esc(t.n)}</b>
       <span class="dim">${esc(t.pos)} · ${esc(t.club)} · £${t.pr.toFixed(1)}m · ${t.xp.toFixed(2)} pts a week</span>
     </div>
     <button class="btn icon ghost" id="pkclose" aria-label="Close">✕</button>
   </header>
   <div class="body">
     <div class="repmath">
       ${out?`bank <b>£${bankOf().toFixed(1)}m</b> <span class="op">+</span> ${esc(out.n)} sells <b>£${sellPriceOf(out).toFixed(1)}m</b>
       <span class="op">=</span> <b>£${pickerBudget().toFixed(1)}m</b> to spend
       <span class="sep">·</span> `:''}${esc(t.n)} costs <b>£${t.pr.toFixed(1)}m</b>
       <span class="sep">·</span> gate +${gateOf().toFixed(2)} pts a week
       <span class="sep">·</span> Gain vs the man you pick, per gameweek
     </div>
     ${note}
     ${rows}
     <div class="moreline" style="border-top:0;padding:8px">All ${list.length} shown</div>
     ${out?pickerStage(t,out):''}
   </div>`;
  wirePicker();
}

/* One row per owned candidate to sell, mirroring pickerRow but keyed the other way:
   affordability and gain are both against the FIXED target `t`, not against R.out (which
   is what THIS row would set if clicked, not what it is priced against). */
function pickerRowBuy(p,t){
  const d=+(t.xp-p.xp).toFixed(2), clears=clearsGate(d);
  const gap=+(sellPriceOf(p)+bankOf()-t.pr).toFixed(1);
  const av=(p.availability===undefined?1:p.availability);
  /* "in your XI" vs "on your bench" is a fact about the squad, not a channel colour --
     both wear .pill.xi's plain ink. Bench players are listed and offerable here too, just
     labelled as such. */
  const xiTag=S.xi.includes(p.id)?'in your XI':'on your bench';
  return `<button class="reprow${R.out===p.id?' on':''}" data-id="${p.id}">
    <span class="g"><span class="gate${clears?' pass':''}"
      title="${clears?'clears':'below'} the +${gateOf().toFixed(2)} gate"></span></span>
    <span class="n"><b>${esc(p.n)}</b><span class="club">${esc(p.club)}</span>
      <span class="pill xi">${xiTag}</span>
      ${av===0?`<span class="pill bad">ruled out</span>`:av<1?`<span class="pill warn">${Math.round(av*100)}% fit</span>`:''}</span>
    <span class="m">sells £${sellPriceOf(p).toFixed(1)}m ${roleChip(p.role,true)}
      ${gap<0?`<span class="short">needs +£${Math.abs(gap).toFixed(1)}m</span>`:''}</span>
    <span class="x"><b class="xp">${p.xp.toFixed(2)}</b>
      <span class="dd ${clears?'dpos':'dneg'}">${d>0?'+':''}${d.toFixed(2)}</span></span>
    ${p.news?`<span class="news">${esc(p.news)}</span>`:''}
  </button>`;
}

function pickerRow(c,o,browse){
  const d=+(c.xp-o.xp).toFixed(2), clears=clearsGate(d), gap=affordGap(c), oc=overClub(c);
  const av=(c.availability===undefined?1:c.availability);
  return `<button class="reprow${R.sel===c.id?' on':''}${browse?' browse':''}" data-id="${c.id}">
    <span class="g">${browse?'':`<span class="gate${clears?' pass':''}"
      title="${clears?'clears':'below'} the +${gateOf().toFixed(2)} gate"></span>`}</span>
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
    <button class="btn sm" id="pkwiden">Show the ones you can’t afford</button>
  </div>`;
}

/* Shared by both modes: c is always the man coming IN, o the man going OUT, regardless of
   which one the reader actually clicked to get here -- see the R.mode comment above.

   Confirm disables ONLY on the club limit. Budget is deliberately not a gate here, same as
   the server: `cmd/armband/webroutes.go`'s validateSession leaves budget unchecked on
   purpose, because an over-budget fifteen is a state the optimiser can legitimately be
   asked about. Marking, not blocking -- the short/needs-money line stays, in the same quiet
   ink price cells already use, it just no longer disables the button beneath it. */
function pickerStage(c,o){
  if(!c) return '';
  const d=+(c.xp-o.xp).toFixed(2), clears=clearsGate(d), gap=affordGap(c);
  return `<div class="stagebar">
    <div class="move"><span class="out">${esc(o.n)} £${sellPriceOf(o).toFixed(1)}m</span> <span class="op">→</span>
      <span class="in"><b>${esc(c.n)}</b> £${c.pr.toFixed(1)}m</span>
      <span class="sep">·</span> ${gap>=0?`leaves <b>£${gap.toFixed(1)}m</b> in the bank`
        :`<span class="short" style="display:inline;margin:0">needs +£${Math.abs(gap).toFixed(1)}m — you will be £${Math.abs(gap).toFixed(1)}m over budget until you sell elsewhere</span>`}</div>
    <div class="verdict ${clears?'pass':'miss'}">Gain ${d>0?'+':''}${d.toFixed(2)} pts a week —
      ${clears?`clears the +${gateOf().toFixed(2)} gate`:`below the +${gateOf().toFixed(2)} gate`}</div>
    <div class="acts">
      <button class="btn primary" id="pkgo" ${overClub(c)?'disabled':''}>Make this transfer</button>
      <button class="btn ghost" id="pkcancel">Cancel</button>
    </div>
  </div>`;
}

function wirePicker(){
  const close=()=>{
    document.getElementById('scrim').classList.remove('open');
    if(location.hash.startsWith('#replace-')||location.hash.startsWith('#buy-')) history.replaceState(null,'','#pitch');
  };
  document.getElementById('pkclose').onclick=close;

  const pos=document.getElementById('pkpos');
  if(pos) pos.onclick=e=>{
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
    /* Which end of R this click sets is the whole difference between the two modes: sell
       mode is browsing for who comes IN (R.sel varies), buy mode is browsing for who goes
       OUT (R.out varies) -- the target stays fixed in R.sel throughout. */
    if(R.mode==='buy') R.out=+b.dataset.id; else R.sel=+b.dataset.id;
    renderPicker();
  });

  const cancel=document.getElementById('pkcancel');
  if(cancel) cancel.onclick=()=>{
    if(R.mode==='buy') R.out=null; else R.sel=null;
    renderPicker();
  };

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
