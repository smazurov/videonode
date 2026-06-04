{{- with (first .Entries) -}}
Released {{ date_in_zone "January 2, 2006" .Date "UTC" }}.

### What's changed
{{ range .Changes }}{{ $note := splitList "\n" .Note -}}
- {{ first $note }}{{ if .Commit }} ([`{{ substr 0 7 .Commit }}`](https://github.com/smazurov/videonode/commit/{{ .Commit }})){{ end }}
{{ end -}}
{{- end -}}
