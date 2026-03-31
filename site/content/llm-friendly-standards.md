# LLM-Friendly Website Standards — Implementation Guide

> Production-quality reference for making websites fully LLM-compatible.

---

## 1. llms.txt Specification

### File Location
- **Primary**: `/llms.txt` (root of domain)
- **Alternative**: `/docs/llms.txt` (subpath for documentation sites)

### Required Fields (MUST have)

```markdown
# <Project/Site Name>

> <Short summary (blockquote) - key information for understanding the rest>
```

### Optional Fields

```markdown
## <Section Name>

- [Link text](https://example.com/path): Description of linked resource
```

### Full Format Structure (in order)

1. **H1** — Project/site name (ONLY required field)
2. **Blockquote** — Short summary with key info
3. **Zero or more paragraphs/lists** — Detailed project info
4. **Zero or more H2 sections** — File lists with markdown hyperlinks

### Example (FastHTML)

```markdown
# FastHTML

> FastHTML is a python library which brings together Starlette, Uvicorn, HTMX, and fastcore's `FT` "FastTags" into a library for creating server-rendered hypermedia applications.

Things to remember when writing FastHTML apps:

- Although parts of its API are inspired by FastAPI, it is *not* compatible with FastAPI syntax
- FastHTML is compatible with JS-native web components and any vanilla JS library, but not with React, Vue, or Svelte

## Docs

- [FastHTML quick start](https://fastht.ml/docs/tutorials/quickstart.html.md): Brief overview of many FastHTML features

## Examples

- [Todo list application](https://github.com/AnswerDotAI/fasthtml/blob/main/examples/adv_app.py): Detailed walk-thru of a complete CRUD app

## Optional

- [Starlette documentation](https://gist.githubusercontent.com/.../starlette-sml.md): Subset useful for FastHTML development
```

---

## 2. llms-full.txt / llms-ctx.txt

### Purpose
Expanded version containing FULL content of linked URLs (not just links).

### Difference from llms.txt

| File | Content | Use Case |
|------|---------|----------|
| `llms.txt` | Links only | Lightweight index |
| `llms-ctx.txt` | Full content (no URLs) | Full context for LLM |
| `llms-ctx-full.txt` | Full content (with URLs) | Full context + source tracking |

### Generation
Use `llms_txt2ctx` CLI tool:

```bash
pip install llms-txt
llms_txt2ctx https://example.com/llms.txt
# Outputs: llms-ctx.txt, llms-ctx-full.txt
```

### FastHTML Implementation

```markdown
# llms-ctx.txt (expanded)
[Document sections with full content embedded]
```

---

## 3. Content Negotiation — Serving Markdown

### Approach 1: Dual Extension
Serve `.md` version at same URL + `.md` extension:

```
/docs/tutorial.html      → /docs/tutorial.html.md
/docs/index.html        → /docs/index.html.md
```

### Approach 2: Accept Header Response

Server configuration to serve `text/markdown` when requested:

```nginx
# Nginx: Content negotiation for markdown
location / {
    # If Accept header contains text/markdown, serve .md version
    if ($http_accept ~ "text/markdown") {
        rewrite ^/(.*)$ /$1.md last;
    }
}
```

```apache
# Apache: Content negotiation
Options +MultiViews
AddType text/markdown .md
```

### MIME Types

| Content-Type | File Extension | Notes |
|--------------|----------------|-------|
| `text/markdown` | `.md` | Preferred for LLMs |
| `text/plain` | `.txt` | Fallback |
| `text/html` | `.html` | Standard web |

---

## 4. robots.txt for LLM Crawlers

### Known LLM Bot User-Agents

| Provider | User-Agent | Purpose |
|----------|------------|---------|
| OpenAI | `GPTBot` | ChatGPT training/inference |
| OpenAI | `ChatGPT-User` | User-initiated requests |
| Anthropic | `ClaudeBot` | Claude web search |
| Anthropic | `Claude-Web` | Claude inference |
| Google | `Google-Extended` | Gemini/Google AI |
| Common Crawl | `CCBot` | Common Crawl index |
| Perplexity | `PerplexityBot` | Perplexity AI |
| Apple | `Applebot` | Apple Intelligence |
| Meta | `FacebookBot` | Meta AI |

### robots.txt Example

```robots.txt
# Allow all AI crawlers full access
User-agent: *
Allow: /

# Specific bot rules
User-agent: GPTBot
Allow: /
Disallow: /private/
Disallow: /api/

User-agent: ClaudeBot
Allow: /
Disallow: /admin/

User-agent: CCBot
Allow: /
Disallow: /login/

# Sitemap
Sitemap: https://example.com/sitemap.xml
```

### Block Specific Bots

```robots.txt
User-agent: GPTBot
Disallow: /

User-agent: ClaudeBot
Disallow: /
```

---

## 5. Sitemap Considerations

### Standard Sitemap (sitemap.xml)

- Lists ALL indexable pages
- Include `lastmod`, `changefreq`, `priority`
- Include markdown versions in same URL set

### LLM-Optimized Sitemap Additions

```xml
<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/docs/llms.txt</loc>
    <lastmod>2026-03-31</lastmod>
    <changefreq>weekly</changefreq>
    <priority>1.0</priority>
  </url>
  <url>
    <loc>https://example.com/docs/tutorial.html.md</loc>
    <lastmod>2026-03-31</lastmod>
    <changefreq>monthly</changefreq>
    <priority>0.8</priority>
  </url>
</urlset>
```

### Sitemap Index for LLMs

Create `/llms-sitemap.xml` specifically for LLM resources:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap>
    <loc>https://example.com/llms.txt</loc>
    <lastmod>2026-03-31</lastmod>
  </sitemap>
  <sitemap>
    <loc>https://example.com/llms-ctx.txt</loc>
    <lastmod>2026-03-31</lastmod>
  </sitemap>
</sitemapindex>
```

---

## 6. Emerging Standards

### llms-ctx.txt
- Expanded version with full content
- Generated by `llms_txt2ctx`
- Two variants: with/without URLs

### .well-known Path
- Proposed: `/.well-known/llms.txt`
- Follows OAuth/.well-known pattern
- Not widely adopted yet

###Proposed Headers
- `Link: </llms.txt>; rel="llms-txt"`
- Similar to `rel="alternate"` for sitemaps

---

## 7. Meta Tags for LLM Discoverability

### Current State
No W3C-standardized LLM meta tags exist yet.

### Proposed Patterns (informal)

```html
<!-- Informational (not standardized) -->
<meta name="llm-index" content="true">
<meta name="llms-full" content="/llms.txt">
<meta name="llm-version" content="2026.1">
```

### Structured Data Approach

```json
{
  "@context": "https://schema.org",
  "@type": "TechArticle",
  "name": "API Documentation",
  "description": "Complete API reference for...",
  "about": {
    "@type": "SoftwareSourceCode",
    "name": "Our Library"
  },
  "url": "https://example.com/docs/api"
}
```

---

## 8. JSON-LD / Structured Data

### Recommended Schemas

```json
{
  "@context": "https://schema.org",
  "@type": "SoftwareSourceCode",
  "name": "Our Library",
  "description": "A Python library for...",
  "programmingLanguage": "Python",
  "author": {
    "@type": "Organization",
    "name": "Our Company"
  },
  "url": "https://example.com",
  "license": "https://opensource.org/licenses/MIT"
}
```

### Documentation Schema

```json
{
  "@context": "https://schema.org",
  "@type": "TechArticle",
  "proficiencyLevel": "Expert",
  "about": {
    "@type": "SoftwareSourceCode",
    "name": "Our Library"
  }
}
```

### Link in llms.txt

Reference structured data in llms.txt:

```markdown
## Documentation

- [API Docs](https://example.com/docs/api.md): Full API reference

## Structured Data

The site uses JSON-LD Schema.org for enhanced semantic understanding.
```

---

## 9. Real-World Implementation Examples

### FastHTML (Primary Reference)

- **URL**: https://fastht.ml/docs/llms.txt
- **llms-ctx.txt**: https://fastht.ml/docs/llms-ctx.txt
- **llms-ctx-full.txt**: https://fastht.ml/docs/llms-ctx-full.txt
- **.md versions**: All pages available as `.md`

### Implementation Tools

| Tool | Platform | URL |
|------|----------|-----|
| `llms_txt2ctx` | CLI/Python | llmstxt.org |
| vitepress-plugin-llms | VitePress | GitHub |
| docusaurus-plugin-llms | Docusaurus | GitHub |
| llms-txt-php | PHP | GitHub |
| Drupal LLM Support | Drupal | drupal.org |

---

## 10. Implementation Checklist

### Core Files

- [ ] `/llms.txt` — Main index file
- [ ] `/llms-ctx.txt` — Expanded context (optional)
- [ ] `/llms-ctx-full.txt` — Full context with URLs (optional)
- [ ] `/robots.txt` — Include LLM crawler rules
- [ ] `/sitemap.xml` — Standard sitemap
- [ ] `/.well-known/llms.txt` — Future-proof location (optional)

### Markdown versions

- [ ] Serve `.md` at each `.html` URL
- [ ] Configure `text/markdown` content-type

### SEO/Crawler

- [ ] Add GPTBot, ClaudeBot to robots.txt
- [ ] Include all docs in sitemap
- [ ] Set appropriate lastmod dates

### Structured Data

- [ ] Add JSON-LD SoftwareSourceCode schema
- [ ] Reference in llms.txt

### Testing

- [ ] Test with `curl -A "GPTBot" https://yoursite.com/llms.txt`
- [ ] Test with `curl -A "ClaudeBot" https://yoursite.com/llms.txt`
- [ ] Verify markdown rendering
- [ ] Test content negotiation

---

## References

- Official Spec: https://llmstxt.dify.ai/
- GitHub Repo: https://github.com/AnswerDotAI/llms-txt
- FastHTML Example: https://fastht.ml/docs/llms.txt
- CLI Tool: https://llmstxt.org/intro.html
