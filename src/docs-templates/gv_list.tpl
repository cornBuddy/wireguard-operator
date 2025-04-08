{{- define "gvList" -}}
{{- $groupVersions := . -}}

---
title: CRDs
nav_order: 1
---

# API Reference

## Packages
{{- range $groupVersions }}
- {{ markdownRenderGVLink . }}
{{- end }}

{{ range $groupVersions }}
{{ template "gvDetails" . }}
{{ end }}

{{- end -}}
