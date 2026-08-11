#!/usr/bin/env bash
set -euo pipefail

# IP 数据库自动更新脚本
# 流程：下载 zip -> 解压到临时目录 -> 校验 -> 原子替换 -> 清理旧文件

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# 默认配置（可通过环境变量覆盖）
BIN_DIR="${BIN_DIR:-${PROJECT_ROOT}/binfiles}"
TMP_DIR="${TMP_DIR:-${PROJECT_ROOT}/tmp/ipdb-update}"
IP2LOCATION_URL="${IP2LOCATION_URL:-https://www.ip2location.com/download?token=YOUR_TOKEN&file=DB1LITEBINIPV6}"
IP2PROXY_URL="${IP2PROXY_URL:-https://www.ip2location.com/download?token=YOUR_TOKEN&file=PX2LITEBIN}"
IP2PROXY_IPV6_URL="${IP2PROXY_IPV6_URL:-https://www.ip2location.com/download?token=YOUR_TOKEN&file=PX2LITECSVIPV6}"

# 下载超时
curl_connect_timeout="${CURL_CONNECT_TIMEOUT:-30}"
curl_max_time="${CURL_MAX_TIME:-300}"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

die() {
    log "ERROR: $*" >&2
    exit 1
}

# 确保目录存在
mkdir -p "${BIN_DIR}"
mkdir -p "${TMP_DIR}"

# 清理函数
cleanup() {
    if [[ -n "${work_dir:-}" && -d "${work_dir}" ]]; then
        rm -rf "${work_dir}"
    fi
}
trap cleanup EXIT

work_dir=$(mktemp -d "${TMP_DIR}/update.XXXXXX")

download_and_extract() {
    local name="$1"
    local url="$2"
    local dest_dir="$3"

    log "Downloading ${name}..."
    local zip_file="${work_dir}/${name}.zip"

    # 下载
    if ! curl -fsSL \
        --connect-timeout "${curl_connect_timeout}" \
        --max-time "${curl_max_time}" \
        -o "${zip_file}" \
        "${url}"; then
        die "Failed to download ${name} from ${url}"
    fi

    # 检查文件大小
    if [[ ! -s "${zip_file}" ]]; then
        die "Downloaded ${name} zip is empty"
    fi

    # 解压到临时目录
    local extract_dir="${work_dir}/${name}"
    mkdir -p "${extract_dir}"
    if ! unzip -q "${zip_file}" -d "${extract_dir}"; then
        die "Failed to extract ${name} zip"
    fi

    # 查找 BIN 文件
    local bin_file
    bin_file=$(find "${extract_dir}" -maxdepth 2 -type f -iname "*.BIN" | head -n1)
    if [[ -z "${bin_file}" ]]; then
        die "No BIN file found in ${name} archive"
    fi

    # 目标目录（保持与原项目结构一致）
    local bin_name
    bin_name=$(basename "$(dirname "${bin_file})")
    local target_dir="${dest_dir}/${bin_name}"

    mkdir -p "${target_dir}"

    # 原子替换：先写到临时文件，再重命名
    local tmp_dest="${target_dir}/.tmp.${bin_name}.BIN"
    cp -f "${bin_file}" "${tmp_dest}"
    mv -f "${tmp_dest}" "${target_dir}/${bin_name}.BIN"

    log "Updated ${name} -> ${target_dir}/${bin_name}.BIN"
}

# 下载并解压 CSV 格式数据库（IP2Proxy IPv6）
download_and_extract_csv() {
    local name="$1"
    local url="$2"
    local dest_dir="$3"

    log "Downloading ${name} CSV..."
    local zip_file="${work_dir}/${name}.zip"

    if ! curl -fsSL \
        --connect-timeout "${curl_connect_timeout}" \
        --max-time "${curl_max_time}" \
        -o "${zip_file}" \
        "${url}"; then
        die "Failed to download ${name} from ${url}"
    fi

    if [[ ! -s "${zip_file}" ]]; then
        die "Downloaded ${name} zip is empty"
    fi

    local extract_dir="${work_dir}/${name}"
    mkdir -p "${extract_dir}"
    if ! unzip -q "${zip_file}" -d "${extract_dir}"; then
        die "Failed to extract ${name} zip"
    fi

    local csv_file
    csv_file=$(find "${extract_dir}" -maxdepth 2 -type f -iname "*.CSV" | head -n1)
    if [[ -z "${csv_file}" ]]; then
        die "No CSV file found in ${name} archive"
    fi

    local csv_name
    csv_name=$(basename "$(dirname "${csv_file}")")
    local target_dir="${dest_dir}/${csv_name}"
    mkdir -p "${target_dir}"

    local tmp_dest="${target_dir}/.tmp.${csv_name}.CSV"
    cp -f "${csv_file}" "${tmp_dest}"
    mv -f "${tmp_dest}" "${target_dir}/${csv_name}.CSV"

    log "Updated ${name} -> ${target_dir}/${csv_name}.CSV"
}

main() {
    log "Starting IP database update..."
    log "BIN_DIR=${BIN_DIR}"

    download_and_extract "IP2LOCATION" "${IP2LOCATION_URL}" "${BIN_DIR}"
    download_and_extract "IP2PROXY" "${IP2PROXY_URL}" "${BIN_DIR}"
    download_and_extract_csv "IP2PROXY-IPV6" "${IP2PROXY_IPV6_URL}" "${BIN_DIR}"

    log "IP database update completed successfully."
    log "Restart risk-engine service to load new BIN/CSV files."
}

main "$@"
