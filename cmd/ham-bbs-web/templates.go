package main

import (
	"fmt"
	"html/template"
)

func parseTemplates(text map[string]map[string]any) *template.Template {
	funcs := template.FuncMap{
		"tr":              func(lang, key string) string { return translation(text, lang, key) },
		"langName":        func(code string) string { return languages[code] },
		"languageChoices": languageChoices,
		"aprsListText":    aprsListText,
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
		"selected": func(a, b string) string {
			if a == b {
				return "selected"
			}
			return ""
		},
		"dict": func(values ...any) map[string]any {
			m := map[string]any{}
			for i := 0; i+1 < len(values); i += 2 {
				m[fmt.Sprint(values[i])] = values[i+1]
			}
			return m
		},
		"inc": func(v int) int { return v + 1 },
	}
	return template.Must(template.New("app").Funcs(funcs).Parse(templates))
}

const templates = `
{{define "layout_start"}}
<!doctype html>
<html lang="{{.Lang}}">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.AppName}}</title>
  <style>
    :root { color-scheme: light; --ink:#14221f; --muted:#68756f; --line:#cbd7d1; --paper:#f7faf8; --panel:#ffffff; --accent:#146c5c; --accent2:#b1382e; --soft:#e8f2ee; }
    * { box-sizing: border-box; }
    body { margin:0; font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background:var(--paper); color:var(--ink); }
    header { border-bottom:1px solid var(--line); background:#fff; }
    .top { max-width:1180px; margin:0 auto; padding:16px 20px; display:flex; gap:16px; align-items:center; justify-content:space-between; }
    .brand { font-weight:800; font-size:20px; line-height:1.1; }
    .sub { color:var(--muted); font-size:13px; margin-top:3px; }
    nav { display:flex; gap:8px; flex-wrap:wrap; align-items:center; }
    nav a, button.link { color:var(--ink); text-decoration:none; border:1px solid var(--line); background:#fff; padding:8px 10px; border-radius:6px; font:inherit; cursor:pointer; }
    nav a:hover, button.link:hover { border-color:var(--accent); color:var(--accent); }
    .language-switcher { display:flex; align-items:center; gap:6px; margin:0; }
    .language-switcher label { margin:0; white-space:nowrap; }
    .language-switcher select { width:auto; min-width:118px; padding:8px 28px 8px 10px; }
    main { max-width:1180px; margin:0 auto; padding:22px 20px 48px; }
    h1 { font-size:28px; margin:0 0 18px; }
    h2 { font-size:18px; margin:24px 0 10px; }
    h3 { font-size:16px; margin:0 0 8px; }
    .grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(220px,1fr)); gap:12px; }
    .card { background:var(--panel); border:1px solid var(--line); border-radius:8px; padding:16px; }
    .row { display:flex; gap:10px; flex-wrap:wrap; align-items:center; }
    .between { display:flex; justify-content:space-between; gap:14px; align-items:start; }
    .muted { color:var(--muted); }
    .pre { white-space:pre-wrap; line-height:1.45; }
    .flash { background:var(--soft); border:1px solid #b7d4c8; padding:10px 12px; border-radius:6px; margin-bottom:16px; }
    .error { background:#fae8e6; border:1px solid #e6b3ae; color:#7d2018; padding:10px 12px; border-radius:6px; margin-bottom:16px; }
    label { display:block; font-size:13px; color:var(--muted); margin-bottom:5px; }
    input, textarea, select { width:100%; border:1px solid var(--line); border-radius:6px; padding:10px; font:inherit; background:white; }
    textarea { min-height:130px; resize:vertical; }
    form.stack { display:grid; gap:13px; max-width:760px; }
    .cols { display:grid; grid-template-columns:repeat(auto-fit,minmax(220px,1fr)); gap:12px; }
    .btn { display:inline-flex; align-items:center; justify-content:center; gap:8px; border:1px solid var(--accent); background:var(--accent); color:white; border-radius:6px; padding:9px 12px; text-decoration:none; font:inherit; cursor:pointer; }
    .btn.small { padding:6px 9px; font-size:13px; }
    .btn.secondary { background:white; color:var(--accent); }
    .btn.danger { border-color:var(--accent2); background:var(--accent2); }
    .badge { display:inline-flex; min-width:1.6rem; justify-content:center; font-weight:800; }
    .ack-ok { color:#16803f; } .ack-rejected { color:#ba2d23; } .ack-partial { color:#c26a00; }
    .detail-grid { display:grid; grid-template-columns:minmax(140px, 220px) 1fr; gap:8px 14px; }
    .detail-grid dt { color:var(--muted); } .detail-grid dd { margin:0; }
    .actions { display:flex; gap:8px; flex-wrap:wrap; align-items:center; }
    .actions form { margin:0; }
    .bulk-bar { display:flex; justify-content:flex-end; margin-top:10px; }
    label.checkbox-label { display:inline-flex; align-items:center; gap:6px; margin:0; color:inherit; font-size:inherit; }
    input[type=checkbox] { width:auto; padding:0; }
    .aprs-table { table-layout:fixed; }
    .aprs-table th, .aprs-table td { white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
    .aprs-table a.rowlink { display:block; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
    .aprs-table .selection-col { width:8%; }
    .aprs-table .time-col { width:14%; }
    .aprs-table .callsign-col { width:14%; }
    .aprs-table .destination-col { width:15%; }
    .aprs-table .message-col { width:34%; }
    .aprs-table.sent-table .message-col { width:29%; }
    .aprs-table .actions-col { width:15%; }
    .aprs-table .ack-col { width:8%; }
    .aprs-table .status-col { width:11%; }
    .aprs-table .actions { flex-wrap:nowrap; }
    a.rowlink { color:var(--ink); text-decoration:none; font-weight:650; }
    a.rowlink:hover { color:var(--accent); text-decoration:underline; }
    table { width:100%; border-collapse:collapse; background:white; border:1px solid var(--line); border-radius:8px; overflow:hidden; }
    th, td { text-align:left; padding:10px; border-bottom:1px solid var(--line); vertical-align:top; }
    th { font-size:13px; color:var(--muted); background:#fbfdfc; }
    .message { border-left:3px solid var(--accent); margin:12px 0; padding:12px 12px 12px 14px; background:white; border-radius:0 8px 8px 0; }
    .indent-1 { margin-left:24px; } .indent-2 { margin-left:48px; } .indent-3 { margin-left:72px; } .indent-4 { margin-left:96px; }
    @media (max-width: 740px) { .top { align-items:flex-start; flex-direction:column; } .language-switcher { align-self:flex-end; } nav a, button.link { padding:7px 8px; } table { display:block; overflow-x:auto; } .aprs-table { min-width:900px; } .indent-1,.indent-2,.indent-3,.indent-4 { margin-left:12px; } }
  </style>
</head>
<body>
<header><div class="top"><div><div class="brand">{{.AppName}}</div><div class="sub">{{.Location}} - {{.Topic}}</div></div>{{if .User}}<nav><a href="/">{{tr .Lang "web_home"}}</a><a href="/bulletins">{{tr .Lang "menu_bulletins"}}</a><a href="/boards">{{tr .Lang "web_message_boards"}}</a><a href="/directory">{{tr .Lang "menu_directory"}}</a><a href="/aprs">APRS</a><a href="/profile">{{.User.Callsign}}</a>{{if .IsSysop}}<a href="/admin/users">{{tr .Lang "sysop"}}</a>{{end}}<form method="post" action="/logout"><button class="link">{{tr .Lang "web_logout"}}</button></form></nav><form class="language-switcher" method="post" action="/language"><label for="header-language">{{tr .Lang "language"}}</label><select id="header-language" name="language" onchange="this.form.submit()">{{range languageChoices}}<option value="{{.Code}}" {{selected $.Lang .Code}}>{{.Name}}</option>{{end}}</select><input type="hidden" name="return_to" value="{{.Path}}"></form>{{end}}</div></header>
<main>
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
{{define "layout_end"}}</main></body></html>{{end}}

{{define "login"}}{{template "layout_start" .}}<h1>{{tr .Lang "web_login"}}</h1><form class="stack" method="post" action="/login"><div><label>{{tr .Lang "callsign_prompt"}}</label><input name="callsign" autocomplete="username" required autofocus></div><div><label>{{tr .Lang "web_bbs_password"}}</label><input name="password" type="password" autocomplete="current-password" required></div><div class="row"><button class="btn">{{tr .Lang "web_login"}}</button><a class="btn secondary" href="/register">{{tr .Lang "web_register"}}</a></div></form>{{template "layout_end" .}}{{end}}

{{define "register"}}{{template "layout_start" .}}<h1>{{tr .Lang "web_register"}}</h1><form class="stack" method="post" action="/register">{{template "user_fields" dict "User" .Data.User "Languages" .Data.Languages "Callsign" .Data.Callsign "RequireCallsign" true "RequirePassword" true "Lang" .Lang}}<button class="btn">{{tr .Lang "web_create_account"}}</button></form>{{template "layout_end" .}}{{end}}

{{define "user_fields"}}{{if .RequireCallsign}}<div><label>{{tr .Lang "callsign_prompt"}}</label><input name="callsign" value="{{.Callsign}}" required></div>{{end}}<div class="cols"><div><label>{{tr .Lang "full_name"}}</label><input name="full_name" value="{{.User.FullName}}" required></div><div><label>{{tr .Lang "email"}}</label><input name="email" type="email" value="{{.User.Email}}" required></div></div><div class="cols"><div><label>{{tr .Lang "maidenhead"}}</label><input name="maidenhead" value="{{.User.Maidenhead}}"></div><div><label>{{tr .Lang "language"}}</label><select name="language">{{range .Languages}}<option value="{{.Code}}" {{selected $.User.Language .Code}}>{{.Name}}</option>{{end}}</select></div></div><div class="cols"><div><label>{{tr .Lang "qth"}}</label><input name="qth" value="{{.User.QTH}}"></div><div><label>{{tr .Lang "rig"}}</label><input name="rig" value="{{.User.Rig}}"></div></div><div><label>{{tr .Lang "enable_aprs"}}</label><select name="enable_aprs"><option value="false">false</option><option value="true" {{if .User.EnableAPRS}}selected{{end}}>true</option></select></div><div class="cols"><div><label>{{if .RequirePassword}}{{tr .Lang "password"}}{{else}}{{tr .Lang "new_password"}}{{end}}</label><input name="new_password" type="password" {{if .RequirePassword}}required{{end}}></div><div><label>{{tr .Lang "verify_password"}}</label><input name="verify_password" type="password" {{if .RequirePassword}}required{{end}}></div></div>{{end}}

{{define "home"}}{{template "layout_start" .}}<h1>{{tr .Lang "web_dashboard"}}</h1><div class="grid"><a class="card" href="/bulletins"><h3>{{tr .Lang "menu_bulletins"}}</h3><div class="brand">{{.Data.Bulletins}}</div></a><a class="card" href="/boards"><h3>{{tr .Lang "web_message_boards"}}</h3><div class="brand">{{.Data.Boards}}</div></a><a class="card" href="/directory"><h3>{{tr .Lang "menu_directory"}}</h3><div class="brand">{{.Data.Users}}</div></a><a class="card" href="/aprs"><h3>{{tr .Lang "aprs_received_messages"}}</h3><div class="brand">{{.Data.Received}}</div></a></div>{{template "layout_end" .}}{{end}}

{{define "profile"}}{{template "layout_start" .}}<h1>{{tr .Lang "web_profile"}}</h1><form class="stack" method="post" action="/profile">{{template "user_fields" dict "User" .User "Languages" .Data.Languages "RequireCallsign" false "RequirePassword" false "Lang" .Lang}}<button class="btn">{{tr .Lang "web_save_profile"}}</button></form>{{template "layout_end" .}}{{end}}

{{define "directory"}}{{template "layout_start" .}}<h1>{{tr .Lang "web_station_directory"}}</h1><table><tr><th>{{tr .Lang "target_callsign"}}</th><th>{{tr .Lang "full_name"}}</th><th>{{tr .Lang "maidenhead"}}</th><th>{{tr .Lang "qth"}}</th><th>{{tr .Lang "rig"}}</th><th>{{tr .Lang "last_connection"}}</th><th>APRS</th></tr>{{range .Data}}<tr><td><strong>{{.Callsign}}</strong></td><td>{{.FullName}}</td><td>{{.Maidenhead}}</td><td>{{.QTH}}</td><td>{{.Rig}}</td><td>{{.LastSeen}}</td><td>{{.EnableAPRS}}</td></tr>{{end}}</table>{{template "layout_end" .}}{{end}}

{{define "bulletins"}}{{template "layout_start" .}}<div class="between"><h1>{{tr .Lang "menu_bulletins"}}</h1>{{if .IsSysop}}<a class="btn" href="/bulletins/new">{{tr .Lang "web_new_bulletin"}}</a>{{end}}</div>{{range .Data}}<article class="card"><div class="between"><div><h2>{{.Title}}</h2><div class="muted">{{.Updated}}{{if .From}} {{tr $.Lang "from"}} {{.From}}{{end}}</div></div>{{if $.IsSysop}}<a class="btn secondary" href="/bulletins/{{.ID}}/edit">{{tr $.Lang "web_edit"}}</a>{{end}}</div><p class="pre">{{.Body}}</p></article>{{else}}<p>{{tr .Lang "no_bulletins"}}</p>{{end}}{{template "layout_end" .}}{{end}}

{{define "bulletin_form"}}{{template "layout_start" .}}<h1>{{if .Data.ID}}{{tr .Lang "bulletin_edit_title"}}{{else}}{{tr .Lang "bulletin_form_title"}}{{end}}</h1><form class="stack" method="post"><div><label>{{tr .Lang "bulletin_title"}}</label><input name="title" value="{{.Data.Title}}" required></div><div><label>{{tr .Lang "bulletin_body"}}</label><textarea name="body" required>{{.Data.Body}}</textarea></div><div class="row"><button class="btn">{{tr .Lang "save_button"}}</button><a class="btn secondary" href="/bulletins">{{tr .Lang "cancel_button"}}</a></div></form>{{if .Data.ID}}<form method="post" action="/bulletins/{{.Data.ID}}/delete" style="margin-top:18px"><button class="btn danger">{{tr .Lang "delete_button"}}</button></form>{{end}}{{template "layout_end" .}}{{end}}

{{define "boards"}}{{template "layout_start" .}}<div class="between"><h1>{{tr .Lang "web_message_boards"}}</h1>{{if .IsSysop}}<a class="btn" href="/boards/new">{{tr .Lang "web_new_board"}}</a>{{end}}</div><div class="grid">{{range .Data.Boards}}<a class="card" href="/boards/{{.ID}}"><h3>{{.Name}}</h3><p>{{.Description}}</p><div class="muted">{{index $.Data.Counts .ID}} {{tr $.Lang "menu_messages"}}</div></a>{{else}}<p>{{tr .Lang "no_boards"}}</p>{{end}}</div>{{template "layout_end" .}}{{end}}

{{define "board"}}{{template "layout_start" .}}<div class="between"><div><h1>{{.Data.Board.Name}}</h1><p class="muted">{{.Data.Board.Description}}</p></div>{{if .IsSysop}}<a class="btn secondary" href="/boards/{{.Data.Board.ID}}/edit">{{tr .Lang "web_edit_board"}}</a>{{end}}</div><form class="stack card" method="post" action="/boards/{{.Data.Board.ID}}/post"><h3>{{tr .Lang "web_post_message"}}</h3><div><label>{{tr .Lang "subject"}}</label><input name="subject" required></div><div><label>{{tr .Lang "message_body"}}</label><textarea name="body" required></textarea></div><button class="btn">{{tr .Lang "web_post"}}</button></form><h2>{{tr .Lang "menu_messages"}}</h2>{{range .Data.Messages}}{{template "message" dict "Node" . "Root" $}}{{else}}<p>{{tr .Lang "web_no_messages_yet"}}</p>{{end}}{{template "layout_end" .}}{{end}}

{{define "message"}}<section class="message indent-{{.Node.Depth}}"><div class="between"><div><strong>{{.Node.Row.Subject}}</strong><div class="muted">{{tr .Root.Lang "from"}} {{.Node.Row.From}} {{tr .Root.Lang "at"}} {{.Node.Row.Created}}{{if .Node.Row.Edited}} - {{tr .Root.Lang "web_edit"}} {{.Node.Row.Edited}}{{end}}</div></div>{{if .Root.IsSysop}}<form method="post" action="/messages/{{.Node.Row.ID}}/delete"><button class="btn danger">{{tr .Root.Lang "delete_button"}}</button></form>{{end}}</div><p class="pre">{{.Node.Row.Body}}</p><details><summary>{{tr .Root.Lang "reply_button"}}</summary><form class="stack" method="post" action="/messages/{{.Node.Row.ID}}/reply"><div><label>{{tr .Root.Lang "subject"}}</label><input name="subject" value="Re: {{.Node.Row.Subject}}" required></div><div><label>{{tr .Root.Lang "message_body"}}</label><textarea name="body" required></textarea></div><button class="btn">{{tr .Root.Lang "reply_button"}}</button></form></details>{{range .Node.Replies}}{{template "message" dict "Node" . "Root" $.Root}}{{end}}</section>{{end}}

{{define "board_form"}}{{template "layout_start" .}}<h1>{{if .Data.ID}}{{tr .Lang "board_rename_title"}}{{else}}{{tr .Lang "board_form_title"}}{{end}}</h1><form class="stack" method="post"><div><label>{{tr .Lang "board_name"}}</label><input name="name" value="{{.Data.Name}}" required></div><div><label>{{tr .Lang "board_description"}}</label><input name="description" value="{{.Data.Description}}"></div><div class="row"><button class="btn">{{tr .Lang "save_button"}}</button><a class="btn secondary" href="/boards">{{tr .Lang "cancel_button"}}</a></div></form>{{if .Data.ID}}<form method="post" action="/boards/{{.Data.ID}}/delete" style="margin-top:18px"><button class="btn danger">{{tr .Lang "delete_button"}}</button></form>{{end}}{{template "layout_end" .}}{{end}}

{{define "aprs"}}
{{template "layout_start" .}}
<h1>{{tr .Lang "menu_aprs"}}</h1>

<h2>{{tr .Lang "aprs_received_messages"}}</h2>
<form method="post" action="/aprs/received/delete">
<table class="aprs-table received-table">
  <tr><th class="selection-col"><label class="checkbox-label" for="aprs-select-all-received"><input type="checkbox" id="aprs-select-all-received" onchange="toggleAPRSSelection('received', this.checked)"><span>{{tr .Lang "select_all"}}</span></label></th><th class="time-col">{{tr .Lang "at"}}</th><th class="callsign-col">{{tr .Lang "from"}}</th><th class="destination-col">{{tr .Lang "aprs_destination_callsign"}}</th><th class="message-col">{{tr .Lang "aprs_text"}}</th><th class="actions-col">{{tr .Lang "web_actions"}}</th></tr>
  {{range .Data.Received}}
  <tr>
    <td><input type="checkbox" name="received_ids" value="{{.ID}}" data-aprs-group="received" onchange="syncAPRSSelection('received')" aria-label="{{tr $.Lang "aprs_received_message_detail"}}"></td>
    <td><a class="rowlink" href="/aprs/received/{{.ID}}">{{.At}}</a></td>
    <td><a class="rowlink" href="/aprs/received/{{.ID}}">{{.From}}</a></td>
    <td><a class="rowlink" href="/aprs/received/{{.ID}}">{{.To}}</a></td>
    <td><a class="rowlink" href="/aprs/received/{{.ID}}">{{aprsListText .Text}}</a></td>
    <td><div class="actions"><a class="btn secondary small" href="/aprs/received/{{.ID}}">{{tr $.Lang "web_detail"}}</a><button class="btn danger small" type="submit" formaction="/aprs/received/{{.ID}}/delete" onclick="return confirm({{printf "%q" (tr $.Lang "confirm_delete_aprs_message")}})">{{tr $.Lang "delete_button"}}</button></div></td>
  </tr>
  {{else}}
  <tr><td colspan="6">{{tr $.Lang "web_no_aprs_received"}}</td></tr>
  {{end}}
</table>
<div class="bulk-bar"><button class="btn danger small" type="submit" data-aprs-delete-selected="received" onclick="return confirm({{printf "%q" (tr .Lang "confirm_delete_aprs_selected")}})" disabled>{{tr .Lang "delete_selected"}}</button></div>
</form>

<h2>{{tr .Lang "aprs_sent_messages"}}</h2>
<form method="post" action="/aprs/sent/delete">
<table class="aprs-table sent-table">
  <tr><th class="selection-col"><label class="checkbox-label" for="aprs-select-all-sent"><input type="checkbox" id="aprs-select-all-sent" onchange="toggleAPRSSelection('sent', this.checked)"><span>{{tr .Lang "select_all"}}</span></label></th><th class="ack-col">{{tr .Lang "web_ack_status"}}</th><th class="time-col">{{tr .Lang "at"}}</th><th class="destination-col">{{tr .Lang "aprs_destination_callsign"}}</th><th class="status-col">{{tr .Lang "status"}}</th><th class="message-col">{{tr .Lang "aprs_text"}}</th><th class="actions-col">{{tr .Lang "web_actions"}}</th></tr>
  {{range .Data.Sent}}
  {{$ack := ackBadge .}}
  <tr>
    <td><input type="checkbox" name="sent_ids" value="{{.ID}}" data-aprs-group="sent" onchange="syncAPRSSelection('sent')" aria-label="{{tr $.Lang "aprs_sent_message_detail"}}"></td>
    <td><a class="rowlink" href="/aprs/sent/{{.ID}}"><span class="badge {{$ack.Class}}" title="{{tr $.Lang $ack.LabelKey}}">{{$ack.Icon}}</span></a></td>
    <td><a class="rowlink" href="/aprs/sent/{{.ID}}">{{.At}}</a></td>
    <td><a class="rowlink" href="/aprs/sent/{{.ID}}">{{.To}}</a></td>
    <td><a class="rowlink" href="/aprs/sent/{{.ID}}">{{aprsStatus $.Lang .Status}}</a></td>
    <td><a class="rowlink" href="/aprs/sent/{{.ID}}">{{aprsListText .Text}}</a></td>
    <td><div class="actions"><a class="btn secondary small" href="/aprs/sent/{{.ID}}">{{tr $.Lang "web_detail"}}</a><button class="btn danger small" type="submit" formaction="/aprs/sent/{{.ID}}/delete" onclick="return confirm({{printf "%q" (tr $.Lang "confirm_delete_aprs_message")}})">{{tr $.Lang "delete_button"}}</button></div></td>
  </tr>
  {{else}}
  <tr><td colspan="7">{{tr $.Lang "web_no_aprs_sent"}}</td></tr>
  {{end}}
</table>
<div class="bulk-bar"><button class="btn danger small" type="submit" data-aprs-delete-selected="sent" onclick="return confirm({{printf "%q" (tr .Lang "confirm_delete_aprs_selected")}})" disabled>{{tr .Lang "delete_selected"}}</button></div>
</form>

<h2>{{tr .Lang "aprs_send_message"}}</h2>
<form class="stack card" method="post" action="/aprs/send">
  <div class="cols"><div><label>{{tr .Lang "aprs_destination_callsign"}}</label><input name="destination" required></div><div><label>{{tr .Lang "aprs_destination_ssid"}}</label><input name="destination_ssid" maxlength="2" value="0"></div></div>
  <div><label>{{tr .Lang "aprs_text"}}</label><textarea name="text"></textarea></div>
  <button class="btn">{{tr .Lang "send_button"}}</button>
</form>

<h2>{{tr .Lang "aprs_enable_title"}}</h2>
<section class="card">
  <form method="post" action="/aprs/toggle" class="row"><label style="margin:0">{{tr .Lang "enable_aprs"}}: {{.User.Callsign}}</label><select name="enable_aprs" style="max-width:140px"><option value="false">false</option><option value="true" {{if .User.EnableAPRS}}selected{{end}}>true</option></select><button class="btn">{{tr .Lang "save_button"}}</button></form>
  <p class="muted">{{tr .Lang "web_aprs_worker_note"}}</p>
</section>
{{template "layout_end" .}}
{{end}}

{{define "aprs_sent_detail"}}
{{template "layout_start" .}}
{{$m := .Data.Message}}{{$ack := ackBadge $m}}
<div class="between"><h1>{{tr .Lang "aprs_sent_message_detail"}}</h1><div class="actions"><a class="btn secondary" href="/aprs">{{tr .Lang "web_back_to_aprs"}}</a><form method="post" action="/aprs/sent/{{$m.ID}}/delete" onsubmit="return confirm({{printf "%q" (tr .Lang "confirm_delete_aprs_message")}})"><button class="btn danger" type="submit">{{tr .Lang "delete_button"}}</button></form></div></div>
<section class="card"><dl class="detail-grid"><dt>{{tr .Lang "from"}}</dt><dd>{{$m.From}}</dd><dt>{{tr .Lang "aprs_destination_callsign"}}</dt><dd>{{$m.To}}</dd><dt>{{tr .Lang "at"}}</dt><dd>{{$m.At}}</dd><dt>{{tr .Lang "status"}}</dt><dd>{{aprsStatus .Lang $m.Status}}</dd><dt>{{tr .Lang "web_ack_status"}}</dt><dd><span class="badge {{$ack.Class}}">{{$ack.Icon}}</span> {{tr .Lang $ack.LabelKey}}</dd><dt>{{tr .Lang "aprs_acknowledged"}}</dt><dd>{{$m.Acked}}</dd><dt>{{tr .Lang "aprs_parts"}}</dt><dd>{{len $m.Parts}}</dd><dt>{{tr .Lang "aprs_text"}}</dt><dd>{{$m.Text}}</dd></dl></section>
<h2>{{tr .Lang "aprs_part_statuses"}}</h2>
<table><tr><th>#</th><th>{{tr .Lang "status"}}</th><th>{{tr .Lang "aprs_acknowledged"}}</th><th>{{tr .Lang "aprs_text"}}</th><th>ID</th><th>{{tr .Lang "web_detail"}}</th></tr>{{range $m.Parts}}<tr><td>{{.Number}}</td><td>{{aprsStatus $.Lang .Status}}</td><td>{{.Acked}}</td><td>{{.Text}}</td><td>{{.MessageID}}</td><td>{{.Detail}}</td></tr>{{else}}<tr><td colspan="6">{{tr $.Lang "web_no_parts"}}</td></tr>{{end}}</table>
{{template "layout_end" .}}
{{end}}

{{define "aprs_received_detail"}}
{{template "layout_start" .}}
{{$m := .Data.Message}}{{$d := .Data.Detail}}
<div class="between"><h1>{{tr .Lang "aprs_received_message_detail"}}</h1><div class="actions"><a class="btn secondary" href="/aprs">{{tr .Lang "web_back_to_aprs"}}</a><a class="btn" href="/aprs/received/{{$m.ID}}/reply">{{tr .Lang "reply_button"}}</a><form method="post" action="/aprs/received/{{$m.ID}}/delete" onsubmit="return confirm({{printf "%q" (tr .Lang "confirm_delete_aprs_message")}})"><button class="btn danger" type="submit">{{tr .Lang "delete_button"}}</button></form></div></div>
<section class="card"><dl class="detail-grid"><dt>{{tr .Lang "from"}}</dt><dd>{{$m.From}}</dd><dt>{{tr .Lang "aprs_destination_callsign"}}</dt><dd>{{$m.To}}</dd><dt>{{tr .Lang "at"}}</dt><dd>{{$m.At}}</dd><dt>{{tr .Lang "aprs_text"}}</dt><dd>{{$d.Text}}</dd></dl>{{if $d.Raw}}<hr><dl class="detail-grid"><dt>{{tr .Lang "aprs_raw_packet"}}</dt><dd class="pre">{{$d.Raw}}</dd></dl>{{end}}</section>
{{template "layout_end" .}}
{{end}}

{{define "aprs_reply"}}
{{template "layout_start" .}}
{{$m := .Data.Message}}{{$d := .Data.Detail}}
<div class="between"><h1>{{tr .Lang "web_reply"}}</h1><a class="btn secondary" href="/aprs/received/{{$m.ID}}">{{tr .Lang "web_back_to_aprs"}}</a></div>
<section class="card"><div class="muted">{{tr .Lang "from"}} {{$m.From}} {{tr .Lang "at"}} {{$m.At}}</div><p class="pre">{{$d.Text}}</p></section>
<form class="stack card" method="post" action="/aprs/received/{{$m.ID}}/reply">
  <div class="cols"><div><label>{{tr .Lang "aprs_destination_callsign"}}</label><input name="destination" value="{{.Data.Destination}}" required></div><div><label>{{tr .Lang "aprs_destination_ssid"}}</label><input name="destination_ssid" maxlength="2" value="{{.Data.DestinationSSID}}"></div></div>
  <div><label>{{tr .Lang "aprs_text"}}</label><textarea name="text"></textarea></div>
  <div class="actions"><button class="btn" type="submit">{{tr .Lang "send_button"}}</button><a class="btn secondary" href="/aprs/received/{{$m.ID}}">{{tr .Lang "cancel_button"}}</a></div>
</form>
{{template "layout_end" .}}
{{end}}

{{define "admin_users"}}{{template "layout_start" .}}<h1>{{tr .Lang "web_admin_users"}}</h1><table><tr><th>{{tr .Lang "target_callsign"}}</th><th>{{tr .Lang "full_name"}}</th><th>{{tr .Lang "email"}}</th><th>{{tr .Lang "status"}}</th><th>{{tr .Lang "role"}}</th><th>{{tr .Lang "web_actions"}}</th></tr>{{range .Data}}<tr><td><strong>{{.Callsign}}</strong></td><td>{{.FullName}}</td><td>{{.Email}}</td><td>{{if .Disabled}}{{tr $.Lang "disabled"}}{{else}}{{tr $.Lang "enabled"}}{{end}}</td><td>{{if .IsSysop}}{{tr $.Lang "sysop_role"}}{{else}}{{tr $.Lang "user_role"}}{{end}}</td><td><form class="row" method="post" action="/admin/users/{{.Callsign}}"><select name="disabled"><option value="false">{{tr $.Lang "enabled"}}</option><option value="true" {{if .Disabled}}selected{{end}}>{{tr $.Lang "disabled"}}</option></select><select name="is_sysop"><option value="false">{{tr $.Lang "user_role"}}</option><option value="true" {{if .IsSysop}}selected{{end}}>{{tr $.Lang "sysop_role"}}</option></select><button class="btn secondary">{{tr $.Lang "save_button"}}</button></form></td></tr>{{end}}</table>{{template "layout_end" .}}{{end}}
`
