#!/bin/bash
set -euo pipefail

die () {
    echo >&2 "$@"
    exit 1
}

DRY_RUN=0
BRANCH_OVERRIDE=0

while [[ "${1:-}" == --* ]]; do
    case "${1}" in
        --dry-run)  DRY_RUN=1; shift ;;
        --dev|--force) BRANCH_OVERRIDE=1; shift ;;
        *) die "Unknown flag: $1" ;;
    esac
done

VERSION=${1:-}

CURRENT_VERSION=$(git tag | grep "^v[0-9]" | sort -V | tail -1 || true)

echo "CURRENT_VERSION: ${CURRENT_VERSION:-<none>}"

if [[ -z "${VERSION}" ]]; then
    if [[ -z "${CURRENT_VERSION}" ]]; then
        VERSION="v0.1.0"
    else
        VERSION=$(echo "${CURRENT_VERSION}" | awk -F. '{$NF+=1} 1' OFS=".")
    fi
    echo "New version is: ${VERSION}"
fi
echo "VERSION: $VERSION"

if [[ ! $VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "VERSION must be a semantic version: vMAJOR.MINOR.PATCH"
    exit 1
fi

CURRENT_BRANCH=$(git branch --show-current)
if [[ 'main' != "$CURRENT_BRANCH" && $BRANCH_OVERRIDE -eq 0 ]]; then
    echo "Not on main git branch!"
    exit 1
fi

if [[ -n "$(git tag -l "$VERSION")" ]]; then
    echo "VERSION $VERSION already exists!"
    exit 1
fi

if [[ $VERSION != `(echo $VERSION; git tag | grep ^v[0-9] || true) | sort -V | tail -1` ]]; then
    echo "$VERSION is not semantically newest!"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Generating version $VERSION - Last Version: ${CURRENT_VERSION:-<none>}"

CHART="$SCRIPT_DIR/helm/Chart.yaml"

if [[ $DRY_RUN -eq 1 ]]; then
    echo "[DRY-RUN] Would update $CHART: version=${VERSION:1}, appVersion=${VERSION:1}"
    echo "[DRY-RUN] Would commit: Release $VERSION"
    echo "[DRY-RUN] Would tag: $VERSION"
    echo "[DRY-RUN] Would push: origin $CURRENT_BRANCH $VERSION"
    exit 0
fi

sed "s/^version:.*/version: ${VERSION:1}/" "$CHART" | sed "s/^appVersion:.*/appVersion: ${VERSION:1}/" > "$CHART.tmp"
mv "$CHART.tmp" "$CHART"

git add helm/Chart.yaml
git commit -S -m "Release $VERSION"
git tag $VERSION
git push origin $CURRENT_BRANCH $VERSION
