#!/usr/bin/env bash
set -euo pipefail

IMAGE_NAME="${IMAGE_NAME:-quazmoz/groupme-mcp}"
TAG="${TAG:-dev}"
ADDITIONAL_TAGS="${ADDITIONAL_TAGS:-}"
BUILD="${BUILD:-false}"
NO_CACHE="${NO_CACHE:-false}"
USERNAME="${DOCKERHUB_USERNAME:-quazmoz}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker CLI is not installed or not available on PATH." >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "Docker daemon is not reachable. Start Docker Desktop or the Docker service and try again." >&2
  exit 1
fi

if [[ -z "${DOCKERHUB_USERNAME:-}" ]]; then
  read -r -p "Docker Hub username [${USERNAME}]: " entered_username
  if [[ -n "${entered_username}" ]]; then
    USERNAME="${entered_username}"
  fi
fi

if [[ "${BUILD}" == "true" ]]; then
  IMAGE_NAME="${IMAGE_NAME}" TAG="${TAG}" ADDITIONAL_TAGS="${ADDITIONAL_TAGS}" NO_CACHE="${NO_CACHE}" "${SCRIPT_DIR}/docker-build.sh"
fi

read -rsp "Docker Hub PAT or password: " DOCKERHUB_TOKEN
echo

cleanup() {
  docker logout >/dev/null 2>&1 || true
  unset DOCKERHUB_TOKEN
}
trap cleanup EXIT

echo "${DOCKERHUB_TOKEN}" | docker login --username "${USERNAME}" --password-stdin

tags=("${TAG}")
if [[ -n "${ADDITIONAL_TAGS}" ]]; then
  IFS=',' read -r -a extra_tags <<< "${ADDITIONAL_TAGS}"
  for raw_tag in "${extra_tags[@]}"; do
    trimmed_tag="$(echo "${raw_tag}" | xargs)"
    if [[ -n "${trimmed_tag}" && ! " ${tags[*]} " =~ " ${trimmed_tag} " ]]; then
      tags+=("${trimmed_tag}")
    fi
  done
fi

for current_tag in "${tags[@]}"; do
  image_ref="${IMAGE_NAME}:${current_tag}"
  if ! docker image inspect "${image_ref}" >/dev/null 2>&1; then
    echo "Local image tag ${image_ref} was not found. Build it first or set BUILD=true." >&2
    exit 1
  fi
  docker push "${image_ref}"
done

if command -v git >/dev/null 2>&1; then
  if git_short_sha="$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null)"; then
    sha_ref="${IMAGE_NAME}:${git_short_sha}"
    if docker image inspect "${sha_ref}" >/dev/null 2>&1; then
      docker push "${sha_ref}"
    fi
  fi
fi

echo "Pushed image tags:"
for pushed_tag in "${tags[@]}"; do
  echo " - ${IMAGE_NAME}:${pushed_tag}"
done
