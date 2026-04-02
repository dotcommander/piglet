# Piglet development recipes

set shell := ["bash", "-euo", "pipefail", "-c"]

sdk_module := "github.com/dotcommander/piglet/sdk"

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

# Deploy piglet + extensions (tag, push, release)
deploy: (_deploy "false" "false")

# Show deployment plan without executing
deploy-dry: (_deploy "true" "false")

# Deploy, skipping SDK tag even if SDK changed
deploy-skip-sdk: (_deploy "false" "true")

_deploy dry_run skip_sdk:
    #!/usr/bin/env bash
    set -euo pipefail

    piglet_dir="$(pwd)"

    # Resolve piglet-extensions: check go.work first, then sibling directory
    ext_dir=""
    gowork="$(go env GOWORK 2>/dev/null || true)"
    if [ -n "$gowork" ]; then
        work_dir="$(dirname "$gowork")"
        while IFS= read -r line; do
            cleaned="${line#use }"
            cleaned="${cleaned%%/}"
            cleaned="${cleaned## }"
            if [[ "$cleaned" == *piglet-extensions ]]; then
                candidate="${work_dir}/${cleaned}"
                [ -d "$candidate" ] && ext_dir="$candidate"
            fi
        done < "$gowork"
    fi
    if [ -z "$ext_dir" ] && [ -d "../piglet-extensions" ]; then
        ext_dir="$(cd ../piglet-extensions && pwd)"
    fi
    if [ -z "$ext_dir" ] || [ ! -d "$ext_dir" ]; then
        echo "error: piglet-extensions not found (checked go.work and ../piglet-extensions)" >&2
        exit 1
    fi

    # === Preflight checks ===
    echo "=== Preflight checks ==="

    if [ -n "$(git -C "$piglet_dir" status --porcelain)" ]; then
        echo "error: piglet has uncommitted changes — commit or stash first" >&2
        exit 1
    fi
    echo "  piglet: clean"

    # Extensions may have go.mod/go.sum changes from SDK bump — ignore those
    ext_dirty=$(git -C "$ext_dir" status --porcelain | grep -v 'go\.\(mod\|sum\)$' || true)
    if [ -n "$ext_dirty" ]; then
        echo "error: piglet-extensions has uncommitted changes (beyond go.mod/go.sum)" >&2
        exit 1
    fi
    echo "  piglet-extensions: clean"

    # === Detect SDK changes ===
    last_sdk_tag=$(git -C "$piglet_dir" tag -l 'sdk/v*' --sort=-v:refname | head -1)
    if [ -z "$last_sdk_tag" ]; then
        echo "error: no SDK tags found" >&2
        exit 1
    fi

    sdk_needs_tag=false
    if [ "{{skip_sdk}}" = "false" ]; then
        if ! git -C "$piglet_dir" diff --quiet "${last_sdk_tag}..HEAD" -- sdk/; then
            sdk_needs_tag=true
        fi
    fi

    # === Compute versions ===
    last_piglet_tag=$(git -C "$piglet_dir" tag -l 'v*' --sort=-v:refname | head -1)
    if [ -z "$last_piglet_tag" ]; then
        echo "error: no piglet tags found" >&2
        exit 1
    fi

    bump_patch() {
        local tag="$1"
        local prefix=""
        local ver="$tag"
        # Handle sdk/vX.Y.Z prefix
        if [[ "$tag" == *"/"* ]]; then
            prefix="${tag%v*}"
            ver="${tag##*/}"
        fi
        ver="${ver#v}"
        local major minor patch
        IFS='.' read -r major minor patch <<< "$ver"
        echo "${prefix}v${major}.${minor}.$((patch + 1))"
    }

    new_piglet_tag=$(bump_patch "$last_piglet_tag")
    new_sdk_tag=""
    if [ "$sdk_needs_tag" = "true" ]; then
        new_sdk_tag=$(bump_patch "$last_sdk_tag")
    fi

    # === Print plan ===
    echo ""
    echo "=== Deployment plan ==="
    echo "  piglet:     $last_piglet_tag → $new_piglet_tag"
    if [ "$sdk_needs_tag" = "true" ]; then
        echo "  SDK:        $last_sdk_tag → $new_sdk_tag"
        echo "  extensions: bump SDK dep, commit, push"
    else
        reason="no changes"
        [ "{{skip_sdk}}" = "true" ] && reason="skipped (--skip-sdk)"
        echo "  SDK:        $last_sdk_tag (no new tag — $reason)"
    fi
    echo "  extensions: verify GOWORK=off build, push"
    if command -v gh &>/dev/null; then
        echo "  release:    create via gh"
    else
        echo "  release:    skip (gh CLI not found)"
    fi

    if [ "{{dry_run}}" = "true" ]; then
        echo ""
        echo "--dry-run: stopping here."
        exit 0
    fi

    # === SDK tag + push ===
    if [ "$sdk_needs_tag" = "true" ]; then
        echo ""
        echo "=== Tagging SDK ==="
        git -C "$piglet_dir" tag "$new_sdk_tag"
        echo "  Tagged $new_sdk_tag"

        git -C "$piglet_dir" push origin main "$new_sdk_tag"
        echo "  Pushed main + $new_sdk_tag"

        # Wait for module proxy
        sdk_version="${new_sdk_tag#sdk/}"
        echo ""
        echo "=== Waiting for module proxy ==="
        echo "  {{sdk_module}}@${sdk_version}"
        for i in $(seq 1 40); do
            if GOWORK=off GONOSUMCHECK='*' go list -m "{{sdk_module}}@${sdk_version}" &>/dev/null; then
                echo "  Module proxy ready."
                break
            fi
            if [ "$i" -eq 40 ]; then
                echo "error: timed out waiting for module proxy" >&2
                exit 1
            fi
            echo "  Still waiting..."
            sleep 3
        done

        # Update extensions go.mod
        echo ""
        echo "=== Updating extensions SDK dep ==="
        (cd "$ext_dir" && GOWORK=off go get "{{sdk_module}}@${sdk_version}")
        echo "  go get {{sdk_module}}@${sdk_version}"
        (cd "$ext_dir" && GOWORK=off go mod tidy)
        echo "  go mod tidy"
    fi

    # === Verify extensions build ===
    echo ""
    echo "=== Verifying extensions build (GOWORK=off) ==="
    (cd "$ext_dir" && GOWORK=off go build ./...)
    echo "  Build OK"

    # === Commit extensions if go.mod changed ===
    if [ "$sdk_needs_tag" = "true" ]; then
        echo ""
        echo "=== Committing extensions ==="
        sdk_version="${new_sdk_tag#sdk/}"
        git -C "$ext_dir" add go.mod go.sum
        git -C "$ext_dir" commit -m "deps: bump piglet SDK to ${sdk_version}"
        echo "  Committed: deps: bump piglet SDK to ${sdk_version}"
    fi

    # === Push extensions ===
    echo ""
    echo "=== Pushing extensions ==="
    git -C "$ext_dir" push origin main
    echo "  Pushed"

    # === Push piglet + tag ===
    echo ""
    echo "=== Tagging and pushing piglet ==="
    if [ "$sdk_needs_tag" = "false" ]; then
        git -C "$piglet_dir" push origin main
        echo "  Pushed main"
    fi
    git -C "$piglet_dir" tag "$new_piglet_tag"
    echo "  Tagged $new_piglet_tag"
    git -C "$piglet_dir" push origin "$new_piglet_tag"
    echo "  Pushed $new_piglet_tag"

    # === GitHub release ===
    if command -v gh &>/dev/null; then
        echo ""
        echo "=== Creating GitHub release ==="
        gh release create "$new_piglet_tag" \
            --generate-notes \
            --repo dotcommander/piglet \
            --title "$new_piglet_tag" || echo "  Warning: release creation failed"
    fi

    # === Summary ===
    echo ""
    echo "=== Deploy complete ==="
    echo "  piglet:     $new_piglet_tag"
    if [ "$sdk_needs_tag" = "true" ]; then
        echo "  SDK:        $new_sdk_tag"
    fi
    echo "  extensions: pushed"
    echo "  release:    https://github.com/dotcommander/piglet/releases/tag/$new_piglet_tag"
