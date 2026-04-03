# Piglet development recipes

set shell := ["bash", "-euo", "pipefail", "-c"]

# ── Build & Test ──────────────────────────────────────────────

# Build all packages
build:
    go build ./...

# Run tests with race detector
test:
    go test -race ./... | tail -50

# Run vet
vet:
    go vet ./...

# Full verification
check: build test vet
    go mod verify

# Build binary and symlink to GOBIN
install:
    go build -o piglet ./cmd/piglet/
    ln -sf "$(pwd)/piglet" ~/go/bin/piglet

# ── Deploy ────────────────────────────────────────────────────

# Deploy piglet (tag, push, release)
deploy: (_deploy "false")

# Show deployment plan without executing
deploy-dry: (_deploy "true")

_deploy dry_run:
    #!/usr/bin/env bash
    set -euo pipefail

    # === Preflight ===
    echo "=== Preflight ==="
    if [ -n "$(git status --porcelain)" ]; then
        echo "error: uncommitted changes — commit or stash first" >&2
        exit 1
    fi
    echo "  clean"

    # === Compute version ===
    last_tag=$(git tag -l 'v*' --sort=-v:refname | head -1)
    if [ -z "$last_tag" ]; then
        echo "error: no tags found" >&2
        exit 1
    fi
    ver="${last_tag#v}"
    IFS='.' read -r major minor patch <<< "$ver"
    new_tag="v${major}.${minor}.$((patch + 1))"

    echo ""
    echo "=== Deployment plan ==="
    echo "  $last_tag → $new_tag"
    if command -v gh &>/dev/null; then
        echo "  release: create via gh"
    fi

    if [ "{{dry_run}}" = "true" ]; then
        echo ""
        echo "--dry-run: stopping here."
        exit 0
    fi

    # === Verify ===
    echo ""
    echo "=== Verify ==="
    go build ./...
    echo "  build OK"
    go test -race ./... -count=1 -timeout 300s > /dev/null 2>&1
    echo "  tests OK"

    # === Tag, push, release ===
    echo ""
    echo "=== Deploy ==="
    git push origin main
    echo "  pushed main"
    git tag "$new_tag"
    echo "  tagged $new_tag"
    git push origin "$new_tag"
    echo "  pushed $new_tag"

    if command -v gh &>/dev/null; then
        gh release create "$new_tag" \
            --generate-notes \
            --repo dotcommander/piglet \
            --title "$new_tag" || echo "  warning: release creation failed"
    fi

    echo ""
    echo "=== Deploy complete ==="
    echo "  $new_tag"
    echo "  https://github.com/dotcommander/piglet/releases/tag/$new_tag"
