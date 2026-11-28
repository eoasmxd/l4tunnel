#!/usr/bin/env bash
# 交叉编译并打包各平台发布压缩包到 packages/ 目录
# Cross-compile and package release archives for all platforms into the packages/ directory
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "${ROOT_DIR}"

VERSION="$(sed -n 's/^var version string = "\([^"]*\)".*/\1/p' ./utils/version.go)"
if [[ -z "${VERSION}" ]]; then
    echo "Failed to parse version from ./utils/version.go" >&2
    exit 1
fi

LDFLAGS="-s -w"
OUT_DIR="${ROOT_DIR}/packages"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "${WORK_DIR}"' EXIT

TARGETS=(
    "linux/386"
    "linux/amd64"
    "linux/arm"
    "linux/arm64"
    "linux/mips"
    "linux/mipsle"
    "linux/mips64"
    "linux/mips64le"
    "windows/386"
    "windows/amd64"
)

rm -rf "${OUT_DIR}"
mkdir -p "${OUT_DIR}"

echo "Building release packages for v${VERSION}"

build_target() {
    local os="$1"
    local arch="$2"
    local pkg_name="l4t_${VERSION}_${os}_${arch}"
    local stage_dir="${WORK_DIR}/${pkg_name}"
    local ext=""
    local -a build_env=("CGO_ENABLED=0" "GOOS=${os}" "GOARCH=${arch}")

    if [[ "${os}" == "windows" ]]; then
        ext=".exe"
    fi

    if [[ "${arch}" == "mips" || "${arch}" == "mipsle" ]]; then
        build_env+=("GOMIPS=softfloat")
    fi

    echo "-> ${os}/${arch}"
    mkdir -p "${stage_dir}"

    env "${build_env[@]}" go build -ldflags "${LDFLAGS}" -o "${stage_dir}/l4ts${ext}" ./cmd/l4ts
    env "${build_env[@]}" go build -ldflags "${LDFLAGS}" -o "${stage_dir}/l4tc${ext}" ./cmd/l4tc

    cp ./LICENSE ./NOTICE ./README.md ./README.zh.md ./conf/*.ini "${stage_dir}/"

    if [[ "${os}" == "linux" ]]; then
        mkdir -p "${stage_dir}/systemd"
        cp ./conf/systemd/l4ts.service ./conf/systemd/l4tc.service "${stage_dir}/systemd/"
    fi

    if [[ "${os}" == "windows" ]]; then
        (cd "${WORK_DIR}" && zip -rq "${OUT_DIR}/${pkg_name}.zip" "${pkg_name}")
    else
        tar -C "${WORK_DIR}" -zcf "${OUT_DIR}/${pkg_name}.tar.gz" "${pkg_name}"
    fi

    rm -rf "${stage_dir}"
}

for target in "${TARGETS[@]}"; do
    build_target "${target%/*}" "${target#*/}"
done

echo "Packages written to ${OUT_DIR}"
