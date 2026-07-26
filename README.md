# SSG

Single-Star Generator (A static site generator for Hajime Hoshi)

## Source directories

The source tree separates files by how they are published:

- `src/pages` contains HTML pages that are rendered with layouts, rewritten,
  and minified.
- `src/assets` contains CSS and JavaScript files that are minified.
- `src/static` contains files that are copied to `public` byte-for-byte.
- `src/layouts` contains HTML layouts, and `src/meta.yaml` contains site-wide
  metadata.

Each publishing directory maps directly to the root of `public`. For example,
`src/pages/about.html`, `src/assets/css/site.css`, and
`src/static/images/logo.png` produce `public/about.html`,
`public/css/site.css`, and `public/images/logo.png`, respectively. Generation
fails if multiple source files map to the same output path.

## Local image metadata

Templates can inspect a GIF, JPEG, PNG, or WebP image under `src/static` with
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

Image inspection reads only the generated local file's configuration. Remote
URLs, queries, fragments, non-canonical paths, and path traversal outside
`public` are rejected. A missing image fails static generation; serve mode
returns no image so the `with` block is skipped.
