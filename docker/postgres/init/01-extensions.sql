-- 官方 postgres 镜像自带 contrib 模块，pg_trgm 可直接创建。
-- 仅在数据卷首次初始化时执行；老数据卷由 docker/create-indexes.sh 幂等补建。
CREATE EXTENSION IF NOT EXISTS pg_trgm;
