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
DEST_ROOT="${OPENCODE_DEST_ROOT:-${ROOT_DIR}/build/opencode}"

PLATFORMS=(
  "darwin amd64 x64 zip"
  "darwin arm64 arm64 zip"
  "linux amd64 x64 tar.gz"
  "linux arm64 arm64 tar.gz"
)

mkdir -p "${DEST_ROOT}"

for entry in "${PLATFORMS[@]}"; do
  os="$(echo "${entry}" | awk '{print $1}')"
  goarch="$(echo "${entry}" | awk '{print $2}')"
  assetarch="$(echo "${entry}" | awk '{print $3}')"
  ext="$(echo "${entry}" | awk '{print $4}')"
  asset="opencode-${os}-${assetarch}.${ext}"
  url="${BASE_URL}/${asset}"
  out_dir="${DEST_ROOT}/${os}_${goarch}"
  tmp_file="${out_dir}/${asset}"

  mkdir -p "${out_dir}"
  echo "Downloading ${url}"
  curl -fsSL -o "${tmp_file}" "${url}"
  if [[ "${ext}" == "zip" ]]; then
    unzip -q -o "${tmp_file}" -d "${out_dir}"
  else
    tar -xzf "${tmp_file}" -C "${out_dir}"
  fi
  rm -f "${tmp_file}"

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
