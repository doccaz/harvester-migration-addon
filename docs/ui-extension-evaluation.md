# Evaluation: a Rancher UI extension instead of a separate UI?

Status: evaluation only (Phase 4.7), 2026-10-04. Nothing here is a commitment. Items marked **(verify)** could not be
checked without the lab and should be before anyone builds on them.

## The question
Today the UI is its own service (React + a Go backend), reached through a `NavLink` and the Kubernetes service proxy.
Each user's browser mints a short-lived Rancher token and sends it as `X-Migration-Token` (docs/identity.md). A Rancher/Harvester
**UI extension** would instead run inside the dashboard itself. Would that be better?

## What an extension is, as far as this project needs
A bundle of JavaScript (Vue 3, built against Rancher's `@rancher/shell`) packaged in a Helm chart and registered with a
`UIPlugin` object (`catalog.cattle.io`); the dashboard loads it into its own page, with the signed-in user's session.
**(verify)** that the dashboard on this Harvester version offers extensions at all: `kubectl get uiplugins.catalog.cattle.io -A`
and look for an Extensions entry in the dashboard's menu.

## What it would give us
- **No token dance.** The extension talks to the cluster through the dashboard's own authenticated API access, so the
  per-user RBAC story needs no `X-Migration-Token`, no Rancher token minting, no CA copying, and no service-proxy RBAC.
- **Native look, navigation and components** (resource tables, YAML editors, namespace pickers, the dashboard's theme),
  and real menu entries instead of one `NavLink`; "View in Harvester" links would become in-app navigation.
- **One place for users:** the pages could appear next to the built-in VM Import resources.

## What it would cost
- **A rewrite of the frontend in Vue** against a framework we do not control and that changes with Rancher releases.
  The React code is ~5 modules and ~200 behaviour tests today; none of it carries over except the API contract and the tests'
  intent. Rough size: weeks, not days.
- **The backend stays.** The browser cannot reach vCenter (govmomi runs server-side), cannot create export Jobs, serve OVAs or build
  the support bundle. So the extension would still call our Go service (through the proxy, authenticated by the dashboard), and
  some of the auth plumbing would remain. The "backend-less" simplification applies only to plain CRUD of Kubernetes objects.
- **Version coupling to Rancher and Harvester's shell**, each upgrade a risk; an extension can break on a dashboard upgrade
  with no change on our side. The separate UI is only coupled to the Kubernetes API and the proxy.
- **Distribution** becomes the chart + a `UIPlugin` + an image serving the bundle, on top of what we already ship **(verify)**
  how this fits `harvester/experimental-addons`.

## Options
| Option | Effort | Gains | Verdict |
|---|---|---|---|
| A. Keep the separate UI (now) | none | works, tested, lab-verified | **Recommended for now** |
| B. Embed our React UI in an extension page (iframe) | small | menu entry | marginal: auth and look unchanged |
| C. Native Vue extension for the pages that are plain CRUD (plans, sources), Go backend for the rest | large | auth, look, navigation | revisit on the triggers below |
| D. Full rewrite | largest | as C | not justified |

## Recommendation
Stay with A. The pain an extension removes (token minting, CA copy, proxy RBAC) is solved and verified on the lab, while its
cost is a Vue rewrite and a new compatibility surface. What was cheap and valuable has already been done: the "View in Harvester"
links and the engine registry (the UI's own pages are now small, per-engine modules, which also makes a later port easier).

## Revisit when
1. The add-on is accepted upstream (`experimental-addons`) and the Harvester maintainers want it integrated natively.
2. The token-minting flow breaks (a Rancher change to `/v3/tokens`, CSRF or the service proxy), which has been the least
   stable part of the design.
3. Users ask for the pages inside the dashboard menu rather than a separate entry.

## If we do spike it
One or two days: a minimal extension that lists `VmwareSource` objects with the shell's own components and opens one in the
YAML editor, installed on the lab through a `UIPlugin`. Measure: does it load on Harvester's dashboard, how much of the list/detail
code the shell gives for free, and how it authenticates a call to our backend. That answers (verify) items before any commitment.
