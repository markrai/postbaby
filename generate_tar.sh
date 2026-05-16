#!/usr/bin/env bash
set -uo pipefail

cd "$(dirname "$0")"

# Portainer "Upload image" / Images -> Import expects: docker save (image layers), NOT a tar of source files.

echo
echo "[1/2] Building Docker image..."
if ! docker build -t postbaby:local .; then
  echo
  echo "Docker build failed. Start Docker Desktop (or your engine) and retry."
  exit 1
fi

OUT="$(pwd)/postbaby.tar"
rm -f "$OUT"

echo
echo "[2/2] Exporting image for Portainer (docker save)..."
echo "      $OUT"
echo

if ! docker save -o "$OUT" postbaby:local; then
  echo "docker save failed."
  exit 1
fi

echo
echo "Done. In Portainer:"
echo "  1. Images -> Import -> upload this file: $OUT"
echo "  2. Stacks -> use docker-compose.portainer.yml (image postbaby:local, no build on the NAS)"
echo
echo "Note: Build on the same CPU family you deploy to (e.g. amd64 vs arm64) or use docker buildx."
echo
