# SSG

Single-Star Generator (A static site generator for Hajime Hoshi)

## Local image metadata

Templates can inspect a GIF, JPEG, PNG, or WebP image under `src/content` with
`.Site.Image`. The argument is a canonical site-root-relative path using
forward slashes. The returned value has `MediaType`, `Width`, and `Height`
fields.

```gotemplate
{{with $image := .Site.Image "/images/share.png"}}
<meta property="og:image:type" content="{{$image.MediaType}}">
<meta property="og:image:width" content="{{$image.Width}}">
<meta property="og:image:height" content="{{$image.Height}}">
{{end}}
```

Image inspection reads only the local file's configuration. Remote URLs,
queries, fragments, non-canonical paths, and path traversal outside
`src/content` are rejected. A missing image fails static generation; serve mode
returns no image so the `with` block is skipped.
