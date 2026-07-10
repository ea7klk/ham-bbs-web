package main

import (
	"fmt"
	"html/template"
)

func parseTemplates() *template.Template {
	funcs := template.FuncMap{
		"langName": func(code string) string { return languages[code] },
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
<html lang="en">
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
    .btn.secondary { background:white; color:var(--accent); }
    .btn.danger { border-color:var(--accent2); background:var(--accent2); }
    table { width:100%; border-collapse:collapse; background:white; border:1px solid var(--line); border-radius:8px; overflow:hidden; }
    th, td { text-align:left; padding:10px; border-bottom:1px solid var(--line); vertical-align:top; }
    th { font-size:13px; color:var(--muted); background:#fbfdfc; }
    .message { border-left:3px solid var(--accent); margin:12px 0; padding:12px 12px 12px 14px; background:white; border-radius:0 8px 8px 0; }
    .indent-1 { margin-left:24px; } .indent-2 { margin-left:48px; } .indent-3 { margin-left:72px; } .indent-4 { margin-left:96px; }
    @media (max-width: 740px) { .top { align-items:flex-start; flex-direction:column; } nav a, button.link { padding:7px 8px; } table { display:block; overflow-x:auto; } .indent-1,.indent-2,.indent-3,.indent-4 { margin-left:12px; } }
  </style>
</head>
<body>
<header><div class="top"><div><div class="brand">{{.AppName}}</div><div class="sub">{{.Location}} - {{.Topic}}</div></div>{{if .User}}<nav><a href="/">Home</a><a href="/bulletins">Bulletins</a><a href="/boards">Boards</a><a href="/directory">Directory</a><a href="/aprs">APRS</a><a href="/profile">{{.User.Callsign}}</a>{{if .IsSysop}}<a href="/admin/users">Sysop</a>{{end}}<form method="post" action="/logout"><button class="link">Log out</button></form></nav>{{end}}</div></header>
<main>
{{if .Flash}}<div class="flash">{{.Flash}}</div>{{end}}
{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
{{end}}
{{define "layout_end"}}</main></body></html>{{end}}

{{define "login"}}{{template "layout_start" .}}<h1>Log in</h1><form class="stack" method="post" action="/login"><div><label>Callsign or handle</label><input name="callsign" autocomplete="username" required autofocus></div><div><label>BBS password</label><input name="password" type="password" autocomplete="current-password" required></div><div class="row"><button class="btn">Log in</button><a class="btn secondary" href="/register">Register</a></div></form>{{template "layout_end" .}}{{end}}

{{define "register"}}{{template "layout_start" .}}<h1>Register</h1><form class="stack" method="post" action="/register">{{template "user_fields" dict "User" .Data.User "Languages" .Data.Languages "Callsign" .Data.Callsign "RequireCallsign" true "RequirePassword" true}}<button class="btn">Create account</button></form>{{template "layout_end" .}}{{end}}

{{define "user_fields"}}{{if .RequireCallsign}}<div><label>Callsign or handle</label><input name="callsign" value="{{.Callsign}}" required></div>{{end}}<div class="cols"><div><label>Full name</label><input name="full_name" value="{{.User.FullName}}" required></div><div><label>Email</label><input name="email" type="email" value="{{.User.Email}}" required></div></div><div class="cols"><div><label>Maidenhead locator</label><input name="maidenhead" value="{{.User.Maidenhead}}"></div><div><label>Language</label><select name="language">{{range .Languages}}<option value="{{.Code}}" {{selected $.User.Language .Code}}>{{.Name}}</option>{{end}}</select></div></div><div class="cols"><div><label>QTH</label><input name="qth" value="{{.User.QTH}}"></div><div><label>Rig</label><input name="rig" value="{{.User.Rig}}"></div></div><div><label>Enable APRS</label><select name="enable_aprs"><option value="false">false</option><option value="true" {{if .User.EnableAPRS}}selected{{end}}>true</option></select></div><div class="cols"><div><label>{{if .RequirePassword}}Password{{else}}New password{{end}}</label><input name="new_password" type="password" {{if .RequirePassword}}required{{end}}></div><div><label>Verify password</label><input name="verify_password" type="password" {{if .RequirePassword}}required{{end}}></div></div>{{end}}

{{define "home"}}{{template "layout_start" .}}<h1>Dashboard</h1><div class="grid"><a class="card" href="/bulletins"><h3>Bulletins</h3><div class="brand">{{.Data.Bulletins}}</div></a><a class="card" href="/boards"><h3>Message boards</h3><div class="brand">{{.Data.Boards}}</div></a><a class="card" href="/directory"><h3>Stations</h3><div class="brand">{{.Data.Users}}</div></a><a class="card" href="/aprs"><h3>Received APRS</h3><div class="brand">{{.Data.Received}}</div></a></div>{{template "layout_end" .}}{{end}}

{{define "profile"}}{{template "layout_start" .}}<h1>Profile</h1><form class="stack" method="post" action="/profile">{{template "user_fields" dict "User" .User "Languages" .Data.Languages "RequireCallsign" false "RequirePassword" false}}<button class="btn">Save profile</button></form>{{template "layout_end" .}}{{end}}

{{define "directory"}}{{template "layout_start" .}}<h1>Station Directory</h1><table><tr><th>Callsign</th><th>Name</th><th>Locator</th><th>QTH</th><th>Rig</th><th>Last seen</th><th>APRS</th></tr>{{range .Data}}<tr><td><strong>{{.Callsign}}</strong></td><td>{{.FullName}}</td><td>{{.Maidenhead}}</td><td>{{.QTH}}</td><td>{{.Rig}}</td><td>{{.LastSeen}}</td><td>{{.EnableAPRS}}</td></tr>{{end}}</table>{{template "layout_end" .}}{{end}}

{{define "bulletins"}}{{template "layout_start" .}}<div class="between"><h1>Bulletins</h1>{{if .IsSysop}}<a class="btn" href="/bulletins/new">New bulletin</a>{{end}}</div>{{range .Data}}<article class="card"><div class="between"><div><h2>{{.Title}}</h2><div class="muted">{{.Updated}}{{if .From}} by {{.From}}{{end}}</div></div>{{if $.IsSysop}}<a class="btn secondary" href="/bulletins/{{.ID}}/edit">Edit</a>{{end}}</div><p class="pre">{{.Body}}</p></article>{{else}}<p>No bulletins yet.</p>{{end}}{{template "layout_end" .}}{{end}}

{{define "bulletin_form"}}{{template "layout_start" .}}<h1>{{if .Data.ID}}Edit{{else}}New{{end}} Bulletin</h1><form class="stack" method="post"><div><label>Title</label><input name="title" value="{{.Data.Title}}" required></div><div><label>Body</label><textarea name="body" required>{{.Data.Body}}</textarea></div><div class="row"><button class="btn">Save</button><a class="btn secondary" href="/bulletins">Cancel</a></div></form>{{if .Data.ID}}<form method="post" action="/bulletins/{{.Data.ID}}/delete" style="margin-top:18px"><button class="btn danger">Delete bulletin</button></form>{{end}}{{template "layout_end" .}}{{end}}

{{define "boards"}}{{template "layout_start" .}}<div class="between"><h1>Message Boards</h1>{{if .IsSysop}}<a class="btn" href="/boards/new">New board</a>{{end}}</div><div class="grid">{{range .Data.Boards}}<a class="card" href="/boards/{{.ID}}"><h3>{{.Name}}</h3><p>{{.Description}}</p><div class="muted">{{index $.Data.Counts .ID}} messages</div></a>{{else}}<p>No boards yet.</p>{{end}}</div>{{template "layout_end" .}}{{end}}

{{define "board"}}{{template "layout_start" .}}<div class="between"><div><h1>{{.Data.Board.Name}}</h1><p class="muted">{{.Data.Board.Description}}</p></div>{{if .IsSysop}}<a class="btn secondary" href="/boards/{{.Data.Board.ID}}/edit">Edit board</a>{{end}}</div><form class="stack card" method="post" action="/boards/{{.Data.Board.ID}}/post"><h3>Post a message</h3><div><label>Subject</label><input name="subject" required></div><div><label>Body</label><textarea name="body" required></textarea></div><button class="btn">Post</button></form><h2>Messages</h2>{{range .Data.Messages}}{{template "message" dict "Node" . "Root" $}}{{else}}<p>No messages yet.</p>{{end}}{{template "layout_end" .}}{{end}}

{{define "message"}}<section class="message indent-{{.Node.Depth}}"><div class="between"><div><strong>{{.Node.Row.Subject}}</strong><div class="muted">From {{.Node.Row.From}} at {{.Node.Row.Created}}{{if .Node.Row.Edited}} - edited {{.Node.Row.Edited}}{{end}}</div></div>{{if .Root.IsSysop}}<form method="post" action="/messages/{{.Node.Row.ID}}/delete"><button class="btn danger">Delete</button></form>{{end}}</div><p class="pre">{{.Node.Row.Body}}</p><details><summary>Reply</summary><form class="stack" method="post" action="/messages/{{.Node.Row.ID}}/reply"><div><label>Subject</label><input name="subject" value="Re: {{.Node.Row.Subject}}" required></div><div><label>Body</label><textarea name="body" required></textarea></div><button class="btn">Reply</button></form></details>{{range .Node.Replies}}{{template "message" dict "Node" . "Root" $.Root}}{{end}}</section>{{end}}

{{define "board_form"}}{{template "layout_start" .}}<h1>{{if .Data.ID}}Edit{{else}}New{{end}} Board</h1><form class="stack" method="post"><div><label>Name</label><input name="name" value="{{.Data.Name}}" required></div><div><label>Description</label><input name="description" value="{{.Data.Description}}"></div><div class="row"><button class="btn">Save</button><a class="btn secondary" href="/boards">Cancel</a></div></form>{{if .Data.ID}}<form method="post" action="/boards/{{.Data.ID}}/delete" style="margin-top:18px"><button class="btn danger">Delete board</button></form>{{end}}{{template "layout_end" .}}{{end}}

{{define "aprs"}}{{template "layout_start" .}}<h1>APRS</h1><section class="card"><form method="post" action="/aprs/toggle" class="row"><label style="margin:0">Enable APRS for {{.User.Callsign}}</label><select name="enable_aprs" style="max-width:140px"><option value="false">false</option><option value="true" {{if .User.EnableAPRS}}selected{{end}}>true</option></select><button class="btn">Save</button></form></section><h2>Send APRS message</h2><form class="stack card" method="post" action="/aprs/send"><div class="cols"><div><label>Destination callsign</label><input name="destination" required></div><div><label>SSID</label><input name="destination_ssid" maxlength="2"></div></div><div><label>Text</label><textarea name="text" required></textarea></div><button class="btn">Send</button></form><h2>Received</h2><table><tr><th>At</th><th>From</th><th>To</th><th>Text</th></tr>{{range .Data.Received}}<tr><td>{{.At}}</td><td>{{.From}}</td><td>{{.To}}</td><td>{{.Text}}</td></tr>{{else}}<tr><td colspan="4">No received APRS messages.</td></tr>{{end}}</table><h2>Sent</h2><table><tr><th>At</th><th>To</th><th>Status</th><th>Text</th></tr>{{range .Data.Sent}}<tr><td>{{.At}}</td><td>{{.To}}</td><td>{{.Status}}{{if .Acked}} acked{{end}}</td><td>{{.Text}}</td></tr>{{else}}<tr><td colspan="4">No sent APRS messages.</td></tr>{{end}}</table>{{template "layout_end" .}}{{end}}

{{define "admin_users"}}{{template "layout_start" .}}<h1>Sysop Users</h1><table><tr><th>Callsign</th><th>Name</th><th>Email</th><th>Status</th><th>Role</th><th>Actions</th></tr>{{range .Data}}<tr><td><strong>{{.Callsign}}</strong></td><td>{{.FullName}}</td><td>{{.Email}}</td><td>{{if .Disabled}}Disabled{{else}}Enabled{{end}}</td><td>{{if .IsSysop}}Sysop{{else}}User{{end}}</td><td><form class="row" method="post" action="/admin/users/{{.Callsign}}"><select name="disabled"><option value="false">enabled</option><option value="true" {{if .Disabled}}selected{{end}}>disabled</option></select><select name="is_sysop"><option value="false">user</option><option value="true" {{if .IsSysop}}selected{{end}}>sysop</option></select><button class="btn secondary">Save</button></form></td></tr>{{end}}</table>{{template "layout_end" .}}{{end}}
`
