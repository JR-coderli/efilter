#!/usr/bin/env bash
# 在 postgres 容器内幂等创建 dashboard 查询索引。
# 用法：应用容器首次启动完成自动迁移后执行一次
#   bash docker/create-indexes.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}"

# psql 逐条执行（每条独立 autocommit），CREATE INDEX CONCURRENTLY 不能在事务块内运行
docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U postgres -d risk_engine <<'SQL'
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_domain_trgm ON access_logs USING gin (domain gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_page_path_trgm ON access_logs USING gin (page_path gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_country_ip2location_trgm ON access_logs USING gin (country_ip2location gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_country_maxmind_trgm ON access_logs USING gin (country_maxmind gin_trgm_ops);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_max_city ON access_logs(max_city);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_access_logs_max_asn ON access_logs(max_asn);
SQL

echo "Indexes created (existing ones skipped)."
