# DeployPanel: name, story and voice

## 1. The name

**DeployPanel** joins two words from two worlds.

- **Deploy** is what developers do. It is the PaaS world of CapRover, Coolify, Dokploy and Dokku: push code, get a running app.
- **Panel** is what operators use. It is the hosting world of CloudPanel and CyberPanel: sites, users, databases, email, quotas.

Until now you picked one world and lived with its gaps. The name says the product is both: a panel where deploying is the main verb.

Write it as **DeployPanel** (one word, capital D and P), always in full. Repo and package: `deploypanel`. In conversation, say "DeployPanel" or "the panel".

**Decision (2026-11-10): the name is final.** Do not shorten it to "D-Panel" or "DPanel". DPanel is an existing Docker management panel (`donknap/dpanel`), so a short form would send people to that project. The internal command names `dpd`, `dp-agent` and `dpctl` are fine because they are tools, not the brand.

## 2. One line

> **Deploy anything. See everything. Trust the defaults.**

Alternates for different contexts:
- Developer: "Push code. Get a site. Keep the panel."
- Operator: "One panel for apps, sites and servers."
- Security-minded: "The panel that does not need an open port."
- Short: "Every site, one panel."

## 3. The story

### The problem
Anyone who self-hosts for real ends up running two toolchains. A PaaS for the containerized apps, and a hosting panel for the WordPress site, the client's PHP app and the mailbox. They do not share users, backups, logs or SSL. When something crashes at 2 a.m., neither tool shows what happened, and the panel itself is often the most exposed thing on the server: an admin port open to the internet, SSH keys to every machine, a Docker socket within easy reach.

### The belief
Hosting a site is one job, whether the site is a Next.js container, a Compose stack or a WordPress blog. It should have one lifecycle, one backup policy, one place for logs and one set of permissions.

### The product
DeployPanel treats every workload as a **Site**. Create it, point a domain at it, deploy to it, watch it, back it up, restore it. The type of Site changes what runs underneath, not how you manage it.

### The promise
1. **Deploy anything.** Dockerfile, Compose, image, Git, `git push`, PHP, WordPress, Node, Python, static and proxy Sites, all through the same flow.
2. **See everything.** Live logs, live container stats and a crash snapshot saved at the moment something dies, built in and not bolted on.
3. **Trust the defaults.** No public admin port, no SSH keys held by the panel, nodes connect out, secrets encrypted, every action audited, backups that prove they can be restored.

### The proof points
- A single Go binary with SQLite by default. Target: under 150 MB of RAM at idle.
- Agents dial out over mTLS, so servers need no inbound panel access.
- Every backup can be restore-tested into a scratch Site.
- Importers for CapRover, Coolify, CloudPanel and CyberPanel so moving in is not a rewrite.

## 4. Who it is for

| Who | What they hear |
|---|---|
| **Developers and small teams** | "Push code and get a live site with SSL, logs and rollbacks. No dashboard sprawl." |
| **Agencies and freelancers** | "Host client WordPress and PHP sites with per-site users, staging and backups." |
| **Small hosting operators** | "Packages, quotas and resellers without cPanel pricing or a second tool." |
| **Security-minded self-hosters** | "A panel built so the panel is not your weakest link." |

Lead with the first row for v0.1 and the second row from v0.2, matching the roadmap.

## 5. Voice

- **Plain and direct.** Say what it does. No hype words.
- **Operator-friendly.** Respect that the reader runs real servers. Show commands, limits and trade-offs.
- **Honest about gaps.** Mark what is planned and what ships. Comparisons cite sources and date themselves.
- **Calm about security.** State the design and the limits, never fear.

Avoid: "revolutionary", "next-gen", "seamless", "all-in-one magic". Prefer concrete claims: "under 150 MB at idle", "no inbound ports on nodes".

## 6. Vocabulary

Use these terms consistently in the UI, docs and CLI:

| Term | Meaning | Do not use |
|---|---|---|
| **Site** | Any hosted workload | app, project, service (except in docs about a specific type) |
| **Node** | A server running the agent | host, machine, worker |
| **Deployment** | One revision going live | release, build (a build is a step inside a deployment) |
| **Module** | An optional feature package | plugin, addon |
| **Package** | A quota plan | plan, tier |
| **Incident** | A crash snapshot | crash report |

## 7. Binary and package names

| Part | Name |
|---|---|
| Product | DeployPanel |
| Control plane | `dpd` |
| Node agent | `dp-agent` |
| CLI | `dpctl` |
| Repo and Go module | `deploypanel` |
| Container image | `deploypanel/dpd`, `deploypanel/dp-agent` |

`deployctl` is deliberately not used because Deno Deploy ships a CLI with that name.

## 8. Ready-to-use copy

### README hero
> **DeployPanel** is a lightweight, self-hosted panel that runs every kind of Site, from container apps to WordPress, with built-in logs, crash snapshots and restore-tested backups. Agents connect out, so there is no public admin port and no SSH key to lose.

### 30-second pitch
"Self-hosters run a PaaS for their apps and a hosting panel for their sites, and neither shows what happened when something crashes. DeployPanel is one panel for both. Everything is a Site, so deploys, domains, backups and logs work the same way for a Docker app and a WordPress blog. It is a single Go binary, the servers connect out instead of exposing ports, and every backup can prove it restores."

### LinkedIn launch post (draft)
> Self-hosting today means two toolchains.
>
> PaaS tools (CapRover, Coolify, Dokploy, Dokku): great at deploying containers, thin on email, DNS, quotas and per-site users.
> Hosting panels (CloudPanel, CyberPanel): great at sites and resellers, thin on Git deploys, Compose and observability.
>
> Both tend to share three gaps: weak crash visibility, a large attack surface, and backups nobody has tested.
>
> I am building DeployPanel to close them:
> - Every workload is a Site: container, Compose, PHP, WordPress, Node, static
> - Built-in logs, stats and crash snapshots
> - Outbound-only agents, no public admin port
> - Restore-tested backups
> - Optional modules for DNS, mail, WAF and resellers
>
> Early stage: PRD, architecture and data model are drafted. Feedback from people who run CapRover, Coolify or CloudPanel in production is very welcome.

(Comparison claims in public posts should be re-checked against each project's current docs.)

## 9. Name risks and checks

- A search for "DeployPanel" and "Deploy Panel" found no product with that exact name. The closest is [DevPanel](https://devpanel.com/), a platform for developing, deploying and managing sites on your own cloud, which overlaps in positioning and is one letter away in spelling. IONOS also sells a product called [Deploy Now](https://www.ionos.com/hosting/deploy-now).
- "Deploy" and "Panel" are common words, so trademark protection and search ranking will be weaker than for a coined name. Mitigate with a distinctive logo, the one-word form and consistent use.
- Still to do before public launch: domain (`deploypanel.com`, `.dev`, `.io`), GitHub org, npm, PyPI and Homebrew names, and a trademark search.
