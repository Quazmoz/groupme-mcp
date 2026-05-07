#!/usr/bin/env bash
set -euo pipefail

IMAGE_NAME="${IMAGE_NAME:-quazmoz/groupme-mcp}"
TAG="${TAG:-dev}"
ADDITIONAL_TAGS="${ADDITIONAL_TAGS:-}"
NO_CACHE="${NO_CACHE:-false}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker CLI is not installed or not available on PATH." >&2
  exit 1
fi

build_args=(build -t "${IMAGE_NAME}:${TAG}")
if [[ "${NO_CACHE}" == "true" ]]; then
  build_args+=(--no-cache)
fi
build_args+=("${REPO_ROOT}")

docker "${build_args[@]}"

tags=("${TAG}")
if [[ -n "${ADDITIONAL_TAGS}" ]]; then
  IFS=',' read -r -a extra_tags <<< "${ADDITIONAL_TAGS}"
  for raw_tag in "${extra_tags[@]}"; do
    trimmed_tag="$(echo "${raw_tag}" | xargs)"
    if [[ -n "${trimmed_tag}" && ! " ${tags[*]} " =~ " ${trimmed_tag} " ]]; then
      tags+=("${trimmed_tag}")
      docker tag "${IMAGE_NAME}:${TAG}" "${IMAGE_NAME}:${trimmed_tag}"
    fi
  done
fi

if command -v git >/dev/null 2>&1; then
  if git_short_sha="$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null)"; then
    if [[ -n "${git_short_sha}" && ! " ${tags[*]} " =~ " ${git_short_sha} " ]]; then
      tags+=("${git_short_sha}")
      docker tag "${IMAGE_NAME}:${TAG}" "${IMAGE_NAME}:${git_short_sha}"
    fi
  fi
fi

echo "Built image tags:"
for built_tag in "${tags[@]}"; do
  echo " - ${IMAGE_NAME}:${built_tag}"
done
