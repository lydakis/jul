#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION_FILE="${OPENCODE_VERSION_FILE:-${ROOT_DIR}/scripts/opencode-version.txt}"
if [[ -z "${OPENCODE_VERSION:-}" ]]; then
  VERSION="$(cat "${VERSION_FILE}")"
else
  VERSION="${OPENCODE_VERSION}"
fi
BASE_URL="https://github.com/anomalyco/opencode/releases/download/v${VERSION}"
DEST_ROOT="${ROOT_DIR}/dist/opencode"

PLATFORMS=(
  "darwin amd64 x64"
  "darwin arm64 arm64"
  "linux amd64 x64"
  "linux arm64 arm64"
)

mkdir -p "${DEST_ROOT}"

for entry in "${PLATFORMS[@]}"; do
  os="$(echo "${entry}" | awk '{print $1}')"
  goarch="$(echo "${entry}" | awk '{print $2}')"
  assetarch="$(echo "${entry}" | awk '{print $3}')"
  asset="opencode-${os}-${assetarch}.zip"
  url="${BASE_URL}/${asset}"
  out_dir="${DEST_ROOT}/${os}_${goarch}"
  tmp_zip="${out_dir}/${asset}"

  mkdir -p "${out_dir}"
  echo "Downloading ${url}"
  curl -fsSL -o "${tmp_zip}" "${url}"
  unzip -q -o "${tmp_zip}" -d "${out_dir}"
  rm -f "${tmp_zip}"

  if [[ -f "${out_dir}/opencode" ]]; then
    chmod +x "${out_dir}/opencode"
  elif [[ -f "${out_dir}/opencode.exe" ]]; then
    chmod +x "${out_dir}/opencode.exe"
  else
    echo "opencode binary not found in ${out_dir}" >&2
    exit 1
  fi
done

echo "Bundled OpenCode ${VERSION} into ${DEST_ROOT}"
