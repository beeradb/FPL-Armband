"""The toast, the new component CSS, and the behaviour behind Optimize and the chip fold.

One-shot, run from the repository root, then deleted.
"""

import io


def sub_file(path, old, new, count=1):
    s = io.open(path, encoding='utf-8').read()
    if s.count(old) != count:
        raise SystemExit('%s: expected %d of %r, found %d' % (path, count, old[:80], s.count(old)))
    io.open(path, 'w', encoding='utf-8').write(s.replace(old, new))


# ---------------------------------------------------------------- the toast
sub_file('internal/webui/assets/pages/app.html', '<body>', '''<body>

<!-- Where a failed save says so. A save that fails silently is the reader's work vanishing
     while the page still claims it is there. -->
<div id="toast" class="toast" role="status" aria-live="polite" hidden></div>
''')

# ---------------------------------------------------------------- component CSS
css = '''

/* ============================================================
   SAVING, AND SAYING SO
   ============================================================ */

/* The page stays readable while a save is in flight. Freezing it would hide the squad the
   reader is deciding about; dimming the controls says "not yet" without taking the page
   away. Pointer events go, so a second click cannot apply a change twice. */
body.saving .card,body.saving .btn,body.saving .iconbtn,body.saving .chipbtn,
body.saving .shape-row,body.saving .ovcard .rm{opacity:.55;pointer-events:none;}
body.saving{cursor:progress;}

/* The toast. Bottom centre on desktop, above the tab bar on mobile, and never over the
   thing it is talking about. */
.toast{
  position:fixed;left:50%;bottom:22px;transform:translateX(-50%);z-index:60;
  max-width:min(560px,calc(100vw - 32px));
  padding:11px 15px;border-radius:var(--r-sm);
  background:var(--panel3);border:1px solid var(--warn);color:var(--ink);
  font-size:13px;line-height:1.45;box-shadow:var(--shadow);
}

/* ============================================================
   THE CHIP FOLD
   ============================================================

   The chip row used to sit between the score bug and the pitch, which gave the best space
   on the screen to a decision taken four times a season. It is below the team now and
   closed by default. The summary still states what is planned, so nothing is hidden --
   only the room to change it, which is one click away. */
.chipfold{
  margin-top:14px;border:1px solid var(--line);border-radius:var(--r-md);
  background:var(--panel);overflow:hidden;
}
.chipfold>summary{
  display:flex;align-items:center;gap:10px;padding:11px 14px;cursor:pointer;
  list-style:none;font-size:13px;
}
.chipfold>summary::-webkit-details-marker{display:none;}
.chipfold>summary::after{
  content:'';margin-left:auto;width:7px;height:7px;flex:0 0 auto;
  border-right:2px solid var(--ink3);border-bottom:2px solid var(--ink3);
  transform:rotate(45deg);transition:transform .15s ease;
}
.chipfold[open]>summary::after{transform:rotate(-135deg);}
.chipfold>summary .dim{margin-left:0;}
.chipfold .chipnow{color:var(--ink);font-weight:600;}
.chipfold .chiprow{border-top:1px solid var(--line);border-radius:0;margin:0;}

/* Where the fifteen came from: the model's best, a varied opening, or the reader's own
   saved team. Quiet, because it is context rather than a number. */
.squadsource{font-size:11.5px;padding:0 2px 8px;font-family:var(--mono);}

@media (max-width:720px){
  .toast{bottom:calc(64px + env(safe-area-inset-bottom,0px));}
}
'''
with io.open('internal/webui/assets/static/armband.css', 'a', encoding='utf-8') as f:
    f.write(css)

# ---------------------------------------------------------------- behaviour
p = 'internal/webui/assets/static/app.js'
s = io.open(p, encoding='utf-8').read()


def sub(old, new, count=1):
    global s
    if s.count(old) != count:
        raise SystemExit('js: expected %d of %r, found %d' % (count, old[:90], s.count(old)))
    s = s.replace(old, new)


# The chip summary line, and where the fifteen came from.
sub('''function renderAll(){renderRail();renderReadout();renderChips();renderPitch();renderShapes();renderWhy();renderPlayers();renderOv();}''',
'''function renderChipSummary(){
  const el=document.getElementById('chipnow');
  if(!el) return;
  const g=gwState()||{};
  const c=(g.playable||CHIPS).find(x=>x.k===g.chip);
  el.textContent=c?c.n:'none this gameweek';
}

/* Where this fifteen came from, said plainly.
 *
 * The opening squad is deliberately varied rather than the model's single best, so a
 * reader looking at it deserves to know which they are looking at -- otherwise the tool
 * appears to be recommending something it is not. */
function renderSquadSource(){
  const el=document.getElementById('squadsource');
  if(!el) return;
  el.textContent = S.saved ? 'your saved team'
    : S.optimised ? "the model's best fifteen"
    : 'a strong opening fifteen — press Optimize for the model’s best';
  const opt=document.getElementById('optimise');
  if(opt) opt.disabled = !!S.optimised && !S.saved;
}

function renderAll(){renderRail();renderReadout();renderChips();renderChipSummary();renderSquadSource();renderPitch();renderShapes();renderWhy();renderPlayers();renderOv();}''')

# Optimize.
sub('''boot();''',
'''/* Optimize discards the arrangement and asks the model for its best fifteen.
 *
 * It clears the squad rather than sending a new one: the server is what knows what "best"
 * is, and a client that sent its own answer would be a second optimiser. */
const optimiseBtn=document.getElementById('optimise');
if(optimiseBtn) optimiseBtn.onclick=()=>save(pending=>{
  pending.opt=true;
  pending.squad=undefined;
  pending.xi=undefined;
  pending.bench=undefined;
  pending.cap=undefined;
  pending.vc=undefined;
});

boot();''')

# Placing a chip is a change worth keeping.
sub('''    const g=gwState(); g.chip = g.chip===b.dataset.chip ? null : b.dataset.chip; renderAll();''',
'''    const g=gwState(); g.chip = g.chip===b.dataset.chip ? null : b.dataset.chip;
    save(pending=>{
      pending.chips=Object.assign({},pending.chips||{});
      if(g.chip) pending.chips[String(g.gw)]=g.chip; else delete pending.chips[String(g.gw)];
    });''')

io.open(p, 'w', encoding='utf-8').write(s)
print('behaviour wired')
