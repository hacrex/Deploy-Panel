# Template spec

Status: Draft v0.1. Implements PRD F-P4: "Template catalog with a unified spec; import CapRover and Coolify catalogs". The `templates` table lives in `migrations/0001_core.sql`; this page defines the catalog format, the unified spec, variable substitution and import mapping.

## 1. Purpose

Templates let users create a Site from a catalog entry instead of writing a `spec` by hand. A template is a parameterized `spec` plus a list of `variables`. Instantiating a template means rendering the variables into a Site create request, which then follows the normal deploy flow (build → proxy route → live).

Catalog entries are:
- `builtin`: shipped with dpd (WordPress, PHP app, static site, container app, proxy).
- `community`: contributed, reviewed before enablement.
- `custom`: operator-authored for a specific fleet.
- `caprover` / `coolify`: imported from those catalogs and normalized to the unified spec.

## 2. Template resource

Fields (matches the `templates` table):

| Field | Type | Notes |
|---|---|---|
| `id` | ULID text | Primary key |
| `slug` | text, unique | Catalog handle, e.g. `wordpress`, `ghost`, `docker-app` |
| `title` | text | Human name, e.g. "WordPress" |
| `source` | text | `builtin` \| `caprover` \| `coolify` \| `community` \| `custom` |
| `site_type` | text | One of the Site types from SITE_DATA_MODEL.md section 3 |
| `spec` | JSON | Template spec with variable references (section 3) |
| `variables` | JSON array | Variable definitions (section 3) |
| `updated_at` | RFC3339 UTC text | Last change |

Only `source = 'custom'` templates may be created or edited through the API; builtin/imported templates are read-only catalog entries.

## 3. Template spec and variables

### 3.1 Variables

Each variable is a JSON object:

```json
{
  "name": "db_password",
  "label": "Database password",
  "type": "secret",            // string | number | boolean | select | secret
  "required": true,
  "default": null,
  "options": ["mysql", "mariadb"],   // for select
  "placeholder": "leave blank to generate",
  "min": 0, "max": 100,        // for number
  "pattern": "^[a-z0-9-]+$"    // for string
}
```

Variable types:
- `string`: free text, validated against `pattern` if present.
- `number`: validated against `min`/`max` if present.
- `boolean`: true/false.
- `select`: one of `options`.
- `secret`: like string but treated as a Site secret (encrypted at rest, write-only, never returned). The rendered value goes into `site_env` with `is_secret=1`, never into the public `spec`.

### 3.2 Spec with variable references

The template `spec` is a JSON document where variables are referenced as `{{ variable.name }}`. Example — a container app template:

```json
{
  "image": "{{ image }}",
  "port": 8080,
  "command": ["sh", "-c", "{{ start_command }}"],
  "healthcheck": { "path": "/healthz", "port": 8080 },
  "env": {
    "APP_ENV": "{{ app_env }}",
    "DB_HOST": "{{ db_host }}",
    "DB_PASSWORD": "{{ db_password }}"
  },
  "secrets": {
    "DB_PASSWORD": "{{ db_password }}"
  }
}
```

Rendering rules:
- Every `{{ variable.name }}` in `spec` must match a defined variable; unknown references fail validation.
- Required variables without a supplied value fail with `template.invalid_variables`.
- `secret` variables are extracted from the rendered spec into `secrets` (write-only); non-secret variables become `env`.
- The rendered result is a normal Site create request; `source_kind` on the resulting deployment is `template`.

### 3.3 Example: WordPress template

```json
{
  "slug": "wordpress",
  "title": "WordPress",
  "source": "builtin",
  "site_type": "wordpress",
  "spec": {
    "php_version": "{{ php_version }}",
    "docroot": "/public",
    "wp": {
      "version": "{{ wp_version }}",
      "auto_update": "{{ auto_update }}",
      "cache": ["page", "object"]
    },
    "database_id": "{{ database_id }}"
  },
  "variables": [
    { "name": "php_version", "type": "select", "options": ["8.2","8.3","8.4"], "default": "8.3", "required": true },
    { "name": "wp_version", "type": "string", "default": "6.8", "required": false },
    { "name": "auto_update", "type": "select", "options": ["none","minor","major"], "default": "minor", "required": false },
    { "name": "database_id", "type": "string", "pattern": "^[0-9A-HJKMNP-TV-Z]{26}$", "required": true },
    { "name": "db_password", "type": "secret", "required": true }
  ]
}
```

## 4. Import mapping

| Source | Concept | Maps to |
|---|---|---|
| CapRover `captain-definition` | `schemaVersion`, `template`, `dockerfile`, `dockerCompose`, `services[]`, env, variables | `site_type` = `compose` or `container`; `spec.file` or `spec.image`; env → variables |
| Coolify | resource `type`, `dockerComposeContent`, `dockerfileContent`, `buildPack`, `env_variables[]` | `site_type` from type; `spec.file`/`spec.build`; env → variables |

Imports are best-effort; fields that cannot be represented (provider-specific addons) are left unrendered and flagged for manual review. Import target: 90% of apps imported without manual edits (PRD success metric).

## 5. Catalog rules

- A template must instantiate to a valid Site create request (validated against `internal/core/site` spec structs at catalog load time).
- Templates must not embed operator secrets in `spec`; credentials are variables of type `secret`.
- Changing a builtin template is a code change with a migration or a seed data update; custom templates are API-managed.
- Template catalog is team-scoped visibility in v0.4 (packages/resellers); in v0.1 it is global read-only.
