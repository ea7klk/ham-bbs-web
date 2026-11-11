package main

import (
	"fmt"
	"html/template"
	"strings"
)

func parseTemplates(text map[string]map[string]any) *template.Template {
	funcs := template.FuncMap{
		"tr":              func(lang, key string) string { return translation(text, lang, key) },
		"langName":        func(code string) string { return languages[code] },
		"languageChoices": languageChoices,
		"aprsListText":    aprsListText,
		"aprsDateTime":    formatAPRSDateTime,
		"ackBadge":        sentAckBadge,
		"aprsStatus": func(lang, status string) string {
			key := "aprs_sent_status_" + normalizeAPRSStatus(status)
			value := translation(text, lang, key)
			if value == key {
				return status
			}
			return value
		},
		"checked": func(v bool) string {
			if v {
				return "checked"
			}
			return ""
		},
		// selected returns an attribute, not a bare word. A plain string is
		// refused in an attribute position by html/template, which emits
		// ZgotmplZ instead — so this helper silently did nothing before.
		"selected": func(a, b string) template.HTMLAttr {
			if a == b {
				return template.HTMLAttr("selected")
			}
			return template.HTMLAttr("")
		},
		"dict": func(values ...any) map[string]any {
			m := map[string]any{}
			for i := 0; i+1 < len(values); i += 2 {
				m[fmt.Sprint(values[i])] = values[i+1]
			}
			return m
		},
		"inc":      func(v int) int { return v + 1 },
		"canReply": func(depth int) bool { return depth < maxReplyDepth },
		// Section helpers. The scope type carries its translation keys as
		// methods, which a template cannot call because they are unexported, so
		// they are exposed here instead.
		"scopeLabel": func(lang string, scope aprsScope) string {
			return translation(text, lang, scope.labelKey())
		},
		"scopeDesc": func(lang string, scope aprsScope) string {
			return translation(text, lang, scope.descriptionKey())
		},
		"scopeValue": func(scope aprsScope) string { return scope.queryValue() },
		// navActive marks the current section. It compares by prefix so a board
		// page keeps "Boards" lit, and "/" is matched exactly so the home tab is
		// not lit on every page.
		"navActive": func(path, prefix string) bool {
			if prefix == "/" {
				return path == "/"
			}
			return path == prefix || strings.HasPrefix(path, prefix+"/")
		},
	}
	return template.Must(template.New("app").Funcs(funcs).Parse(templates))
}

const templates = `{{define "layout_start"}}
<!doctype html>
<html lang="{{.Lang}}">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="color-scheme" content="dark">
  <title>{{.AppName}}</title>
  <style>
    /* Packet Manual structure, Spectrum palette.
       Hard rules, monospace, uppercase labels and ruled tables — the shape a
       radio operator recognises. The colour comes from the spectrum direction:
       a deep indigo void lit by cyan, violet, magenta and lime. */
    :root {
      color-scheme: dark;
      --void:#080a18; --panel:#0e1024; --panel-2:#12152e;
      --rule:#2e3159; --rule-hi:#5b5fa0; --edge:#ffffff1f;
      --ink:#f2f0ff; --muted:#a7a3c8; --dim:#7b78a0;
      --cyan:#4fd8ff; --violet:#8b7bff; --pink:#ff6ec7; --lime:#b6f36a; --amber:#ffcf8f;
      --grad: linear-gradient(100deg, var(--cyan), var(--violet));
      --mono: ui-monospace, "SF Mono", "JetBrains Mono", "Cascadia Mono", Menlo, Consolas, "Liberation Mono", monospace;
    }
    * { box-sizing: border-box; }
    html { -webkit-text-size-adjust: 100%; }
    body {
      margin: 0; color: var(--ink); font-family: var(--mono); font-size: 14px; line-height: 1.55;
      background:
        radial-gradient(920px 480px at 4% -10%, #1c1254 0%, transparent 60%),
        radial-gradient(760px 420px at 98% 0%, #3b1440 0%, transparent 58%),
        var(--void);
      background-attachment: fixed;
      -webkit-font-smoothing: antialiased;
    }
    :focus-visible { outline: 2px solid var(--cyan); outline-offset: 2px; }
    a { color: inherit; }

    .frame { max-width: 1200px; margin: 0 auto; border-left: 1px solid var(--rule); border-right: 1px solid var(--rule); min-height: 100vh; }

    /* ---- title block ---- */
    .head { display: grid; grid-template-columns: 1fr auto; border-bottom: 2px solid var(--rule-hi); }
    .head.solo { grid-template-columns: 1fr; }
    .head-main { padding: 15px 20px; display: flex; flex-direction: column; gap: 4px; min-width: 0; }
    .logo { display: flex; align-items: center; gap: 10px; }
    .logo i { width: 18px; height: 18px; flex: none; display: block; background: conic-gradient(from 210deg, var(--violet), var(--cyan), var(--pink), var(--violet)); }
    .appname { font-size: 15px; font-weight: 800; letter-spacing: .18em; text-transform: uppercase; text-decoration: none; }
    .sub { font-size: 10.5px; letter-spacing: .12em; text-transform: uppercase; color: var(--muted); }
    /* The identity line carries a URL and a command, so it keeps its case: the
       uppercase above would turn "ssh -p" into "ssh -P", which is not the port
       option and would fail if copied. */
    .sub.ident { text-transform: none; letter-spacing: .04em; font-size: 11.5px; overflow-wrap: anywhere; }
    .head-meta { border-left: 2px solid var(--rule-hi); padding: 12px 18px; display: flex; flex-direction: column; gap: 6px; justify-content: center; font-size: 10.5px; letter-spacing: .1em; text-transform: uppercase; color: var(--muted); }
    .head-meta .who b { color: var(--cyan); letter-spacing: .06em; }
    .head-meta .role { font-style: normal; color: var(--amber); }

    /* ---- section tabs ---- */
    .tabs { display: flex; flex-wrap: wrap; border-bottom: 2px solid var(--rule-hi); }
    .tabs a, .tabs button.link {
      display: inline-flex; align-items: center; gap: 6px; padding: 11px 15px; font: inherit; font-size: 10.5px;
      letter-spacing: .14em; text-transform: uppercase; text-decoration: none; color: var(--muted);
      background: none; border: 0; border-right: 1px solid var(--rule); cursor: pointer;
    }
    .tabs a:hover, .tabs button.link:hover { color: var(--ink); background: #ffffff0a; }
    .tabs a[aria-current="page"] { background: var(--grad); color: #0b0a1c; font-weight: 700; }
    .tabs .push { margin-left: auto; border-left: 1px solid var(--rule); border-right: 0; }

    main { padding: 22px 20px 44px; }

    /* ---- page heading ---- */
    .pagehead { display: flex; justify-content: space-between; align-items: flex-end; gap: 18px; flex-wrap: wrap; padding-bottom: 16px; border-bottom: 2px solid var(--rule-hi); margin-bottom: 20px; }
    .kicker { margin: 0 0 8px; font-size: 10px; letter-spacing: .2em; text-transform: uppercase; color: var(--cyan); }
    h1 { margin: 0; font-size: clamp(20px, 3vw, 28px); line-height: 1.08; letter-spacing: -.01em; text-transform: uppercase; font-weight: 800; }
    h2 { margin: 0 0 12px; font-size: 13px; letter-spacing: .06em; text-transform: uppercase; }
    h3 { margin: 0 0 8px; font-size: 12px; letter-spacing: .1em; text-transform: uppercase; color: var(--muted); }
    .lede { margin: 10px 0 0; color: var(--muted); font-size: 13.5px; max-width: 68ch; }
    .pageactions { display: flex; gap: 8px; flex-wrap: wrap; }

    /* ---- hero ---- */
    /* The pinned bulletin: the page's first element, so it gets weight without
       becoming a different design — a rule, a tinted panel and a readable title. */
    .pinned { border: 1px solid var(--rule-hi); border-left: 3px solid var(--cyan); background: linear-gradient(180deg, #12152e, #0e1024); padding: 16px 18px; margin-bottom: 26px; }
    .pinned .kicker { margin: 0 0 8px; }
    .pinned h2 { margin: 0 0 10px; font-size: clamp(17px, 2.4vw, 23px); line-height: 1.15; letter-spacing: -.01em; text-transform: none; color: var(--ink); }
    .pinned .pinned-body { margin: 0 0 12px; font-size: 13.5px; }
    .pinned .metaline { margin: 0; }
    .pinned .metaline a { color: var(--cyan); text-decoration: none; }
    .pinned .metaline a:hover { text-decoration: underline; }
    .hero { padding: 18px 0 26px; border-bottom: 2px solid var(--rule-hi); }
    .hero h1 { font-size: clamp(26px, 5vw, 44px); max-width: 20ch; }
    .hero-actions { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 20px; }

    /* ---- metric strip ---- */
    .status { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 1px; margin: 0; background: var(--rule); border: 1px solid var(--rule); border-top: 0; }
    .status .cell { background: var(--panel); padding: 15px 16px; }
    .status dt { margin: 0 0 6px; font-size: 10px; letter-spacing: .16em; text-transform: uppercase; color: var(--muted); font-weight: 700; }
    .status dd { margin: 0; font-size: 30px; font-weight: 800; letter-spacing: -.03em; line-height: 1; }

    /* ---- section strip ---- */
    .strip { display: flex; justify-content: space-between; align-items: baseline; gap: 12px; padding: 12px 0 8px; border-bottom: 2px solid var(--rule-hi); margin-bottom: 0; }
    .strip h2, .strip h3 { margin: 0; font-size: 10.5px; letter-spacing: .18em; text-transform: uppercase; font-weight: 700; color: var(--ink); }
    .strip span { font-size: 10px; letter-spacing: .1em; text-transform: uppercase; color: var(--muted); }

    /* ---- ruled rows ---- */
    .rows { border-bottom: 1px solid var(--rule); margin-bottom: 26px; }
    .rowlink { display: grid; grid-template-columns: 1fr auto; gap: 14px; align-items: center; padding: 12px 2px; border-bottom: 1px solid var(--rule); text-decoration: none; }
    .rowlink:last-child { border-bottom: 0; }
    .rowlink b { font-size: 13.5px; font-weight: 700; }
    .rowlink small { display: block; margin-top: 2px; color: var(--muted); font-size: 12px; letter-spacing: 0; }
    .rowlink .n { font-size: 12px; color: var(--lime); font-weight: 700; }
    .rowlink:hover b, .rowlink:hover .n { color: var(--cyan); }

    /* ---- panels and tiles ---- */
    .card, .panel { background: var(--panel); border: 1px solid var(--rule); padding: 16px; }
    .card h2, .panel h2 { font-size: 10.5px; letter-spacing: .18em; text-transform: uppercase; color: var(--muted); margin: 0 0 12px; font-weight: 700; }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 1px; background: var(--rule); border: 1px solid var(--rule); margin-bottom: 24px; }
    .grid > * { background: var(--panel); }
    .tiles { display: grid; grid-template-columns: repeat(auto-fit, minmax(230px, 1fr)); gap: 1px; background: var(--rule); border: 1px solid var(--rule); margin-bottom: 24px; }
    .tile { display: block; background: var(--panel); padding: 18px; text-decoration: none; }
    .tile h2 { margin: 0; font-size: 10.5px; letter-spacing: .18em; text-transform: uppercase; color: var(--muted); font-weight: 700; }
    .tile .metric { margin: 8px 0 0; font-size: 32px; font-weight: 800; line-height: 1; letter-spacing: -.03em; }
    .tile .meta { display: block; margin-top: 8px; font-size: 10px; letter-spacing: .1em; text-transform: uppercase; color: var(--muted); }
    .tile:hover h2, .tile:hover .metric { color: var(--cyan); }
    .split { display: grid; grid-template-columns: 1.6fr 1fr; gap: 34px; margin-top: 26px; }
    .split.single { grid-template-columns: 1fr; }
    /* stacked cards share one hairline instead of each carrying its own box */
    .cards { display: grid; gap: 1px; background: var(--rule); border: 1px solid var(--rule); margin-bottom: 24px; }
    .cards > .card { border: 0; }
    .wide { max-width: 760px; }
    .narrow { max-width: 620px; }
    .mt-sm { margin-top: 12px; } .mt { margin-top: 16px; } .mt-lg { margin-top: 26px; } .mt-xl { margin-top: 30px; }
    .mb { margin-bottom: 22px; } .mb-lg { margin-bottom: 26px; }
    .stack { display: grid; gap: 14px; max-width: 760px; }
    /* a label that is also a control: prose, but small caps like the rest */
    .metaline { font-size: 10.5px; letter-spacing: .08em; text-transform: uppercase; color: var(--muted); }
    .quiet { font-size: 12.5px; color: var(--muted); }
    .prose p { margin: 0 0 10px; }
    .prose p:last-child { margin-bottom: 0; }
    .w-narrow { max-width: 150px; }
    .card p { margin: 0 0 12px; }
    .card p:last-child { margin-bottom: 0; }
    .row > label, .row > label.checkbox-label { margin-bottom: 0; }
    .accent { color: var(--cyan); }
    details.post { background: var(--panel); border: 1px solid var(--rule); padding: 0; margin-bottom: 26px; }
    details.post > summary { cursor: pointer; list-style: none; padding: 13px 16px; font-size: 10.5px; letter-spacing: .16em; text-transform: uppercase; color: var(--cyan); }
    details.post > summary::-webkit-details-marker { display: none; }
    details.post > summary::before { content: "+ "; }
    details.post[open] > summary::before { content: "− "; }
    details.post > summary:hover { color: var(--ink); }
    details.post[open] > summary { border-bottom: 1px solid var(--rule); }
    details.post > form { padding: 16px; }

    /* ---- buttons ---- */
    .btn {
      display: inline-flex; align-items: center; justify-content: center; gap: 8px; font: inherit; font-size: 10.5px;
      font-weight: 700; letter-spacing: .12em; text-transform: uppercase; padding: 11px 15px; cursor: pointer;
      border: 1px solid transparent; background: var(--grad); color: #0b0a1c; text-decoration: none; white-space: nowrap;
    }
    .btn:hover { filter: brightness(1.12); }
    .btn.secondary { background: transparent; border-color: var(--rule-hi); color: var(--ink); }
    .btn.secondary:hover { border-color: var(--cyan); color: var(--cyan); filter: none; }
    .btn.danger { background: var(--pink); color: #2a0716; border-color: transparent; }
    .btn.danger:hover { filter: brightness(1.1); }
    .btn.small { padding: 7px 10px; font-size: 9.5px; letter-spacing: .1em; }
    .btn:disabled { opacity: .45; cursor: not-allowed; filter: none; }
    /* the sign-in hint: prose, then the command in a box that selects as a unit */
    .sshhint { margin-top: 26px; padding-top: 16px; border-top: 1px solid var(--rule); max-width: 760px; font-size: 12.5px; color: var(--muted); }
    .sshhint code { display: inline-block; margin-top: 8px; padding: 8px 11px; border: 1px solid var(--rule-hi); background: #0b0d1e; color: var(--cyan); font-family: var(--mono); font-size: 12.5px; overflow-wrap: anywhere; user-select: all; }
    /* Section bar: which slice of the history this page shows. Uses the same
       filled/current treatment as the top tabs, one level down. */
    .sections { display: flex; flex-wrap: wrap; gap: 0; border: 1px solid var(--rule-hi); border-bottom: 0; margin-bottom: 0; }
    .sections a { display: inline-flex; align-items: baseline; gap: 8px; padding: 10px 14px; text-decoration: none; color: var(--muted); font-size: 10.5px; letter-spacing: .14em; text-transform: uppercase; border-right: 1px solid var(--rule); }
    .sections a:last-child { border-right: 0; }
    .sections a b { font-weight: 700; color: var(--dim); font-variant-numeric: tabular-nums; }
    .sections a:hover { color: var(--ink); background: #ffffff0a; }
    .sections a[aria-current="page"] { background: var(--grad); color: #0b0a1c; font-weight: 700; }
    .sections a[aria-current="page"] b { color: #0b0a1c; }
    @media (max-width: 620px) { .sections a { flex: 1 1 auto; justify-content: center; padding: 9px 8px; font-size: 9.5px; letter-spacing: .08em; } }
    .switch { display: inline-flex; align-items: center; gap: 14px; flex-wrap: wrap; }
    .switch form { margin: 0; }
    .switch-state { font-size: 10px; letter-spacing: .14em; text-transform: uppercase; color: var(--muted); }
    .switch-state b { font-weight: 700; }
    .switch-state b.on { color: var(--lime); }
    .switch-state b.off { color: var(--amber); }

    /* ---- forms ---- */
    label { display: block; margin-bottom: 6px; font-size: 10.5px; letter-spacing: .14em; text-transform: uppercase; color: var(--muted); font-weight: 600; }
    input, textarea, select {
      width: 100%; font: inherit; font-size: 13.5px; padding: 10px 11px; color: var(--ink);
      background: #0b0d1e; border: 1px solid var(--rule-hi);
    }
    input:focus, textarea:focus, select:focus { outline: 2px solid var(--cyan); outline-offset: 1px; border-color: var(--cyan); }
    input::placeholder, textarea::placeholder { color: var(--dim); }
    textarea { min-height: 130px; resize: vertical; line-height: 1.5; }
    select { cursor: pointer; }
    form.stack { display: grid; gap: 14px; max-width: 760px; }
    .cols { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 14px; }
    .row { display: flex; gap: 10px; flex-wrap: wrap; align-items: center; }
    .row form { margin: 0; }
    .between { display: flex; justify-content: space-between; gap: 14px; align-items: flex-start; }
    .muted { color: var(--muted); }
    .pre { white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.5; }
    ul.plain { margin: 0; padding-left: 18px; }
    ul.plain li { margin: 4px 0; }
    .actions { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
    .actions form { margin: 0; }
    .sr { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; border: 0; }

    /* ---- notices ---- */
    .flash { border: 1px solid var(--lime); border-left: 3px solid var(--lime); background: #b6f36a14; color: #e2fcc6; padding: 11px 14px; margin: 0 0 18px; font-size: 13px; }
    .error { border: 1px solid var(--pink); border-left: 3px solid var(--pink); background: #ff6ec714; color: #ffd3ee; padding: 11px 14px; margin: 0 0 18px; font-size: 13px; }

    /* ---- tables ---- */
    .scroll { overflow-x: auto; -webkit-overflow-scrolling: touch; }
    table { width: 100%; border-collapse: collapse; font-size: 12.5px; border-top: 2px solid var(--rule-hi); border-bottom: 2px solid var(--rule-hi); }
    th, td { text-align: left; padding: 10px 10px; border-bottom: 1px solid var(--rule); vertical-align: top; }
    th { font-size: 9.5px; letter-spacing: .16em; text-transform: uppercase; color: var(--muted); font-weight: 700; background: #ffffff06; border-bottom: 1px solid var(--rule-hi); }
    tbody tr:hover { background: #ffffff05; }
    table strong { color: var(--ink); }
    a.rowlink { display: block; color: var(--ink); text-decoration: none; font-weight: 700; }
    a.rowlink:hover { color: var(--cyan); }
    table a.rowlink { font-weight: 600; }

    /* ---- messages ---- */
    .message { background: var(--panel); border: 1px solid var(--rule); border-left: 3px solid var(--cyan); padding: 14px 16px; margin: 0 0 12px; }
    .message > .between > div > strong { display: block; font-size: 13.5px; margin-bottom: 3px; }
    .indent-1 { margin-left: 22px; border-left-color: var(--violet); }
    .indent-2 { margin-left: 44px; border-left-color: var(--pink); }
    .indent-3 { margin-left: 66px; border-left-color: var(--lime); }
    .indent-4 { margin-left: 88px; border-left-color: var(--amber); }
    .indent-5 { margin-left: 110px; border-left-color: var(--rule-hi); }
    .message details { margin-top: 10px; }
    .message summary { cursor: pointer; font-size: 10px; letter-spacing: .14em; text-transform: uppercase; color: var(--cyan); }
    .message summary:hover { color: var(--ink); }

    /* ---- APRS table ---- */
    .aprs-table { table-layout: fixed; min-width: 840px; }
    .aprs-table th, .aprs-table td { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; padding: 9px 8px; }
    .aprs-table a.rowlink { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .aprs-table .time-col, .aprs-table .time-col a.rowlink { white-space: normal; overflow: visible; text-overflow: clip; }
    .aprs-table .message-col, .aprs-table .message-col a.rowlink { white-space: pre-wrap; overflow: visible; text-overflow: clip; overflow-wrap: anywhere; font-weight: 400; }
    .aprs-time { display: flex !important; flex-direction: column; line-height: 1.25; }
    .aprs-table .selection-col { width: 8%; }
    .aprs-table .time-col { width: 13%; }
    .aprs-table .callsign-col { width: 14%; }
    .aprs-table .destination-col { width: 15%; }
    .aprs-table .message-col { width: 33%; }
    .aprs-table.sent-table .message-col { width: 28%; }
    .aprs-table .actions-col { width: 15%; }
    .aprs-table .ack-col { width: 8%; }
    .aprs-table .status-col { width: 11%; }
    .aprs-table .actions { flex-wrap: nowrap; }
    label.checkbox-label { display: inline-flex; align-items: center; gap: 6px; margin: 0; color: inherit; font-size: inherit; letter-spacing: inherit; text-transform: none; }
    input[type=checkbox] { width: auto; padding: 0; accent-color: var(--cyan); }
    .badge { display: inline-flex; min-width: 1.5rem; justify-content: center; font-weight: 800; }
    .ack-ok { color: var(--lime); } .ack-rejected { color: var(--pink); } .ack-partial { color: var(--amber); }

    /* ---- misc ---- */
    .detail-grid { display: grid; grid-template-columns: minmax(130px, 200px) 1fr; gap: 0; margin: 0; border-top: 1px solid var(--rule); }
    .detail-grid dt { padding: 9px 10px 9px 0; color: var(--muted); font-size: 10px; letter-spacing: .14em; text-transform: uppercase; border-bottom: 1px solid var(--rule); }
    .detail-grid dd { margin: 0; padding: 9px 0; border-bottom: 1px solid var(--rule); overflow-wrap: anywhere; }
    .bulk-bar { display: flex; justify-content: flex-end; margin-top: 12px; }
    .pagination-bar { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; margin-top: 16px; padding-top: 12px; border-top: 1px solid var(--rule); }
    .pagination-summary { color: var(--muted); font-size: 10.5px; letter-spacing: .1em; text-transform: uppercase; }
    .pagination-size { display: flex; align-items: center; gap: 8px; margin: 0; }
    .pagination-size label { margin: 0; white-space: nowrap; }
    .pagination-size select { width: auto; min-width: 76px; }
    .pagination-links { display: flex; gap: 8px; }
    .language-switcher { display: flex; align-items: center; gap: 6px; margin: 0; }
    .language-switcher select { width: auto; min-width: 116px; padding: 5px 8px; font-size: 11px; }
    hr { border: 0; border-top: 1px solid var(--rule); margin: 14px 0; }

    footer { border-top: 2px solid var(--rule-hi); margin-top: 34px; }
    .foot { display: flex; justify-content: space-between; gap: 14px; flex-wrap: wrap; padding: 14px 20px; font-size: 10px; letter-spacing: .12em; text-transform: uppercase; color: var(--muted); }
    .foot a { text-decoration: none; border-bottom: 1px solid var(--rule-hi); }
    .foot a:hover { color: var(--cyan); border-color: var(--cyan); }

    @media (max-width: 900px) {
      .head { grid-template-columns: 1fr; }
      .head-meta { border-left: 0; border-top: 1px solid var(--rule); flex-direction: row; align-items: center; justify-content: space-between; flex-wrap: wrap; padding: 10px 16px; }
      .head-main { padding: 13px 16px; }
      main { padding: 18px 14px 36px; }
      .split { grid-template-columns: 1fr; gap: 26px; }
      .hero { padding-top: 8px; }
      .indent-1, .indent-2, .indent-3, .indent-4, .indent-5 { margin-left: 10px; }
      .detail-grid { grid-template-columns: 1fr; }
      .detail-grid dt { padding-bottom: 0; border-bottom: 0; }
      .detail-grid dd { padding-top: 2px; }
    }
    @media (max-width: 620px) {
      body { font-size: 13.5px; }
      .tabs a, .tabs button.link { padding: 10px 11px; font-size: 9.5px; letter-spacing: .1em; }
      .tabs .push { margin-left: 0; }
      .status dd { font-size: 24px; }
      .status .cell { padding: 12px; }
      .rowlink { gap: 10px; }
      /* Keep tables readable instead of scrolling them into a corner: drop the
         reference columns that are least useful on a phone. */
      .tbl-directory th:nth-child(3), .tbl-directory td:nth-child(3),
      .tbl-directory th:nth-child(5), .tbl-directory td:nth-child(5),
      .tbl-directory th:nth-child(6), .tbl-directory td:nth-child(6) { display: none; }
      .tbl-users th:nth-child(3), .tbl-users td:nth-child(3),
      .tbl-users th:nth-child(4), .tbl-users td:nth-child(4) { display: none; }
      .tbl-users form { flex-direction: column; align-items: stretch; }
      .aprs-table { min-width: 720px; }
    }
    @media (prefers-reduced-motion: no-preference) {
      .tabs a, .btn, .rowlink, .tile { transition: color .12s ease, background .12s ease, border-color .12s ease; }
    }
  </style>
</head>
<body>
<div class="frame">
<header>
  <div class="head{{if not .User}} solo{{end}}">
    <div class="head-main">
      <div class="logo"><i aria-hidden="true"></i><a class="appname" href="/">{{.AppName}}</a></div>
      <div class="sub ident">https://bbs.ea7klk.es · The web companion to ssh -p 2222 bbs@bbs.ea7klk.es</div>
    </div>
    {{if .User}}
    <div class="head-meta">
      <span class="who">{{tr .Lang "logged_in_as"}} <b>{{.User.Callsign}}</b>{{if .IsSysop}} <em class="role">{{tr .Lang "sysop"}}</em>{{end}}</span>
      <form class="language-switcher" method="post" action="/language"><label class="sr" for="header-language">{{tr .Lang "language"}}</label><select id="header-language" name="language" onchange="this.form.submit()">{{range languageChoices}}<option value="{{.Code}}" {{selected $.Lang .Code}}>{{.Name}}</option>{{end}}</select><input type="hidden" name="return_to" value="{{.Path}}"></form>
    </div>
    {{end}}
  </div>
  {{if .User}}
  <nav class="tabs" aria-label="{{tr .Lang "web_home"}}">
    <a href="/" {{if navActive .Path "/"}}aria-current="page"{{end}}>{{tr .Lang "web_home"}}</a>
    <a href="/bulletins" {{if navActive .Path "/bulletins"}}aria-current="page"{{end}}>{{tr .Lang "menu_bulletins"}}</a>
    <a href="/boards" {{if navActive .Path "/boards"}}aria-current="page"{{end}}>{{tr .Lang "web_message_boards"}}</a>
    <a href="/directory" {{if navActive .Path "/directory"}}aria-current="page"{{end}}>{{tr .Lang "menu_directory"}}</a>
    <a href="/aprs" {{if navActive .Path "/aprs"}}aria-current="page"{{end}}>{{tr .Lang "menu_aprs"}}</a>
    <a href="/profile" {{if navActive .Path "/profile"}}aria-current="page"{{end}}>{{.User.Callsign}}</a>
    {{if .IsSysop}}<a href="/admin/users" {{if navActive .Path "/admin"}}aria-current="page"{{end}}>{{tr .Lang "sysop"}}</a>{{end}}
    <form method="post" action="/logout" class="push"><button class="link" type="submit">{{tr .Lang "web_logout"}}</button></form>
  </nav>
  {{end}}
</header>
<main id="main">
{{if .Flash}}<div class="flash">{{.Flash}}</div>{{end}}
{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
<script>
  function aprsSelectionBoxes(group) {
    return Array.prototype.slice.call(document.querySelectorAll('[data-aprs-group="' + group + '"]'));
  }
  function syncAPRSSelection(group) {
    var boxes = aprsSelectionBoxes(group);
    var selected = boxes.filter(function (box) { return box.checked; });
    var header = document.getElementById('aprs-select-all-' + group);
    if (header) {
      header.checked = boxes.length > 0 && selected.length === boxes.length;
      header.indeterminate = selected.length > 0 && selected.length < boxes.length;
    }
    var bulk = document.querySelector('[data-aprs-delete-selected="' + group + '"]');
    if (bulk) bulk.disabled = selected.length === 0;
  }
  function toggleAPRSSelection(group, checked) {
    aprsSelectionBoxes(group).forEach(function (box) { box.checked = checked; });
    syncAPRSSelection(group);
  }
</script>
{{end}}
{{define "layout_end"}}</main>
<footer><div class="foot"><span>{{.AppName}} · {{.Location}}</span><span><a href="/about">{{tr .Lang "about_title"}}</a> · <a href="/resources">{{tr .Lang "resources_title"}}</a></span></div></footer>
</div>
</body></html>{{end}}

{{define "login"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{.Location}}</p><h1>{{tr .Lang "web_login"}}</h1></div></div>
<form class="stack" method="post" action="/login"><div><label>{{tr .Lang "callsign_prompt"}}</label><input name="callsign" autocomplete="username" required autofocus></div><div><label>{{tr .Lang "web_bbs_password"}}</label><input name="password" type="password" autocomplete="current-password" required></div><div class="row"><button class="btn">{{tr .Lang "web_login"}}</button><a class="btn secondary" href="/register">{{tr .Lang "web_register"}}</a><a class="btn secondary" href="/forgot">{{tr .Lang "forgot_button"}}</a></div></form>
<div class="sshhint"><span>{{tr .Lang "web_ssh_note"}}</span> <code>ssh -p 2222 bbs@bbs.ea7klk.es</code></div>
{{template "layout_end" .}}{{end}}

{{define "register"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{tr .Lang "web_login"}}</p><h1>{{tr .Lang "web_register"}}</h1></div></div>
<form class="stack" method="post" action="/register">{{template "user_fields" dict "User" .Data.User "Languages" .Data.Languages "Callsign" .Data.Callsign "RequireCallsign" true "RequirePassword" true "Lang" .Lang}}<button class="btn">{{tr .Lang "web_create_account"}}</button></form>
{{template "layout_end" .}}{{end}}

{{define "user_fields"}}{{if .RequireCallsign}}<div><label>{{tr .Lang "callsign_prompt"}}</label><input name="callsign" value="{{.Callsign}}" required></div>{{end}}<div class="cols"><div><label>{{tr .Lang "full_name"}}</label><input name="full_name" value="{{.User.FullName}}" required></div><div><label>{{tr .Lang "email"}}</label><input name="email" type="email" value="{{.User.Email}}" required></div></div><div class="cols"><div><label>{{tr .Lang "maidenhead"}}</label><input name="maidenhead" value="{{.User.Maidenhead}}"></div><div><label>{{tr .Lang "language"}}</label><select name="language">{{range .Languages}}<option value="{{.Code}}" {{selected $.User.Language .Code}}>{{.Name}}</option>{{end}}</select></div></div><div class="cols"><div><label>{{tr .Lang "qth"}}</label><input name="qth" value="{{.User.QTH}}"></div><div><label>{{tr .Lang "rig"}}</label><input name="rig" value="{{.User.Rig}}"></div></div><div><label>{{tr .Lang "enable_aprs"}}</label><select name="enable_aprs"><option value="false">false</option><option value="true" {{if .User.EnableAPRS}}selected{{end}}>true</option></select></div><div class="cols"><div><label>{{if .RequirePassword}}{{tr .Lang "password"}}{{else}}{{tr .Lang "new_password"}}{{end}}</label><input name="new_password" type="password" {{if .RequirePassword}}required{{end}}></div><div><label>{{tr .Lang "verify_password"}}</label><input name="verify_password" type="password" {{if .RequirePassword}}required{{end}}></div></div>{{end}}

{{define "home"}}{{template "layout_start" .}}
{{with .Data.LatestBulletin}}
{{/* The newest bulletin leads the page: it is the thing most likely to have
     changed since the last visit, and it is the only content here that someone
     else wrote. */}}
<section class="pinned" aria-labelledby="pinned-title">
  <p class="kicker">{{tr $.Lang "web_latest_bulletin"}}</p>
  <h2 id="pinned-title">{{.Title}}</h2>
  <p class="pre pinned-body">{{.Body}}</p>
  <p class="metaline">{{.Updated}}{{if .From}} · {{.From}}{{end}} <a href="/bulletins">{{tr $.Lang "menu_bulletins"}} →</a></p>
</section>
{{end}}
<section class="hero">
  <p class="kicker">{{tr .Lang "web_dashboard"}}</p>
  <h1>{{.AppName}}</h1>
  <p class="lede">{{.Topic}}</p>
  <div class="hero-actions"><a class="btn" href="/boards">{{tr .Lang "web_message_boards"}}</a><a class="btn secondary" href="/bulletins">{{tr .Lang "menu_bulletins"}}</a><a class="btn secondary" href="/aprs">{{tr .Lang "menu_aprs"}}</a></div>
</section>
<dl class="status">
  <div class="cell"><dt>{{tr .Lang "menu_bulletins"}}</dt><dd>{{.Data.Bulletins}}</dd></div>
  <div class="cell"><dt>{{tr .Lang "web_message_boards"}}</dt><dd>{{.Data.Boards}}</dd></div>
  <div class="cell"><dt>{{tr .Lang "menu_directory"}}</dt><dd>{{.Data.Users}}</dd></div>
  <div class="cell"><dt>{{tr .Lang "aprs_received_messages"}}</dt><dd>{{.Data.Received}}</dd></div>
</dl>
<div class="split{{if not .Data.Latest}} single{{end}}">
  <section>
    <div class="strip"><h2>{{tr .Lang "web_message_boards"}}</h2><span>{{tr .Lang "menu_messages"}}</span></div>
    <div class="rows">{{range .Data.BoardList}}<a class="rowlink" href="/boards/{{.ID}}"><span><b>{{.Name}}</b><small>{{.Description}}</small></span><span class="n">{{index $.Data.Counts .ID}} →</span></a>{{else}}<p class="muted">{{tr .Lang "no_boards"}}</p>{{end}}</div>
  </section>
  {{if .Data.Latest}}
  <aside>
    <div class="strip"><h2>{{tr .Lang "menu_bulletins"}}</h2><a class="btn secondary small" href="/bulletins">{{tr .Lang "web_detail"}}</a></div>
    <div class="rows">{{range .Data.Latest}}<a class="rowlink" href="/bulletins"><span><b>{{.Title}}</b><small>{{.Updated}}{{if .From}} · {{.From}}{{end}}</small></span></a>{{end}}</div>
  </aside>
  {{end}}
</div>
{{template "layout_end" .}}{{end}}

{{define "profile"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{.User.Callsign}}</p><h1>{{tr .Lang "web_profile"}}</h1></div></div>
<form class="stack" method="post" action="/profile">{{template "user_fields" dict "User" .User "Languages" .Data.Languages "RequireCallsign" false "RequirePassword" false "Lang" .Lang}}<button class="btn">{{tr .Lang "web_save_profile"}}</button></form>
{{template "layout_end" .}}{{end}}

{{define "directory"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{tr .Lang "menu_directory"}}</p><h1>{{tr .Lang "web_station_directory"}}</h1></div></div>
<div class="scroll"><table class="tbl-directory"><thead><tr><th>{{tr .Lang "target_callsign"}}</th><th>{{tr .Lang "full_name"}}</th><th>{{tr .Lang "maidenhead"}}</th><th>{{tr .Lang "qth"}}</th><th>{{tr .Lang "rig"}}</th><th>{{tr .Lang "last_connection"}}</th><th>APRS</th></tr></thead><tbody>{{range .Data}}<tr><td><strong>{{.Callsign}}</strong></td><td>{{.FullName}}</td><td>{{.Maidenhead}}</td><td>{{.QTH}}</td><td>{{.Rig}}</td><td>{{.LastSeen}}</td><td>{{.EnableAPRS}}</td></tr>{{end}}</tbody></table></div>
{{template "layout_end" .}}{{end}}

{{define "bulletins"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{tr .Lang "menu_bulletins"}}</p><h1>{{tr .Lang "menu_bulletins"}}</h1></div>{{if .IsSysop}}<div class="pageactions"><a class="btn" href="/bulletins/new">{{tr .Lang "web_new_bulletin"}}</a></div>{{end}}</div>
<div class="cards">{{range .Data}}
<article class="card">
  <div class="between"><div><h2>{{.Title}}</h2><div class="metaline">{{.Updated}}{{if .From}} · {{.From}}{{end}}</div></div>{{if $.IsSysop}}<a class="btn secondary small" href="/bulletins/{{.ID}}/edit">{{tr $.Lang "web_edit"}}</a>{{end}}</div>
  <p class="pre mt-sm">{{.Body}}</p>
</article>
{{else}}<p class="muted">{{tr .Lang "no_bulletins"}}</p>{{end}}</div>
{{template "layout_end" .}}{{end}}

{{define "bulletin_form"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{tr .Lang "menu_bulletins"}}</p><h1>{{if .Data.ID}}{{tr .Lang "bulletin_edit_title"}}{{else}}{{tr .Lang "bulletin_form_title"}}{{end}}</h1></div></div>
<form class="stack" method="post"><div><label>{{tr .Lang "bulletin_title"}}</label><input name="title" value="{{.Data.Title}}" required></div><div><label>{{tr .Lang "bulletin_body"}}</label><textarea name="body" required>{{.Data.Body}}</textarea></div><div class="row"><button class="btn">{{tr .Lang "save_button"}}</button><a class="btn secondary" href="/bulletins">{{tr .Lang "cancel_button"}}</a></div></form>
{{if .Data.ID}}<form method="post" action="/bulletins/{{.Data.ID}}/delete" class="mt"><button class="btn danger">{{tr .Lang "delete_button"}}</button></form>{{end}}
{{template "layout_end" .}}{{end}}

{{define "boards"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{tr .Lang "web_message_boards"}}</p><h1>{{tr .Lang "web_message_boards"}}</h1></div>{{if .IsSysop}}<div class="pageactions"><a class="btn" href="/boards/new">{{tr .Lang "web_new_board"}}</a></div>{{end}}</div>
<div class="rows">{{range .Data.Boards}}<a class="rowlink" href="/boards/{{.ID}}"><span><b>{{.Name}}</b><small>{{.Description}}</small></span><span class="n">{{index $.Data.Counts .ID}} {{tr $.Lang "menu_messages"}} →</span></a>{{else}}<p class="muted">{{tr .Lang "no_boards"}}</p>{{end}}</div>
{{template "layout_end" .}}{{end}}

{{define "board"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{tr .Lang "web_message_boards"}}</p><h1>{{.Data.Board.Name}}</h1>{{if .Data.Board.Description}}<p class="lede">{{.Data.Board.Description}}</p>{{end}}</div>{{if .IsSysop}}<div class="pageactions"><a class="btn secondary" href="/boards/{{.Data.Board.ID}}/edit">{{tr .Lang "web_edit_board"}}</a></div>{{end}}</div>
<details class="post"><summary>{{tr .Lang "web_post_message"}}</summary>
<form class="stack" method="post" action="/boards/{{.Data.Board.ID}}/post"><div><label>{{tr .Lang "subject"}}</label><input name="subject" required></div><div><label>{{tr .Lang "message_body"}}</label><textarea name="body" required></textarea></div><button class="btn">{{tr .Lang "web_post"}}</button></form>
</details>
<div class="strip"><h2>{{tr .Lang "menu_messages"}}</h2><span>{{tr .Lang "web_message_boards"}}</span></div>
<div class="mt">{{range .Data.Messages}}{{template "message" dict "Node" . "Root" $}}{{else}}<p class="muted">{{tr .Lang "web_no_messages_yet"}}</p>{{end}}</div>
{{template "layout_end" .}}{{end}}

{{define "message"}}<section class="message indent-{{.Node.Depth}}"><div class="between"><div><strong>{{.Node.Row.Subject}}</strong><div class="metaline">{{tr .Root.Lang "from"}} {{.Node.Row.From}} · {{tr .Root.Lang "at"}} {{.Node.Row.Created}}{{if .Node.Row.Edited}} · {{tr .Root.Lang "web_edit"}} {{.Node.Row.Edited}}{{end}}</div></div>{{if .Root.IsSysop}}<div class="actions"><a class="btn secondary small" href="/messages/{{.Node.Row.ID}}/edit">{{tr .Root.Lang "web_edit"}}</a><form method="post" action="/messages/{{.Node.Row.ID}}/delete"><button class="btn danger small">{{tr .Root.Lang "delete_button"}}</button></form></div>{{end}}</div><p class="pre mt-sm">{{.Node.Row.Body}}</p>{{if canReply .Node.Depth}}<details><summary>{{tr .Root.Lang "reply_button"}}</summary><form class="stack" method="post" action="/messages/{{.Node.Row.ID}}/reply"><div><label>{{tr .Root.Lang "subject"}}</label><input name="subject" value="Re: {{.Node.Row.Subject}}" required></div><div><label>{{tr .Root.Lang "message_body"}}</label><textarea name="body" required></textarea></div><button class="btn">{{tr .Root.Lang "reply_button"}}</button></form></details>{{else}}<div class="metaline mt-sm">{{tr .Root.Lang "thread_too_deep"}}</div>{{end}}{{range .Node.Replies}}{{template "message" dict "Node" . "Root" $.Root}}{{end}}</section>{{end}}

{{define "board_form"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{tr .Lang "web_message_boards"}}</p><h1>{{if .Data.ID}}{{tr .Lang "board_rename_title"}}{{else}}{{tr .Lang "board_form_title"}}{{end}}</h1></div></div>
<form class="stack" method="post"><div><label>{{tr .Lang "board_name"}}</label><input name="name" value="{{.Data.Name}}" required></div><div><label>{{tr .Lang "board_description"}}</label><input name="description" value="{{.Data.Description}}"></div><div class="row"><button class="btn">{{tr .Lang "save_button"}}</button><a class="btn secondary" href="/boards">{{tr .Lang "cancel_button"}}</a></div></form>
{{if .Data.ID}}<form method="post" action="/boards/{{.Data.ID}}/delete" class="mt"><button class="btn danger">{{tr .Lang "delete_button"}}</button></form>{{end}}
{{template "layout_end" .}}{{end}}

{{define "aprs_toggle"}}
{{/* One control, used wherever APRS can be switched on or off. The button names
     the action it performs rather than the state, so a single click is never
     ambiguous, and the hidden field carries the value to write. */}}
<div class="switch">
  <span class="switch-state">{{tr .Lang "aprs_status"}}:
    <b class="{{if .User.EnableAPRS}}on{{else}}off{{end}}">{{if .User.EnableAPRS}}{{tr .Lang "enabled"}}{{else}}{{tr .Lang "disabled"}}{{end}}</b>
  </span>
  <form method="post" action="/aprs/toggle">
    <input type="hidden" name="enable_aprs" value="{{if .User.EnableAPRS}}false{{else}}true{{end}}">
    <input type="hidden" name="return_to" value="{{.Path}}">
    <button class="btn {{if .User.EnableAPRS}}secondary{{end}}" type="submit">{{if .User.EnableAPRS}}{{tr .Lang "web_aprs_disable_action"}}{{else}}{{tr .Lang "web_aprs_enable_action"}}{{end}}</button>
  </form>
</div>
{{end}}

{{define "aprs"}}
{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">APRS</p><h1>{{tr .Lang "menu_aprs"}}</h1></div><div class="pageactions">{{template "aprs_toggle" .}}</div></div>
<div class="tiles">
  <a class="tile" href="/aprs/received"><h2>{{tr .Lang "aprs_received_messages"}}</h2><p class="metric">{{.Data.Received.Total}}</p><span class="meta">{{tr .Lang "web_messages_total"}}</span></a>
  <a class="tile" href="/aprs/sent"><h2>{{tr .Lang "aprs_sent_messages"}}</h2><p class="metric">{{.Data.Sent.Total}}</p><span class="meta">{{tr .Lang "web_messages_total"}}</span></a>
  <a class="tile" href="/aprs/send"><h2>{{tr .Lang "aprs_send_message"}}</h2><span class="meta">{{tr .Lang "web_send_aprs_intro"}}</span></a>
  <a class="tile" href="/aprs/bulletins"><h2>{{tr .Lang "aprs_bulletins_menu"}}</h2><span class="meta">{{tr .Lang "aprs_bulletin_window"}}</span></a>
  <a class="tile" href="/aprs/aprsph"><h2>{{tr .Lang "aprsph_menu_title"}}</h2><span class="meta">{{tr .Lang "aprsph_menu_hint"}}</span></a>
</div>

<div class="strip"><h2>{{tr .Lang "aprs_join_aprs_thursday"}}</h2><span>ANSRVR · {{.Data.ThursdayGroup}}</span></div>
<div class="card mt wide">
  {{if not .Data.ThursdayActive}}<p class="quiet">{{tr .Lang "aprs_not_thursday_warning"}}</p>{{end}}
  <div class="row mt-sm">
    <form method="post" action="/aprs/thursday/join"><button class="btn" type="submit" {{if not .Data.ThursdayActive}}disabled{{end}}>{{tr .Lang "aprs_join_aprs_thursday"}}</button></form>
    <form method="post" action="/aprs/thursday/keepalive"><button class="btn secondary" type="submit">{{tr .Lang "aprs_keepalive_aprs_thursday"}}</button></form>
    <form method="post" action="/aprs/thursday/leave" onsubmit="return confirm({{printf "%q" (tr .Lang "aprs_leave_confirm")}})"><button class="btn danger" type="submit">{{tr .Lang "aprs_leave_aprs_thursday"}}</button></form>
  </div>
</div>
{{template "layout_end" .}}
{{end}}

{{define "aprs_bulletins"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">APRS</p><h1>{{tr .Lang "aprs_bulletin_title"}}</h1><p class="lede">{{tr .Lang "web_aprs_bulletins_hint"}}</p></div><div class="pageactions"><a class="btn secondary" href="/aprs">{{tr .Lang "web_back_to_aprs"}}</a></div></div>
{{if .Data.Categories}}
<div class="rows">{{range .Data.Categories}}<a class="rowlink" href="/aprs/bulletins/{{.ID}}"><span><b>{{.Label}}</b><small>{{.Desc}}</small></span><span class="n">{{.Count}} →</span></a>{{end}}</div>
{{else}}<p class="muted">{{tr .Lang "aprs_bulletin_none"}}</p><p class="quiet">{{tr .Lang "aprs_bulletin_none_hint"}}</p>{{end}}
{{template "layout_end" .}}{{end}}

{{define "aprs_bulletin_list"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{tr .Lang "aprs_bulletin_title"}}</p><h1>{{.Data.Label}}</h1><p class="lede">{{.Data.Desc}}</p></div><div class="pageactions"><a class="btn secondary" href="/aprs/bulletins">{{tr .Lang "aprs_bulletin_title"}}</a><a class="btn secondary" href="/aprs">{{tr .Lang "web_back_to_aprs"}}</a></div></div>
{{if .Data.Bulletins}}
<div class="scroll"><table><thead><tr><th>{{tr .Lang "aprs_bulletin_last_seen"}}</th><th>{{tr .Lang "from"}}</th><th>{{tr .Lang "aprs_bulletin_group"}}</th><th>{{tr .Lang "aprs_text"}}</th><th>{{tr .Lang "aprs_bulletin_times_heard"}}</th></tr></thead><tbody>
{{range .Data.Bulletins}}<tr>
<td><a class="rowlink" href="/aprs/bulletins/{{$.Data.Category}}/{{.Item.ID}}">{{.Age}}</a></td>
<td><a class="rowlink" href="/aprs/bulletins/{{$.Data.Category}}/{{.Item.ID}}">{{.Item.From}}</a></td>
<td>{{if .Item.Group}}{{.Item.Group}}{{else}}{{tr $.Lang "aprs_bulletin_no_group"}}{{end}}</td>
<td class="pre">{{.Item.Text}}</td>
<td>{{.Item.SeenCount}}</td>
</tr>{{end}}
</tbody></table></div>
{{else}}<p class="muted">{{tr .Lang "aprs_bulletin_none"}}</p><p class="quiet">{{tr .Lang "aprs_bulletin_none_hint"}}</p>{{end}}
{{template "layout_end" .}}{{end}}

{{define "aprs_bulletin_detail"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{tr .Lang "aprs_bulletin_detail"}}</p><h1>{{.Data.CategoryName}}</h1></div><div class="pageactions"><a class="btn secondary" href="/aprs/bulletins/{{.Data.Category}}">{{.Data.CategoryName}}</a><a class="btn secondary" href="/aprs/bulletins">{{tr .Lang "aprs_bulletin_title"}}</a></div></div>
<section class="card"><dl class="detail-grid">
<dt>{{tr .Lang "from"}}</dt><dd>{{.Data.Item.From}}</dd>
<dt>{{tr .Lang "aprs_bulletin_group"}}</dt><dd>{{if .Data.Item.Group}}{{.Data.Item.Group}}{{else}}{{tr .Lang "aprs_bulletin_no_group"}}{{end}}</dd>
<dt>{{tr .Lang "aprs_bulletin_category"}}</dt><dd>{{.Data.CategoryName}}</dd>
<dt>{{tr .Lang "aprs_bulletin_slot"}}</dt><dd>{{.Data.Item.Slot}}</dd>
<dt>{{tr .Lang "aprs_bulletin_first_seen"}}</dt><dd>{{.Data.Item.FirstSeen}}</dd>
<dt>{{tr .Lang "aprs_bulletin_last_seen"}}</dt><dd>{{.Data.Item.LastSeen}}</dd>
<dt>{{tr .Lang "aprs_bulletin_times_heard"}}</dt><dd>{{.Data.Item.SeenCount}}</dd>
<dt>{{tr .Lang "aprs_text"}}</dt><dd class="pre">{{.Data.Item.Text}}</dd>
</dl>{{if .Data.Item.Raw}}<hr><dl class="detail-grid"><dt>{{tr .Lang "aprs_raw_packet"}}</dt><dd class="pre">{{.Data.Item.Raw}}</dd></dl>{{end}}</section>
{{template "layout_end" .}}{{end}}

{{define "aprs_aprsph"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">APRS · {{.Data.Bot}}</p><h1>{{tr .Lang "aprsph_menu_title"}}</h1><p class="lede">{{tr .Lang "aprsph_menu_hint"}}</p></div><div class="pageactions"><a class="btn secondary" href="/aprs">{{tr .Lang "web_back_to_aprs"}}</a></div></div>

<div class="split">
  <div>
    <div class="strip"><h2>{{tr .Lang "aprsph_checkin"}}</h2></div>
    <form class="stack mt" method="post" action="/aprs/aprsph">
      <input type="hidden" name="kind" value="checkin">
      <div><label for="aprsph-cq">{{tr .Lang "aprsph_checkin_text"}}</label><textarea id="aprsph-cq" name="phrase" maxlength="{{.Data.Max}}" required></textarea></div>
      <button class="btn" type="submit">{{tr .Lang "send_button"}}</button>
    </form>
    <form class="stack mt" method="post" action="/aprs/aprsph">
      <input type="hidden" name="kind" value="quiet">
      <div><label for="aprsph-net">{{tr .Lang "aprsph_checkin_text_optional"}}</label><textarea id="aprsph-net" name="phrase" maxlength="{{.Data.Max}}"></textarea></div>
      <button class="btn secondary" type="submit">{{tr .Lang "send_button"}}</button>
    </form>

    <div class="strip mt-lg"><h2>{{tr .Lang "aprsph_net_messages"}}</h2><span>{{tr .Lang "aprsph_count_hint"}}</span></div>
    <form class="stack mt" method="post" action="/aprs/aprsph">
      <input type="hidden" name="kind" value="last">
      <div class="cols"><div><label for="aprsph-last-count">{{tr .Lang "aprsph_count_hint"}}</label><select id="aprsph-last-count" name="count"><option value="5">5</option><option value="10">10</option><option value="15">15</option></select></div><div><label for="aprsph-last-thurs">{{tr .Lang "aprsph_thursday_only"}}</label><select id="aprsph-last-thurs" name="thursday"><option value="false">{{tr .Lang "no_label"}}</option><option value="true">{{tr .Lang "yes_label"}}</option></select></div></div>
      <button class="btn" type="submit">{{tr .Lang "send_button"}}</button>
    </form>

    <div class="strip mt-lg"><h2>{{tr .Lang "aprsph_my_messages"}}</h2></div>
    <form class="stack mt" method="post" action="/aprs/aprsph">
      <input type="hidden" name="kind" value="mine">
      <div class="cols"><div><label for="aprsph-mine-count">{{tr .Lang "aprsph_count_hint"}}</label><select id="aprsph-mine-count" name="count"><option value="5">5</option><option value="10">10</option><option value="15">15</option></select></div><div><label for="aprsph-mine-target">{{tr .Lang "aprsph_callsign_optional"}}</label><input id="aprsph-mine-target" name="target" maxlength="12"></div></div>
      <div><label for="aprsph-mine-thurs">{{tr .Lang "aprsph_thursday_only"}}</label><select id="aprsph-mine-thurs" name="thursday"><option value="false">{{tr .Lang "no_label"}}</option><option value="true">{{tr .Lang "yes_label"}}</option></select></div>
      <button class="btn" type="submit">{{tr .Lang "send_button"}}</button>
    </form>

    <div class="strip mt-lg"><h2>{{tr .Lang "aprsph_search"}}</h2></div>
    <form class="stack mt" method="post" action="/aprs/aprsph">
      <input type="hidden" name="kind" value="search">
      <div class="cols"><div><label for="aprsph-search">{{tr .Lang "aprsph_search_prompt"}}</label><input id="aprsph-search" name="phrase" required></div><div><label for="aprsph-search-count">{{tr .Lang "aprsph_count_hint"}}</label><select id="aprsph-search-count" name="count"><option value="5">5</option><option value="10">10</option><option value="15">15</option></select></div></div>
      <button class="btn" type="submit">{{tr .Lang "send_button"}}</button>
    </form>
  </div>

  <aside>
    <div class="strip"><h2>{{tr .Lang "aprsph_bot_info"}}</h2><span>{{.Data.Bot}}</span></div>
    <div class="rows mt-sm">
      <form method="post" action="/aprs/aprsph"><input type="hidden" name="kind" value="list"><button class="btn secondary small" type="submit">{{tr .Lang "aprsph_list"}}</button></form>
      <form method="post" action="/aprs/aprsph"><input type="hidden" name="kind" value="inbox"><button class="btn secondary small" type="submit">{{tr .Lang "aprsph_inbox"}}</button></form>
      <form method="post" action="/aprs/aprsph"><input type="hidden" name="kind" value="path"><button class="btn secondary small" type="submit">{{tr .Lang "aprsph_path"}}</button></form>
      <form method="post" action="/aprs/aprsph"><input type="hidden" name="kind" value="position"><button class="btn secondary small" type="submit">{{tr .Lang "aprsph_position"}}</button></form>
      <form method="post" action="/aprs/aprsph"><input type="hidden" name="kind" value="status"><button class="btn secondary small" type="submit">{{tr .Lang "aprsph_status"}}</button></form>
      <form method="post" action="/aprs/aprsph"><input type="hidden" name="kind" value="checkout"><button class="btn secondary small" type="submit">{{tr .Lang "aprsph_checkout"}}</button></form>
    </div>

    <div class="strip mt-lg"><h2>{{tr .Lang "aprsph_help"}}</h2></div>
    <div class="card mt-sm"><ul class="plain">{{range .Data.Lines}}<li class="pre">{{.}}</li>{{end}}</ul></div>
  </aside>
</div>
{{template "layout_end" .}}{{end}}

{{define "aprs_received"}}
{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">APRS</p><h1>{{tr .Lang "aprs_received_messages"}}</h1><p class="lede">{{scopeDesc .Lang .Data.Section}}</p></div><div class="pageactions"><a class="btn secondary" href="{{.Data.SentURL}}">{{tr .Lang "aprs_sent_messages"}}</a><a class="btn secondary" href="/aprs">{{tr .Lang "web_back_to_aprs"}}</a></div></div>
{{template "aprs_sections" .Data}}
{{template "aprs_received_table" .Data}}
{{template "layout_end" .}}
{{end}}

{{define "aprs_sections"}}
<nav class="sections" aria-label="{{tr .Lang "aprs_section_prompt"}}">{{range .Sections}}<a href="{{.URL}}" {{if .Current}}aria-current="page"{{end}}><span>{{.Label}}</span><b>{{.Count}}</b></a>{{end}}</nav>
{{end}}

{{define "aprs_sent"}}
{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">APRS</p><h1>{{tr .Lang "aprs_sent_messages"}}</h1><p class="lede">{{scopeDesc .Lang .Data.Section}}</p></div><div class="pageactions"><a class="btn secondary" href="{{.Data.ReceivedURL}}">{{tr .Lang "aprs_received_messages"}}</a><a class="btn secondary" href="/aprs">{{tr .Lang "web_back_to_aprs"}}</a></div></div>
{{template "aprs_sections" .Data}}
{{template "aprs_sent_table" .Data}}
{{template "layout_end" .}}
{{end}}

{{define "aprs_send"}}
{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">APRS</p><h1>{{tr .Lang "aprs_send_message"}}</h1></div><div class="pageactions"><a class="btn secondary" href="/aprs">{{tr .Lang "web_back_to_aprs"}}</a></div></div>
<form class="stack" method="post" action="/aprs/send">
  <div class="cols"><div><label>{{tr .Lang "aprs_destination_callsign"}}</label><input name="destination" required></div><div><label>{{tr .Lang "aprs_destination_ssid"}}</label><input name="destination_ssid" maxlength="2" value="0"></div></div>
  <div><label>{{tr .Lang "aprs_text"}}</label><textarea name="text"></textarea></div>
  <button class="btn">{{tr .Lang "send_button"}}</button>
</form>
<div class="strip mt-xl"><h2>{{tr .Lang "aprs_enable_title"}}</h2></div>
<section class="card mt wide">
  <div class="row between">{{template "aprs_toggle" .}}</div>
  <p class="quiet mt">{{tr .Lang "web_aprs_toggle_help"}}</p>
  <p class="quiet">{{tr .Lang "web_aprs_worker_note"}}</p>
</section>
{{template "layout_end" .}}
{{end}}

{{define "aprs_received_table"}}
<form method="post" action="/aprs/received/delete">
<div class="scroll"><table class="aprs-table received-table">
  <thead><tr><th class="selection-col"><label class="checkbox-label" for="aprs-select-all-received"><input type="checkbox" id="aprs-select-all-received" onchange="toggleAPRSSelection('received', this.checked)"><span>{{tr .Lang "select_all"}}</span></label></th><th class="time-col">{{tr .Lang "at"}}</th><th class="callsign-col">{{tr .Lang "from"}}</th><th class="destination-col">{{tr .Lang "aprs_destination_callsign"}}</th><th class="message-col">{{tr .Lang "aprs_text"}}</th><th class="actions-col">{{tr .Lang "web_actions"}}</th></tr></thead>
  <tbody>{{range .Rows}}
  {{$date := aprsDateTime .At}}
  <tr>
    <td><input type="checkbox" name="received_ids" value="{{.ID}}" data-aprs-group="received" onchange="syncAPRSSelection('received')" aria-label="{{tr $.Lang "aprs_received_message_detail"}}"></td>
    <td><a class="rowlink aprs-time" href="/aprs/received/{{.ID}}"><span>{{ $date.Date }}</span><span>{{ $date.Time }}</span></a></td>
    <td><a class="rowlink" href="/aprs/received/{{.ID}}">{{.From}}</a></td>
    <td><a class="rowlink" href="/aprs/received/{{.ID}}">{{.To}}</a></td>
    <td class="message-col"><a class="rowlink" href="/aprs/received/{{.ID}}">{{aprsListText .Text}}</a></td>
    <td><div class="actions"><a class="btn secondary small" href="/aprs/received/{{.ID}}">{{tr $.Lang "web_detail"}}</a><button class="btn danger small" type="submit" formaction="/aprs/received/{{.ID}}/delete" onclick="return confirm({{printf "%q" (tr $.Lang "confirm_delete_aprs_message")}})">{{tr $.Lang "delete_button"}}</button></div></td>
  </tr>
  {{else}}
  <tr><td colspan="6" class="muted">{{tr .Lang "web_no_aprs_received"}}</td></tr>
  {{end}}</tbody>
</table></div>
<div class="bulk-bar"><button class="btn danger small" type="submit" data-aprs-delete-selected="received" onclick="return confirm({{printf "%q" (tr .Lang "confirm_delete_aprs_selected")}})" disabled>{{tr .Lang "delete_selected"}}</button></div>
</form>
{{template "aprs_pagination" .}}
{{end}}

{{define "aprs_sent_table"}}
<form method="post" action="/aprs/sent/delete">
<div class="scroll"><table class="aprs-table sent-table">
  <thead><tr><th class="selection-col"><label class="checkbox-label" for="aprs-select-all-sent"><input type="checkbox" id="aprs-select-all-sent" onchange="toggleAPRSSelection('sent', this.checked)"><span>{{tr .Lang "select_all"}}</span></label></th><th class="ack-col">{{tr .Lang "web_ack_status"}}</th><th class="time-col">{{tr .Lang "at"}}</th><th class="destination-col">{{tr .Lang "aprs_destination_callsign"}}</th><th class="status-col">{{tr .Lang "status"}}</th><th class="message-col">{{tr .Lang "aprs_text"}}</th><th class="actions-col">{{tr .Lang "web_actions"}}</th></tr></thead>
  <tbody>{{range .Rows}}
  {{$ack := ackBadge .}}{{$date := aprsDateTime .At}}
  <tr>
    <td><input type="checkbox" name="sent_ids" value="{{.ID}}" data-aprs-group="sent" onchange="syncAPRSSelection('sent')" aria-label="{{tr $.Lang "aprs_sent_message_detail"}}"></td>
    <td><a class="rowlink" href="/aprs/sent/{{.ID}}"><span class="badge {{$ack.Class}}" title="{{tr $.Lang $ack.LabelKey}}">{{$ack.Icon}}</span></a></td>
    <td><a class="rowlink aprs-time" href="/aprs/sent/{{.ID}}"><span>{{ $date.Date }}</span><span>{{ $date.Time }}</span></a></td>
    <td><a class="rowlink" href="/aprs/sent/{{.ID}}">{{.To}}</a></td>
    <td><a class="rowlink" href="/aprs/sent/{{.ID}}">{{aprsStatus $.Lang .Status}}</a></td>
    <td class="message-col"><a class="rowlink" href="/aprs/sent/{{.ID}}">{{aprsListText .Text}}</a></td>
    <td><div class="actions"><a class="btn secondary small" href="/aprs/sent/{{.ID}}">{{tr $.Lang "web_detail"}}</a><button class="btn danger small" type="submit" formaction="/aprs/sent/{{.ID}}/delete" onclick="return confirm({{printf "%q" (tr $.Lang "confirm_delete_aprs_message")}})">{{tr $.Lang "delete_button"}}</button></div></td>
  </tr>
  {{else}}
  <tr><td colspan="7" class="muted">{{tr .Lang "web_no_aprs_sent"}}</td></tr>
  {{end}}</tbody>
</table></div>
<div class="bulk-bar"><button class="btn danger small" type="submit" data-aprs-delete-selected="sent" onclick="return confirm({{printf "%q" (tr .Lang "confirm_delete_aprs_selected")}})" disabled>{{tr .Lang "delete_selected"}}</button></div>
</form>
{{template "aprs_pagination" .}}
{{end}}

{{define "aprs_pagination"}}
{{$p := .Pagination}}
<div class="pagination-bar">
  <div class="pagination-summary">{{tr .Lang "web_page"}} {{$p.Page}} / {{$p.Pages}} · {{$p.Total}} {{tr .Lang "web_messages_total"}}</div>
  <form class="pagination-size" method="get"><label for="aprs-per-page">{{tr .Lang "web_page_size"}}</label><select id="aprs-per-page" name="per_page" onchange="this.form.submit()">{{range $size := $p.PageSizes}}<option value="{{$size}}" {{if eq $size $p.PerPage}}selected{{end}}>{{$size}}</option>{{end}}</select><input type="hidden" name="page" value="1">{{with scopeValue .Section}}<input type="hidden" name="section" value="{{.}}">{{end}}<button class="btn secondary small" type="submit">{{tr .Lang "web_apply"}}</button></form>
  <div class="pagination-links">{{if $p.HasPrevious}}<a class="btn secondary small" href="{{$p.PreviousURL}}">{{tr .Lang "web_previous"}}</a>{{end}}{{if $p.HasNext}}<a class="btn secondary small" href="{{$p.NextURL}}">{{tr .Lang "web_next"}}</a>{{end}}</div>
</div>
{{end}}

{{define "aprs_sent_detail"}}
{{template "layout_start" .}}
{{$m := .Data.Message}}{{$ack := ackBadge $m}}
{{$date := aprsDateTime $m.At}}
<div class="pagehead"><div><p class="kicker">APRS</p><h1>{{tr .Lang "aprs_sent_message_detail"}}</h1></div><div class="pageactions"><a class="btn secondary" href="/aprs/sent">{{tr .Lang "web_back_to_aprs"}}</a><form method="post" action="/aprs/sent/{{$m.ID}}/delete" onsubmit="return confirm({{printf "%q" (tr .Lang "confirm_delete_aprs_message")}})"><button class="btn danger" type="submit">{{tr .Lang "delete_button"}}</button></form></div></div>
<section class="card"><dl class="detail-grid"><dt>{{tr .Lang "from"}}</dt><dd>{{$m.From}}</dd><dt>{{tr .Lang "aprs_destination_callsign"}}</dt><dd>{{$m.To}}</dd><dt>{{tr .Lang "at"}}</dt><dd class="aprs-time">{{$date.Date}} · {{$date.Time}}</dd><dt>{{tr .Lang "status"}}</dt><dd>{{aprsStatus .Lang $m.Status}}</dd><dt>{{tr .Lang "web_ack_status"}}</dt><dd><span class="badge {{$ack.Class}}">{{$ack.Icon}}</span> {{tr .Lang $ack.LabelKey}}</dd><dt>{{tr .Lang "aprs_acknowledged"}}</dt><dd>{{$m.Acked}}</dd><dt>{{tr .Lang "aprs_parts"}}</dt><dd>{{len $m.Parts}}</dd><dt>{{tr .Lang "aprs_text"}}</dt><dd class="pre">{{$m.Text}}</dd></dl></section>
<div class="strip mt-lg"><h2>{{tr .Lang "aprs_part_statuses"}}</h2></div>
<div class="scroll mt"><table><thead><tr><th>#</th><th>{{tr .Lang "status"}}</th><th>{{tr .Lang "aprs_acknowledged"}}</th><th>{{tr .Lang "aprs_text"}}</th><th>ID</th><th>{{tr .Lang "web_detail"}}</th></tr></thead><tbody>{{range $m.Parts}}<tr><td>{{.Number}}</td><td>{{aprsStatus $.Lang .Status}}</td><td>{{.Acked}}</td><td class="pre">{{.Text}}</td><td>{{.MessageID}}</td><td>{{.Detail}}</td></tr>{{else}}<tr><td colspan="6" class="muted">{{tr $.Lang "web_no_parts"}}</td></tr>{{end}}</tbody></table></div>
{{template "layout_end" .}}
{{end}}

{{define "aprs_received_detail"}}
{{template "layout_start" .}}
{{$m := .Data.Message}}{{$d := .Data.Detail}}
{{$date := aprsDateTime $m.At}}
<div class="pagehead"><div><p class="kicker">APRS</p><h1>{{tr .Lang "aprs_received_message_detail"}}</h1></div><div class="pageactions"><a class="btn secondary" href="/aprs/received">{{tr .Lang "web_back_to_aprs"}}</a><a class="btn" href="/aprs/received/{{$m.ID}}/reply">{{tr .Lang "reply_button"}}</a><form method="post" action="/aprs/received/{{$m.ID}}/delete" onsubmit="return confirm({{printf "%q" (tr .Lang "confirm_delete_aprs_message")}})"><button class="btn danger" type="submit">{{tr .Lang "delete_button"}}</button></form></div></div>
<section class="card"><dl class="detail-grid"><dt>{{tr .Lang "from"}}</dt><dd>{{$m.From}}</dd><dt>{{tr .Lang "aprs_destination_callsign"}}</dt><dd>{{$m.To}}</dd><dt>{{tr .Lang "at"}}</dt><dd class="aprs-time">{{$date.Date}} · {{$date.Time}}</dd><dt>{{tr .Lang "aprs_text"}}</dt><dd class="pre">{{$d.Text}}</dd></dl>{{if $d.Raw}}<hr><dl class="detail-grid"><dt>{{tr .Lang "aprs_raw_packet"}}</dt><dd class="pre">{{$d.Raw}}</dd></dl>{{end}}</section>
{{template "layout_end" .}}
{{end}}

{{define "aprs_reply"}}
{{template "layout_start" .}}
{{$m := .Data.Message}}{{$d := .Data.Detail}}
{{$date := aprsDateTime $m.At}}
<div class="pagehead"><div><p class="kicker">APRS</p><h1>{{tr .Lang "web_reply"}}</h1></div><div class="pageactions"><a class="btn secondary" href="/aprs/received/{{$m.ID}}">{{tr .Lang "web_back_to_aprs"}}</a></div></div>
<section class="card mb"><div class="metaline">{{tr .Lang "from"}} {{$m.From}} · {{tr .Lang "at"}} {{$date.Date}} {{$date.Time}}</div><p class="pre mt-sm">{{$d.Text}}</p></section>
<form class="stack" method="post" action="/aprs/received/{{$m.ID}}/reply">
  <div class="cols"><div><label>{{tr .Lang "aprs_destination_callsign"}}</label><input name="destination" value="{{.Data.Destination}}" required></div><div><label>{{tr .Lang "aprs_destination_ssid"}}</label><input name="destination_ssid" maxlength="2" value="{{.Data.DestinationSSID}}"></div></div>
  <div><label>{{tr .Lang "aprs_text"}}</label><textarea name="text"></textarea></div>
  <div class="actions"><button class="btn" type="submit">{{tr .Lang "send_button"}}</button><a class="btn secondary" href="/aprs/received/{{$m.ID}}">{{tr .Lang "cancel_button"}}</a></div>
</form>
{{template "layout_end" .}}
{{end}}

{{define "admin_users"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{tr .Lang "sysop"}}</p><h1>{{tr .Lang "web_admin_users"}}</h1></div></div>
<div class="scroll"><table class="tbl-users"><thead><tr><th>{{tr .Lang "target_callsign"}}</th><th>{{tr .Lang "full_name"}}</th><th>{{tr .Lang "email"}}</th><th>{{tr .Lang "date_registered"}}</th><th>{{tr .Lang "status"}}</th><th>{{tr .Lang "role"}}</th><th>{{tr .Lang "web_actions"}}</th></tr></thead><tbody>{{range .Data}}<tr><td><strong>{{.Callsign}}</strong></td><td>{{.FullName}}</td><td>{{.Email}}</td><td>{{if .Registered}}{{.Registered}}{{else}}-{{end}}</td><td>{{if .Disabled}}{{tr $.Lang "disabled"}}{{else}}{{tr $.Lang "enabled"}}{{end}}</td><td>{{if .IsSysop}}{{tr $.Lang "sysop_role"}}{{else}}{{tr $.Lang "user_role"}}{{end}}</td><td><form class="row" method="post" action="/admin/users/{{.Callsign}}"><select name="disabled"><option value="false">{{tr $.Lang "enabled"}}</option><option value="true" {{if .Disabled}}selected{{end}}>{{tr $.Lang "disabled"}}</option></select><select name="is_sysop"><option value="false">{{tr $.Lang "user_role"}}</option><option value="true" {{if .IsSysop}}selected{{end}}>{{tr $.Lang "sysop_role"}}</option></select><button class="btn secondary small">{{tr $.Lang "save_button"}}</button></form></td></tr>{{end}}</tbody></table></div>
{{template "layout_end" .}}{{end}}

{{define "activate"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{tr .Lang "activation_title"}}</p><h1>{{tr .Lang "activation_title"}}</h1><p class="lede">{{tr .Lang "web_activation_help"}}</p></div></div>
<form class="stack" method="post" action="/activate"><div><label>{{tr .Lang "target_callsign"}}</label><input name="callsign" value="{{index .Data "Callsign"}}" autocomplete="username" required></div><div><label>{{tr .Lang "activation_code"}}</label><input name="code" inputmode="latin" autocapitalize="characters" autocomplete="one-time-code" required autofocus></div><div class="row"><button class="btn" name="action" value="activate">{{tr .Lang "activate_button"}}</button><button class="btn secondary" name="action" value="resend" formnovalidate>{{tr .Lang "resend_button"}}</button><a class="btn secondary" href="/login">{{tr .Lang "cancel_button"}}</a></div></form>
{{template "layout_end" .}}{{end}}

{{define "forgot"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{tr .Lang "web_login"}}</p><h1>{{tr .Lang "forgot_password_title"}}</h1></div></div>
<form class="stack" method="post" action="/forgot"><div><label>{{tr .Lang "forgot_password_prompt"}}</label><input name="identifier" required autofocus></div><div class="row"><button class="btn">{{tr .Lang "send_button"}}</button><a class="btn secondary" href="/login">{{tr .Lang "cancel_button"}}</a></div></form>
{{template "layout_end" .}}{{end}}

{{define "reset"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{tr .Lang "web_login"}}</p><h1>{{tr .Lang "reset_password_title"}}</h1></div></div>
{{if index .Data "Token"}}<form class="stack" method="post" action="/reset"><input type="hidden" name="token" value="{{index .Data "Token"}}"><p class="muted">{{tr .Lang "target_callsign"}}: <strong class="accent">{{index .Data "Callsign"}}</strong></p><div class="cols"><div><label>{{tr .Lang "new_password"}}</label><input name="new_password" type="password" autocomplete="new-password" minlength="8" required autofocus></div><div><label>{{tr .Lang "verify_password"}}</label><input name="verify_password" type="password" autocomplete="new-password" minlength="8" required></div></div><div class="row"><button class="btn">{{tr .Lang "reset_button"}}</button></div></form>{{end}}
{{template "layout_end" .}}{{end}}

{{define "email_confirm"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{tr .Lang "web_profile"}}</p><h1>{{tr .Lang "email_change_pending"}}</h1></div></div>
{{if index .Data "Done"}}<div class="card narrow">{{if index .Data "Applied"}}<p>{{tr .Lang "web_email_change_applied"}}</p>{{else}}<p>{{tr .Lang "web_email_change_half"}}</p>{{end}}<a class="btn secondary" href="/login">{{tr .Lang "web_login"}}</a></div>{{end}}
{{template "layout_end" .}}{{end}}

{{define "message_form"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{.Data.Board.Name}}</p><h1>{{tr .Lang "message_edit_title"}}</h1></div></div>
<form class="stack" method="post" action="/messages/{{.Data.Message.ID}}/edit"><div><label>{{tr .Lang "subject"}}</label><input name="subject" value="{{.Data.Message.Subject}}" required></div><div><label>{{tr .Lang "message_body"}}</label><textarea name="body" required>{{.Data.Message.Body}}</textarea></div><div class="row"><button class="btn">{{tr .Lang "save_button"}}</button><a class="btn secondary" href="/boards/{{.Data.Board.ID}}">{{tr .Lang "cancel_button"}}</a></div></form>
{{template "layout_end" .}}{{end}}

{{define "not_found"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">404</p><h1>{{tr .Lang "web_not_found"}}</h1><p class="lede">{{tr .Lang "web_not_found_body"}}</p></div></div>
<div class="row"><a class="btn" href="/">{{tr .Lang "web_home"}}</a><a class="btn secondary" href="/boards">{{tr .Lang "web_message_boards"}}</a></div>
{{template "layout_end" .}}{{end}}

{{define "about"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{.AppName}}</p><h1>{{tr .Lang "about_title"}}</h1></div></div>
<div class="card wide prose"><p>{{tr .Lang "about_hamnet"}}</p><p>{{tr .Lang "about_storage"}}</p><p>{{tr .Lang "about_graphics"}}</p><p>{{tr .Lang "about_note"}}</p></div>
{{template "layout_end" .}}{{end}}

{{define "resources"}}{{template "layout_start" .}}
<div class="pagehead"><div><p class="kicker">{{.Location}}</p><h1>{{tr .Lang "resources_title"}}</h1></div></div>
<div class="card wide"><ul class="plain"><li>{{tr .Lang "hamnet_address"}}</li><li>{{tr .Lang "local_ssh"}}</li><li>{{tr .Lang "hamnet_ssh"}}</li></ul></div>
{{template "layout_end" .}}{{end}}
`
