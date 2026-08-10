package report

import (
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/Cyberlane/mori/internal/model"
)

var htmlReportTemplate = template.Must(template.New("report").Funcs(template.FuncMap{
	"percent": func(value float64) string { return fmt.Sprintf("%.1f%%", value*100) },
	"join":    strings.Join,
}).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Mori structural review</title>
<style>
:root{color-scheme:light dark;--forest:#0f766e;--gold:#d97706;--muted:#64748b;--panel:#ffffff;--ink:#17202a;--line:#d8e1df}
@media(prefers-color-scheme:dark){:root{--panel:#13211f;--ink:#ecf4f2;--muted:#a7b8b4;--line:#2f4843}}
*{box-sizing:border-box}body{margin:0;background:color-mix(in srgb,var(--forest) 8%,Canvas);color:var(--ink);font:15px/1.55 system-ui,sans-serif}
main{max-width:1050px;margin:auto;padding:2.5rem 1rem 5rem}header{border-left:.45rem solid var(--gold);padding:.2rem 1rem;margin-bottom:2rem}h1{margin:0;color:var(--forest);font-size:2.2rem}h2{font-size:1.1rem;margin:.2rem 0}.summary{color:var(--muted)}
.notice{padding:.8rem 1rem;border:1px solid var(--line);border-radius:.6rem;background:var(--panel);margin:1rem 0}.groups{display:grid;gap:1rem}.group{background:var(--panel);border:1px solid var(--line);border-radius:.75rem;padding:1rem;box-shadow:0 4px 18px #0000000b}.score{color:var(--forest);font-weight:750}.identity{font:12px ui-monospace,monospace;color:var(--muted);overflow-wrap:anywhere}.locations{display:grid;grid-template-columns:repeat(auto-fit,minmax(260px,1fr));gap:.6rem;margin:.8rem 0}.location{padding:.65rem;border-radius:.45rem;background:color-mix(in srgb,var(--forest) 8%,var(--panel));font-family:ui-monospace,monospace}.features{color:var(--muted)}footer{margin-top:2rem;color:var(--muted)}
</style>
</head>
<body><main>
<header><h1>森 Mori</h1><div class="summary">{{.TotalMatchGroups}} content-pair groups · {{.TotalLocationPairs}} location pairs · {{.Fragments}} fragments · {{.Files}} files</div></header>
<div class="notice"><strong>Review evidence, not a verdict.</strong> Structural similarity does not prove semantic or behavioral equivalence.</div>
{{if .Truncated}}<div class="notice">This report is truncated to {{len .Groups}} retained groups.</div>{{end}}
<section class="groups">
{{range $index,$group := .Groups}}<article class="group">
<h2><span class="score">{{percent $group.Similarity}}</span> structural similarity · {{$group.LocationPairs}} location pair(s)</h2>
<div class="identity">{{$group.ID}}</div>
<div class="locations">{{range $profile := $group.Profiles}}{{range $occurrence := $profile.Occurrences}}<div class="location">{{$occurrence.Location.Path}}:{{$occurrence.Location.StartLine}}–{{$occurrence.Location.EndLine}}<br>{{$occurrence.Location.Language}} · {{$occurrence.Location.Name}}</div>{{end}}{{end}}</div>
{{if $group.ShapeSummary}}<div class="features">Shared shape: {{join $group.ShapeSummary ", "}}</div>{{end}}
{{if $group.ReviewSignals}}<div class="features">Review signals: {{join $group.ReviewSignals ", "}}</div>{{end}}
</article>{{else}}<div class="notice">No retained structural-similarity groups met the configured threshold.</div>{{end}}
</section>
{{if .Warnings}}<section><h2>Warnings</h2>{{range .Warnings}}<div class="notice">{{.Path}}: {{.Message}}</div>{{end}}</section>{{end}}
<footer>Generated locally by Mori. Source code is not embedded in this report.</footer>
</main></body></html>
`))

// HTML writes a standalone, source-free report suitable for local review.
func HTML(writer io.Writer, report model.Report) error {
	return htmlReportTemplate.Execute(writer, report)
}
