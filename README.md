# mod

Optional modules live here, one directory each: `mod/wordpress`, `mod/dns`, `mod/mail`, `mod/waf`, `mod/resell`, `mod/import`. See `docs/MODULES.md` for the contract.

Rules, enforced by `internal/boundary`:

1. Code under `internal/core` must not import anything under `mod/`.
2. A module must not import another module. Modules talk to each other through core interfaces and events.

A module is a Go package that implements `module.Module` and embeds its own migrations, with tables prefixed by the module name (`wp_`, `dns_`, `mail_`, `waf_`, `resell_`).

No modules are implemented yet.
