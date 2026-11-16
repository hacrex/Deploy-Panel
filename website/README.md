# DeployPanel marketing site

Static marketing site for DeployPanel, built in the same editorial UI language as
[cyberpanel.net](https://cyberpanel.net/): warm paper background, deep green ink,
orange signal, oversized type and bordered grids. All copy is DeployPanel's own,
drawn from [docs/BRAND.md](../docs/BRAND.md).

No framework, no build step, no tracking scripts. Three files and two SVGs.

## Preview

Any static file server works. From this folder:

```bash
python3 -m http.server 4173
# open http://127.0.0.1:4173
```

Or from the repository root:

```bash
make website
# same server on http://127.0.0.1:4173
```

## Structure

```
website/
  index.html          one-page site: hero, pathways, walkthrough, features,
                      security, open source, roadmap, quickstart, FAQ, CTA
  assets/styles.css   design tokens and all layout, responsive down to 360px
  assets/site.js      mobile nav, dropdowns, walkthrough tabs, copy button,
                      scroll reveal (all progressive enhancement)
  assets/favicon.svg
  assets/mark.svg
```

The page renders and reads correctly with JavaScript disabled; the only
JS-dependent extras are the mobile menu, the walkthrough tabs and the copy
button. FAQ items use native `<details>`.

## Deploy

The output is plain static files, so any host works:

- **Caddy**: `root * /srv/deploypanel/website` + `file_server`
- **nginx**: `root /srv/deploypanel/website;` + `try_files $uri $uri/ =404;`
- **GitHub Pages / Netlify / Cloudflare Pages**: publish the `website` directory
- **Object storage**: upload the folder with `index.html` as the index document

Long-cache `assets/*` (they are versioned only by filename, so rename or add a
query string when you change them) and keep `index.html` short-lived.

## Editing

- **Colours and measurements** live at the top of `assets/styles.css` under
  `:root`: `--paper`, `--mint`, `--ink`, `--orange`, `--muted`, `--line`,
  `--gutter`, `--max`.
- **Copy** lives in `index.html`. Keep the vocabulary from `docs/BRAND.md`:
  Site, Node, Deployment, Module, Package, Incident. Write the name
  **DeployPanel** in full.
- **Status claims** in the roadmap and FAQ match `docs/ROADMAP.md` as of
  November 2026. Update both when a milestone moves.
- **Fonts**: Inter loads from Google Fonts; the system sans and mono stacks are
  the fallbacks. To self-host, drop the woff2 files in `assets/fonts/`, add
  `@font-face` rules and remove the Google Fonts `<link>` tags.

## Notes

- The links to documentation point at the GitHub repository, not at routes on
  this site. Update them if the docs move to a dedicated docs domain.
- The product mockups in the hero and walkthrough are HTML/CSS, not images, so
  they stay sharp and can be edited in place.
- Before launch, replace the placeholder social metadata and add a real Open
  Graph image (1200×630 PNG) plus a canonical URL once the domain is chosen.
