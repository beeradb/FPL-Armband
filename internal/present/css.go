package present

// paletteCSS is the token set every page in this package shares.
//
// # The palette
//
// Cool paper and a blue-black ink, with a single amber accent spent only on the
// captain's armband and the figure a reader is actually tracking. Semantic green and
// red are deliberately NOT the accent: "was this transfer right" has to read at a
// glance without competing with "who is captain", and one hue cannot carry both.
//
// The neutrals are biased blue rather than pure grey. A pure mid-grey reads as
// unconsidered; a grey pulled toward the page's own accent reads as chosen.
//
// # Three theme states, not two
//
// A viewer who picks a theme stamps the root; a viewer on the default "system"
// setting stamps nothing, and that un-stamped case is the common one. So the bare
// :root carries the complete light palette, the prefers-color-scheme block covers the
// un-stamped viewer, and the [data-theme="dark"] block covers one who chose dark on a
// light OS. A token defined in only one of them renders one theme's text on the
// other's ground — the bug this package shipped until 2026-08-13, when only the media
// query existed.
//
// Every token is redefined in full in both dark blocks. Redefining a subset is how
// the since-removed pitch layout stayed bright green on a near-black page for weeks:
// the ground was defined once and the tokens over it twice.
//
// # Why the override hue is a sixth colour and not one of the five
//
// Amber is "the number to read first", teal is chips, green and red are verdict, and
// warn-amber is "act on this". A standing override is none of those. It is a statement
// about AUTHORSHIP — a human wrote this number, the model did not — and it carries no
// valence at all: an override is neither good news nor bad. Green or red would assert
// that a hand-set correction was one or the other, and the accent would put it in
// competition with the captain's armband, which is the one thing on the page allowed to
// win. Violet is the remaining slot. It appears only on badges and a 3px rule, never on
// a number. Contrast is 7.1:1 light and 8.4:1 dark.
//
// # Why the fixture tints are so weak
//
// --fdr1..5 sit behind a difficulty digit that is always printed, so the tint is
// redundant encoding and never the sole carrier of the rating. Five saturated cells on
// each of fifteen rows would make the fixture column the loudest thing on the page, and
// it is the fourth most important thing on it.
const paletteCSS = `
  :root {
    --bg:#EDF0F3; --panel:#FFFFFF; --panel2:#F4F6F9;
    --ink:#141A21; --ink2:#48545F; --ink3:#7A8791;
    --line:#DCE2E8; --line2:#C4CCD5;
    --accent:#B9762A; --onacc:#FFFFFF;
    --good:#2F7A57; --goodbg:#DFEDE6; --bad:#A8404E; --badbg:#F4E0E3;
    --chipbg:#1F5F73; --chipink:#FFFFFF;
    --warn:#B9770E; --warnbg:#FBF2E3; --warnink:#402F0D;
    --ovink:#5B4B9E; --ovbg:#ECE9F7; --ovline:#C9C0E8;
    --barfill:#BCC5CF;
    --pop-shadow:0 6px 24px rgba(20,26,33,.17);
    --fdr1:#C6E2D5; --fdr2:#DCEBE3; --fdr3:#E9ECF0; --fdr4:#F5D9DE; --fdr5:#E9C2CA;
    --sans:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;
    --mono:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
  }
  @media (prefers-color-scheme: dark) {
    :root:not([data-theme="light"]) {
      --bg:#11161C; --panel:#191F27; --panel2:#212933;
      --ink:#E6ECF2; --ink2:#A3B0BC; --ink3:#78848F;
      --line:#28313B; --line2:#3A4550;
      --accent:#DFA059; --onacc:#11161C;
      --good:#5FBF90; --goodbg:#183026; --bad:#E2757F; --badbg:#361E23;
      --chipbg:#3F93AC; --chipink:#11161C;
      --warn:#E0A33C; --warnbg:#2A2314; --warnink:#F1E3C6;
      --ovink:#B7A9EE; --ovbg:#221E33; --ovline:#443C63;
      --barfill:#43505E;
      --pop-shadow:0 8px 28px rgba(0,0,0,.6);
      --fdr1:#153229; --fdr2:#182722; --fdr3:#212933; --fdr4:#331F24; --fdr5:#46212A;
    }
  }
  :root[data-theme="dark"] {
    --bg:#11161C; --panel:#191F27; --panel2:#212933;
    --ink:#E6ECF2; --ink2:#A3B0BC; --ink3:#78848F;
    --line:#28313B; --line2:#3A4550;
    --accent:#DFA059; --onacc:#11161C;
    --good:#5FBF90; --goodbg:#183026; --bad:#E2757F; --badbg:#361E23;
    --chipbg:#3F93AC; --chipink:#11161C;
    --warn:#E0A33C; --warnbg:#2A2314; --warnink:#F1E3C6;
    --ovink:#B7A9EE; --ovbg:#221E33; --ovline:#443C63;
    --barfill:#43505E;
    --pop-shadow:0 8px 28px rgba(0,0,0,.6);
    --fdr1:#153229; --fdr2:#182722; --fdr3:#212933; --fdr4:#331F24; --fdr5:#46212A;
  }
`

// docCSS styles the standalone briefing document, which is long-form prose and wide
// tables where the squad pages are data. Same tokens, different furniture.
const docCSS = `
  body { margin:0; padding:2.5rem 1rem 5rem; background:var(--bg); color:var(--ink);
         font-family:var(--sans); line-height:1.6; }
  .doc { max-width:56rem; margin:0 auto; }
  .doc h1 { font-size:clamp(1.6rem,4vw,2.3rem); letter-spacing:-.025em; margin:0 0 .4rem;
            text-wrap:balance; line-height:1.15; font-weight:800; }
  .doc h2 { font-size:1.25rem; letter-spacing:-.015em; margin:2.4rem 0 .7rem;
            padding-bottom:.35rem; border-bottom:2px solid var(--line2); text-wrap:balance; }
  .doc h3 { font-family:var(--mono); font-size:.78rem; text-transform:uppercase;
            letter-spacing:.12em; color:var(--ink3); margin:1.8rem 0 .6rem; }
  .doc h4 { font-size:1rem; margin:1.3rem 0 .4rem; }
  .doc p { margin:0 0 .8rem; max-width:68ch; }
  .doc .sub { font-family:var(--mono); font-size:.72rem; letter-spacing:.12em;
              text-transform:uppercase; color:var(--ink3); margin:0 0 1.6rem; }
  .doc ul { margin:0 0 1rem; padding-left:1.15rem; max-width:68ch; }
  .doc li { margin:.25rem 0; }
  .doc hr { border:0; border-top:1px solid var(--line); margin:2.2rem 0; }
  .doc code { font-family:var(--mono); font-size:.86em; background:var(--panel2);
              border:1px solid var(--line); border-radius:3px; padding:.05rem .3rem; }
  .doc blockquote { margin:1rem 0; padding:.8rem 1rem; background:var(--warnbg);
                    border-left:3px solid var(--warn); color:var(--warnink);
                    border-radius:0 4px 4px 0; }
  .doc blockquote p { margin:0; }
  .doc .tscroll { overflow-x:auto; margin:0 0 1.2rem; border:1px solid var(--line);
                  border-radius:6px; }
  .doc table { width:100%; border-collapse:collapse; font-size:.86rem;
               font-variant-numeric:tabular-nums; }
  .doc th { text-align:left; padding:.5rem .7rem; background:var(--panel);
            border-bottom:1px solid var(--line); font-family:var(--mono);
            font-size:.66rem; text-transform:uppercase; letter-spacing:.09em;
            color:var(--ink3); font-weight:600; white-space:nowrap; }
  .doc td { padding:.45rem .7rem; border-bottom:1px solid var(--line); vertical-align:top; }
  .doc tbody tr:last-child td { border-bottom:0; }
`
