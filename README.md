# SSG

Single-Star Generator (A static site generator for Hajime Hoshi)

## Source directories

The source tree separates files by how they are published:

- `src/pages` contains HTML and Markdown pages that are rendered with layouts,
  rewritten, and minified.
- `src/assets` contains CSS and JavaScript files that are minified.
- `src/static` contains files that are copied to `public` byte-for-byte.
- `src/layouts` contains HTML layouts and shared templates.

Each publishing directory maps directly to the root of `public`. For example,
`src/pages/about.md`, `src/assets/css/site.css`, and
`src/static/images/logo.png` produce `public/about.html`,
`public/css/site.css`, and `public/images/logo.png`, respectively. Generation
fails if multiple source files map to the same output path.

## Layout templates

HTML files in `src/layouts` whose names begin with `_` are shared Go templates
available to every layout. They are not selectable as layouts. A layout can
render a shared template with the `template` action.

```gotemplate
{{template "_base.html" .}}
```

Shared templates in nested directories use their slash-separated path relative
to `src/layouts` as their template name.

## Markdown pages

Markdown pages use GitHub Flavored Markdown with automatic heading IDs and can
contain raw HTML. A local link to a Markdown source file is rewritten to the
generated page URL. For example, `[About](/about.md)` becomes a link to `/about`
when HTML extensions are omitted from page URLs.

A Markdown page can begin with YAML front matter. The values are available to
the layout through `.Page.Meta`; `_layout` selects the page's layout.

```markdown
---
_layout: article
title: About
---

# About
```

## Content title

`.Page.ContentTitle` is the text of the first `h1` in the rendered page
content, including text inside nested markup. It is empty when the content has
no `h1`. A layout can use it while retaining control over title policy:

```gotemplate
<title>{{with .Page.ContentTitle}}{{.}} – {{end}}{{.Site.Name}}</title>
```

## Page metadata

An `_meta.yaml` file provides default metadata for every page in its directory
and descendant directories. Metadata from a closer directory overrides values
from its ancestors, and a page's own metadata overrides all directory defaults.
The merge operates on top-level keys, replacing mapping and sequence values as
whole values. The result is available to layouts through `.Page.Meta`.

For example, `src/pages/ja/_meta.yaml` can set the language for every page under
`src/pages/ja`:

```yaml
language: ja
```

`src/pages/_meta.yaml` provides defaults for every page. The `_layout` value can
also be inherited from directory metadata.

## Local image metadata

Templates can inspect a GIF, JPEG, PNG, or WebP image under `src/static` with
`imageMetadata`. The argument is a canonical site-root-relative path using
forward slashes. The returned value has `MediaType`, `Width`, and `Height`
fields.

```gotemplate
{{with $image := imageMetadata "/images/share.png"}}
<meta property="og:image:type" content="{{$image.MediaType}}">
<meta property="og:image:width" content="{{$image.Width}}">
<meta property="og:image:height" content="{{$image.Height}}">
{{end}}
```

Image inspection reads only the generated local file's configuration. Remote
URLs, queries, fragments, non-canonical paths, and path traversal outside
`public` are rejected. A missing image fails static generation; serve mode
returns no image so the `with` block is skipped.
