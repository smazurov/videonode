# Changelog

Generated at build time from [`changelog.yml`](https://github.com/smazurov/videonode/blob/main/changelog.yml). Cross-reference [GitHub Releases](https://github.com/smazurov/videonode/releases) for download links and signed artifacts.

{{ $entries := .Entries -}}
{{ range .Entries -}}
{{ $show := true -}}
{{ if contains "-rc" .Semver -}}
{{ $final := regexReplaceAll "-rc.*$" .Semver "" -}}
{{ range $entries }}{{ if eq .Semver $final }}{{ $show = false }}{{ end }}{{ end -}}
{{ end -}}
{{ if $show -}}
## {{ .Semver }}

_{{ date_in_zone "2006-01-02" .Date "UTC" }}_

{{ range .Changes -}}
- {{ .Note }}{{ if .Commit }} ([`{{ substr 0 7 .Commit }}`](https://github.com/smazurov/videonode/commit/{{ .Commit }})){{ end }}
{{ end }}
{{ end -}}
{{ end -}}
