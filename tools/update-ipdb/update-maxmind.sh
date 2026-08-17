#!/usr/bin/env bash
set -euo pipefail

# MaxMind GeoLite2 自动更新脚本
# 使用系统 geoipupdate 拉取 GeoLite2-ASN/City/Country 到本地目录。

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# 默认配置（可通过环境变量覆盖）
GEOIP_CONF="${GEOIP_CONF:-${PROJECT_ROOT}/configs/GeoIP.conf}"
GEOIP_DB_DIR="${GEOIP_DB_DIR:-${PROJECT_ROOT}/binfiles/maxmind}"
LOG_FILE="${LOG_FILE:-${PROJECT_ROOT}/logs/geoipupdate.log}"
GEOIPUPDATE_BIN="${GEOIPUPDATE_BIN:-/usr/bin/geoipupdate}"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

die() {
    log "ERROR: $*" >&2
    exit 1
}

mkdir -p "${GEOIP_DB_DIR}"
mkdir -p "$(dirname "${LOG_FILE}")"

if [[ ! -f "${GEOIP_CONF}" ]]; then
    die "GeoIP.conf not found at ${GEOIP_CONF}; copy GeoIP.conf.example and fill AccountID/LicenseKey"
fi

if [[ ! -x "${GEOIPUPDATE_BIN}" ]]; then
    die "geoipupdate not found or not executable at ${GEOIPUPDATE_BIN}"
fi

log "Updating MaxMind GeoLite2 databases to ${GEOIP_DB_DIR}..."
"${GEOIPUPDATE_BIN}" -v -f "${GEOIP_CONF}" -d "${GEOIP_DB_DIR}" >> "${LOG_FILE}" 2>&1 || die "geoipupdate failed, see ${LOG_FILE}"
log "MaxMind update completed. Files in ${GEOIP_DB_DIR}:"
ls -lh "${GEOIP_DB_DIR}" >> "${LOG_FILE}"
log "Restart risk-engine service to load new .mmdb files."
