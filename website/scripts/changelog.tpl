# Changelog

Generated at build time from [`changelog.yml`](https://github.com/smazurov/videonode/blob/main/changelog.yml). Cross-reference [GitHub Releases](https://github.com/smazurov/videonode/releases) for download links and signed artifacts.

{{ range .Entries -}}
## {{ .Semver }}

_{{ date_in_zone "2006-01-02" .Date "UTC" }}_

{{ range .Changes -}}
- {{ .Note }}{{ if .Commit }} ([`{{ substr 0 7 .Commit }}`](https://github.com/smazurov/videonode/commit/{{ .Commit }})){{ end }}
{{ end }}
{{ end -}}
