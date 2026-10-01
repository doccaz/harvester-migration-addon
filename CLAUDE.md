# CLAUDE.md

Read PLAN.md first; it holds the analysis, decisions and phases.

- Merge the deployment, not the code: controller stays the unchanged upstream image.
- Keep the controller names/labels Harvester hard-codes (`harvester-vm-import-controller`).
- Forklift is a separate add-on (harvester/forklift-packaging): detect it, never bundle it.
- Reference clones go in `reference/` (not committed).
- Commit messages: no Co-Authored-By / Claude-Session trailers.
