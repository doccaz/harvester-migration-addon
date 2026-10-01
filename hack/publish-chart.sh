#!/usr/bin/env bash
# Package charts/harvester-migration at VERSION (a v-prefixed tag) and publish the
# .tgz + index.yaml to the gh-pages branch, served by GitHub Pages as a Helm repo.
# Creates gh-pages on first use and keeps earlier versions in the index.
#
# Env: VERSION (required, e.g. v0.1.0)  PAGES_URL (required)  GIT_USER  GIT_EMAIL
set -euo pipefail

: "${VERSION:?VERSION is required, e.g. v0.1.0}"
: "${PAGES_URL:?PAGES_URL is required}"
V="${VERSION#v}"
CHART=charts/harvester-migration
WT="$(mktemp -d)"

git config user.name "${GIT_USER:-github-actions[bot]}"
git config user.email "${GIT_EMAIL:-github-actions[bot]@users.noreply.github.com}"

# Dependencies are fetched by repository URL, which Helm requires to be a known repo.
helm repo add harvester https://charts.harvesterhci.io >/dev/null 2>&1 || true
helm dependency build "$CHART"

if git ls-remote --exit-code --heads origin gh-pages >/dev/null 2>&1; then
  git fetch origin gh-pages
  git worktree add "$WT" gh-pages
else
  git worktree add --orphan -b gh-pages "$WT"
fi

# appVersion = image tag. The chart's own version follows the same release.
helm package "$CHART" --version "$V" --app-version "$V" --destination "$WT"

if [ -f "$WT/index.yaml" ]; then
  helm repo index "$WT" --url "$PAGES_URL" --merge "$WT/index.yaml"
else
  helm repo index "$WT" --url "$PAGES_URL"
fi

cd "$WT"
git add -A
if git diff --cached --quiet; then
  echo "Nothing to publish."
else
  git commit -m "Publish harvester-migration $V"
  git push origin HEAD:gh-pages
fi
