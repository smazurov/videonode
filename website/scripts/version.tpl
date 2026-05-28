{"version": "{{ (first .Entries).Semver }}", "date": "{{ date_in_zone "2006-01-02" (first .Entries).Date "UTC" }}"}
