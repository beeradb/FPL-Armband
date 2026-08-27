#!/usr/bin/env python3
"""Render the assembled accuracy series as a static page.

    scripts/accuracy-series.py --out DIR          # assembles DIR/series.json
    scripts/accuracy-dashboard.py --in DIR --out DIR/index.html

# What this is and is not

It is a VIEW. `scripts/accuracy-series.py` is the generator and the citable
thing; this renders what that produced. ⚠️ **The published snapshot series is not
a citable record** — the workflow that publishes it says so — so nothing in this
repository should cite the rendered page as the source of a claim. It exists so a
figure can be looked at, not so a comment can link to it.

# Why the annotations are the point

A dated step tells you when the model changed. It does not tell you why, and
those are different facts: `5b97033` reads "Stop squad feasibility from depending
on score order", which is true and gives no hint it would trade early-season
calibration for late-season calibration three to one.

So each move carries the reason its commit declared in a `Figures-moved:`
trailer, or — for commits predating that convention — a row reconstructed after
the fact in `stats/figures-moved.csv`.

⚠️ **The page marks those differently and must keep doing so.** A declaration is
evidence about intent. A reconstruction is a later reader's account, written by
someone who already knew the figures had moved and was looking for a cause; that
search finds one more often than it should. Collapsing the two would launder
guesses into declarations.
"""
import argparse, html, json, os

CSS = """
:root{--ground:#f4f6f8;--panel:#fff;--panel-2:#eef2f5;--ink:#0f171d;--muted:#5a6a76;
--line:#dbe2e8;--line-soft:#e8edf1;--trace:#0d7d8a;--over:#a85410;--under:#2b5f8f;
--ok:#2e6b4f;--crit:#8f2f2f;--shadow:0 1px 2px rgba(15,23,29,.06),0 8px 24px rgba(15,23,29,.05);
--mono:ui-monospace,SFMono-Regular,"SF Mono",Menlo,Consolas,monospace;
--sans:ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
@media (prefers-color-scheme:dark){:root:not([data-theme="light"]){--ground:#0d1317;--panel:#141d23;
--panel-2:#18232a;--ink:#d9e3ea;--muted:#86969f;--line:#1f2c34;--line-soft:#1a252c;--trace:#3fb3bd;
--over:#d98b45;--under:#6ea8dd;--ok:#5da882;--crit:#d97070;
--shadow:0 1px 2px rgba(0,0,0,.4),0 8px 24px rgba(0,0,0,.3)}}
:root[data-theme="dark"]{--ground:#0d1317;--panel:#141d23;--panel-2:#18232a;--ink:#d9e3ea;
--muted:#86969f;--line:#1f2c34;--line-soft:#1a252c;--trace:#3fb3bd;--over:#d98b45;--under:#6ea8dd;
--ok:#5da882;--crit:#d97070;--shadow:0 1px 2px rgba(0,0,0,.4),0 8px 24px rgba(0,0,0,.3)}
*{box-sizing:border-box}
body{margin:0;background:var(--ground);color:var(--ink);font-family:var(--sans);font-size:15px;
line-height:1.5;-webkit-font-smoothing:antialiased}
.wrap{max-width:1120px;margin:0 auto;padding:40px 24px 72px;display:flex;flex-direction:column;gap:32px}
.eyebrow{font-size:11px;letter-spacing:.14em;text-transform:uppercase;color:var(--muted);font-family:var(--mono)}
h1{font-size:clamp(26px,3.4vw,38px);line-height:1.1;letter-spacing:-.022em;margin:6px 0 0;
text-wrap:balance;font-weight:620}
.lede{color:var(--muted);max-width:66ch;margin:10px 0 0}
.finding{background:var(--panel);border:1px solid var(--line);border-left:3px solid var(--over);
border-radius:3px;padding:18px 20px;box-shadow:var(--shadow)}
.finding h2{margin:0 0 6px;font-size:15px;letter-spacing:-.01em}
.finding p{margin:0;color:var(--muted);font-size:14px}
code{font-family:var(--mono);font-size:12.5px;background:var(--panel-2);padding:1px 5px;
border-radius:2px;color:var(--ink)}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(122px,1fr));gap:10px}
.card{background:var(--panel);border:1px solid var(--line);border-radius:3px;padding:12px 13px;
display:flex;flex-direction:column;gap:7px}
.card .gw{font-family:var(--mono);font-size:11px;letter-spacing:.08em;color:var(--muted)}
.card .val{font-family:var(--mono);font-size:21px;font-variant-numeric:tabular-nums;letter-spacing:-.02em}
.bar{height:3px;background:var(--line-soft);position:relative;border-radius:2px}
.bar i{position:absolute;top:0;bottom:0;border-radius:2px}
.card .delta{font-family:var(--mono);font-size:11px;font-variant-numeric:tabular-nums}
h2.sec{font-size:13px;letter-spacing:.1em;text-transform:uppercase;color:var(--muted);
font-family:var(--mono);margin:0 0 12px;font-weight:500}
.chartbox{background:var(--panel);border:1px solid var(--line);border-radius:3px;padding:18px;
box-shadow:var(--shadow)}
.legend{display:flex;flex-wrap:wrap;gap:6px;margin-bottom:14px}
.legend button{font-family:var(--mono);font-size:11px;padding:3px 9px;border-radius:2px;
border:1px solid var(--line);background:transparent;color:var(--muted);cursor:pointer;letter-spacing:.04em}
.legend button[aria-pressed="true"]{color:var(--panel);border-color:transparent}
.legend button:focus-visible{outline:2px solid var(--trace);outline-offset:2px}
svg{display:block;width:100%;height:auto;overflow:visible}
.readout{font-family:var(--mono);font-size:12px;color:var(--muted);margin-top:10px;min-height:3.4em;
font-variant-numeric:tabular-nums}
.readout b{color:var(--ink);font-weight:600}
.tablewrap{overflow-x:auto;background:var(--panel);border:1px solid var(--line);border-radius:3px;
box-shadow:var(--shadow)}
table{border-collapse:collapse;width:100%;font-size:13.5px}
th{text-align:left;font-family:var(--mono);font-size:10.5px;letter-spacing:.1em;text-transform:uppercase;
color:var(--muted);font-weight:500;padding:11px 14px;border-bottom:1px solid var(--line);white-space:nowrap}
td{padding:10px 14px;border-bottom:1px solid var(--line-soft);vertical-align:top}
tr:last-child td{border-bottom:0}
td.num{font-family:var(--mono);font-variant-numeric:tabular-nums;white-space:nowrap}
td.sha{font-family:var(--mono);font-size:12px;color:var(--muted)}
.pill{display:inline-block;font-family:var(--mono);font-size:11px;padding:1px 7px;border-radius:2px;
letter-spacing:.03em}
.pill.worse{background:color-mix(in srgb,var(--over) 15%,transparent);color:var(--over)}
.pill.better{background:color-mix(in srgb,var(--ok) 15%,transparent);color:var(--ok)}
.tag{display:inline-block;font-family:var(--mono);font-size:10px;letter-spacing:.06em;
text-transform:uppercase;padding:1px 6px;border-radius:2px;border:1px solid var(--line);color:var(--muted)}
.tag.declared{border-color:var(--ok);color:var(--ok)}
.tag.recon{border-color:var(--muted);color:var(--muted);border-style:dashed}
.tag.unexp{border-color:var(--crit);color:var(--crit)}
.why{color:var(--muted);font-size:12.5px;margin-top:5px;max-width:62ch}
.note{color:var(--muted);font-size:13px;max-width:70ch}
.note strong{color:var(--ink);font-weight:600}
footer{color:var(--muted);font-size:12px;border-top:1px solid var(--line);padding-top:18px;font-family:var(--mono)}
"""

BODY = """
<div class="wrap">
<header>
  <div class="eyebrow">FPL engine &middot; accuracy telemetry</div>
  <h1>Model calibration, __N__ published snapshots</h1>
  <p class="lede">Every push to <code>main</code> publishes an accuracy snapshot comparing predicted
  points against what players actually scored. The snapshots existed; nothing joined them into a
  series. This is that series, annotated with the reason each change was made.</p>
</header>
<div class="finding">
  <h2>__HEADLINE__</h2>
  <p>__SUB__</p>
</div>
<section>
  <h2 class="sec">Current calibration by model age</h2>
  <div class="grid" id="cards"></div>
  <p class="note" style="margin-top:12px"><strong>Ratio is actual &divide; predicted, so 1.000 is
  perfect.</strong> Below 1 the model over-predicts. The bar shows deviation from 1.000.</p>
</section>
<section>
  <h2 class="sec">The series</h2>
  <div class="chartbox">
    <div class="legend" id="legend"></div>
    <svg id="chart" viewBox="0 0 900 340" role="img" aria-label="Calibration ratio over time"></svg>
    <div class="readout" id="readout">Hover the chart to read a snapshot.</div>
  </div>
</section>
<section>
  <h2 class="sec">Every move, and why</h2>
  <div class="tablewrap"><table>
    <thead><tr><th>Cohort</th><th>Change</th><th>Delta</th><th>Date</th><th>Commit</th><th>What landed &amp; why</th></tr></thead>
    <tbody id="moves"></tbody>
  </table></div>
  <p class="note" style="margin-top:12px"><strong>A
  <span class="tag declared">declared</span> reason is the author saying why at the time; a
  <span class="tag recon">reconstructed</span> one is a later reader's account.</strong> They are
  not the same evidence &mdash; a reconstruction is written by someone who already knew the figures
  moved and was looking for a cause, and that search finds one more often than it should. CI refuses
  a commit that moves the figures without declaring why, so new rows should all be declarations.</p>
</section>
<section>
  <h2 class="sec">What this record cannot see</h2>
  <div class="chartbox">
    <p class="note" style="margin:0 0 14px"><strong>Every snapshot is on the reconstructed
    expected-goals-conceded input.</strong> CI publishes from a runner with no access to any local
    measured cache, so the series tracks the shipped model faithfully and is blind to the source
    local development may run against.</p>
    <p class="note" style="margin:0 0 14px"><strong>A drop after an input change would not mean the
    new input is worse.</strong> Every scoring constant was fitted against the reconstruction, so a
    swap leaves them stale by construction. The two estimators measure a dead heat on accuracy,
    which makes mistuning the likelier reading of any gap, not the weaker one.</p>
    <p class="note" style="margin:0"><strong>Ratio is a calibration level, not accuracy.</strong> A
    model can be perfectly calibrated and rank players badly, and an optimiser consumes the
    ordering. Mean absolute error travels alongside; neither measures rank.</p>
  </div>
</section>
<footer>__FOOTER__</footer>
</div>
"""

SCRIPT = r"""
const CO=DATA.cohorts,ROWS=DATA.rows;
const HUE={4:'#c2410c',8:'#a85410',12:'#0d7d8a',16:'#2b5f8f',20:'#4a5568',24:'#2e6b4f',28:'#6b4f8f',32:'#8f2f5f'};
const on=new Set([4,8,12,20,32]);
const ANN=ROWS.map((r,i)=>r.why?i:-1).filter(i=>i>=0);
function esc(s){return (s||'').replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]))}
function cards(){
  const last=ROWS[ROWS.length-1];
  document.getElementById('cards').innerHTML=CO.map(g=>{
    const c=last[g]; if(!c) return '';
    const r=c[0],dev=r-1,w=Math.min(Math.abs(dev)*100*3.2,50);
    const col=dev<0?'var(--over)':'var(--under)';
    const side=dev<0?`right:50%;width:${w}%`:`left:50%;width:${w}%`;
    return `<div class="card"><div class="gw">BUILT THRU GW${g}</div>
      <div class="val" style="color:${Math.abs(dev)>0.08?col:'var(--ink)'}">${r.toFixed(4)}</div>
      <div class="bar"><i style="${side};background:${col}"></i></div></div>`;}).join('');
}
function legend(){
  const el=document.getElementById('legend');
  el.innerHTML=CO.map(g=>`<button data-g="${g}" aria-pressed="${on.has(g)}" style="${on.has(g)?`background:${HUE[g]};`:''}">GW${g}</button>`).join('');
  el.querySelectorAll('button').forEach(b=>b.onclick=()=>{const g=+b.dataset.g;on.has(g)?on.delete(g):on.add(g);legend();chart();});
}
function chart(){
  const W=900,H=340,L=52,R=14,T=26,B=34;
  const vals=[];ROWS.forEach(r=>CO.forEach(g=>{if(on.has(g)&&r[g])vals.push(r[g][0])}));
  if(!vals.length){document.getElementById('chart').innerHTML='';return}
  let lo=Math.min(...vals,1),hi=Math.max(...vals,1);
  const pad=(hi-lo)*0.14||0.02;lo-=pad;hi+=pad;
  const X=i=>L+(i/(ROWS.length-1))*(W-L-R),Y=v=>T+(1-(v-lo)/(hi-lo))*(H-T-B);
  let s='';
  for(let k=0;k<=4;k++){const v=lo+(hi-lo)*k/4;
    s+=`<line x1="${L}" x2="${W-R}" y1="${Y(v)}" y2="${Y(v)}" stroke="var(--line-soft)"/>
        <text x="${L-9}" y="${Y(v)+4}" text-anchor="end" fill="var(--muted)" font-size="10.5" font-family="var(--mono)">${v.toFixed(3)}</text>`;}
  s+=`<line x1="${L}" x2="${W-R}" y1="${Y(1)}" y2="${Y(1)}" stroke="var(--muted)" stroke-dasharray="3 3"/>
      <text x="${W-R}" y="${Y(1)-7}" text-anchor="end" fill="var(--muted)" font-size="10.5" font-family="var(--mono)">1.000 perfect</text>`;
  ANN.forEach(i=>{const r=ROWS[i],c=r.expected===false?'var(--crit)':'var(--muted)';
    s+=`<line x1="${X(i)}" x2="${X(i)}" y1="${T-8}" y2="${H-B}" stroke="${c}" stroke-width="1.2" stroke-dasharray="${r.expected===false?'':'2 3'}"/>
        <text x="${X(i)}" y="${T-12}" text-anchor="middle" fill="${c}" font-size="10" font-family="var(--mono)">${esc(r.s)}</text>`;});
  CO.forEach(g=>{if(!on.has(g))return;let d='',n=0;
    ROWS.forEach((r,i)=>{if(!r[g])return;d+=(n++?'L':'M')+X(i).toFixed(1)+' '+Y(r[g][0]).toFixed(1)});
    s+=`<path d="${d}" fill="none" stroke="${HUE[g]}" stroke-width="1.7" stroke-linejoin="round"/>`;
    const li=ROWS.length-1;if(ROWS[li][g])s+=`<circle cx="${X(li)}" cy="${Y(ROWS[li][g][0])}" r="3" fill="${HUE[g]}"/>`;});
  s+=`<text x="${L}" y="${H-10}" fill="var(--muted)" font-size="10.5" font-family="var(--mono)">${ROWS[0].d}</text>
      <text x="${W-R}" y="${H-10}" text-anchor="end" fill="var(--muted)" font-size="10.5" font-family="var(--mono)">${ROWS[ROWS.length-1].d}</text>
      <rect x="${L}" y="${T}" width="${W-L-R}" height="${H-T-B}" fill="transparent" id="hit"/>`;
  const c=document.getElementById('chart');c.innerHTML=s;
  document.getElementById('hit').onmousemove=e=>{
    const bb=c.getBoundingClientRect();
    const i=Math.round(((e.clientX-bb.left)/bb.width*W-L)/(W-L-R)*(ROWS.length-1));
    const r=ROWS[Math.max(0,Math.min(ROWS.length-1,i))];if(!r)return;
    const parts=CO.filter(g=>on.has(g)&&r[g]).map(g=>`<b style="color:${HUE[g]}">GW${g}</b> ${r[g][0].toFixed(4)}`).join('  ');
    const why=r.why?`<br><span style="color:${r.expected===false?'var(--crit)':'var(--muted)'}">${r.src==='declared'?'declared':'reconstructed'}: ${esc(r.why).slice(0,150)}</span>`:'';
    document.getElementById('readout').innerHTML=`<b>${r.d}</b> ${esc(r.s)} &mdash; ${esc(r.m)}<br>${parts}${why}`;};
  document.getElementById('hit').onmouseleave=()=>{document.getElementById('readout').textContent='Hover the chart to read a snapshot.'};
}
function moves(){
  document.getElementById('moves').innerHTML=DATA.moves.map(m=>{
    const d=m.to-m.from,worse=Math.abs(m.to-1)>Math.abs(m.from-1);
    const tag=m.src==='declared'?'<span class="tag declared">declared</span>':
      m.src==='reconstructed'?'<span class="tag recon">reconstructed</span>':
      '<span class="tag">no reason given</span>';
    const exp=m.expected===false?' <span class="tag unexp">unexpected</span>':'';
    const why=m.why?`<div class="why">${tag}${exp} ${esc(m.why)}</div>`:`<div class="why">${tag}</div>`;
    return `<tr><td class="num">GW${m.g}</td>
      <td class="num">${m.from.toFixed(4)} &rarr; ${m.to.toFixed(4)}</td>
      <td class="num"><span class="pill ${worse?'worse':'better'}">${d>0?'+':''}${d.toFixed(4)}</span></td>
      <td class="num">${m.d}</td><td class="sha">${esc(m.s)}</td>
      <td>${esc(m.m)}${why}</td></tr>`;}).join('');
}
cards();legend();chart();moves();
"""


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--in", dest="src", default="accuracy-series")
    ap.add_argument("--out", default="accuracy-series/index.html")
    a = ap.parse_args()
    d = json.load(open(os.path.join(a.src, "series.json")))
    rows, moves = d["rows"], d["moves"]

    # Flatten: the page reads r["4"] as an array, not a nested object.
    flat = []
    for r in rows:
        e = {"s": r["s"], "d": r["d"], "m": r["m"], "why": r.get("why", ""),
             "src": r.get("src", ""), "expected": r.get("expected")}
        for g, c in r["c"].items():
            e[g] = [round(c["ratio"], 4), round(c.get("bias", 0), 4),
                    round(c.get("mean_absolute_error", 0), 4)]
        flat.append(e)

    big = max(moves, key=lambda m: abs(m["to"] - m["from"])) if moves else None
    if big:
        head = (f"{len(set(m['s'] for m in moves))} commit(s) moved the figures "
                f"in {len(rows)} snapshots")
        sub = (f"The largest: <code>{html.escape(big['s'])}</code> moved GW{big['g']} "
               f"by <b>{big['to']-big['from']:+.4f}</b>. "
               + (html.escape(big["why"])[:260] if big["why"]
                  else "No reason was declared for it."))
    else:
        head, sub = "Nothing moved", ("No figure moved beyond the threshold across "
                                      f"{len(rows)} snapshots — which is a result, not an empty run.")

    page = ("<title>Model calibration — running record</title>\n<style>" + CSS + "</style>\n"
            + BODY.replace("__N__", str(len(rows))).replace("__HEADLINE__", head)
                  .replace("__SUB__", sub)
                  .replace("__FOOTER__", f"{len(rows)} snapshots &middot; {rows[0]['d']} to "
                                         f"{rows[-1]['d']} &middot; generated by "
                                         f"scripts/accuracy-dashboard.py")
            + "\n<script>\nconst DATA=" + json.dumps({"rows": flat, "moves": moves,
                                                      "cohorts": d["cohorts"]},
                                                     separators=(",", ":"))
            + ";\n" + SCRIPT + "\n</script>\n")
    os.makedirs(os.path.dirname(a.out) or ".", exist_ok=True)
    open(a.out, "w").write(page)
    print(f"wrote {a.out} ({len(page)} bytes, {len(rows)} snapshots, {len(moves)} moves)")


if __name__ == "__main__":
    main()
