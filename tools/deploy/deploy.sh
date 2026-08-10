#!/usr/bin/env bash
set -euo pipefail

# efilter 一键部署脚本（CentOS 7/8/Stream）
# 用法：
#   1. 将本项目 push 到 GitHub
#   2. 在 CentOS 服务器上：
#      curl -fsSL https://raw.githubusercontent.com/YOUR_USER/efilter/main/tools/deploy/deploy.sh | sudo bash
#   或者先 clone 再执行：
#      sudo bash tools/deploy/deploy.sh

# ==================== 配置（按需修改） ====================
PROJECT_NAME="efilter"
REPO_URL="${REPO_URL:-https://github.com/JR-coderli/efilter.git}"
INSTALL_DIR="${INSTALL_DIR:-/opt/efilter}"
APP_USER="${APP_USER:-efilter}"
GO_VERSION="${GO_VERSION:-1.25.0}"
DB_NAME="${DB_NAME:-risk_engine}"
DB_USER="${DB_USER:-postgres}"
# 生产环境请修改密码
DB_PASSWORD="${DB_PASSWORD:-admin123}"
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
REDIS_PORT="${REDIS_PORT:-6379}"
API_PORT="${API_PORT:-8080}"

# IP 数据库下载地址（请替换为实际 token）
IP2LOCATION_URL="${IP2LOCATION_URL:-https://www.ip2location.com/download?token=YOUR_TOKEN&file=DB1LITEBINIPV6}"
IP2PROXY_URL="${IP2PROXY_URL:-https://www.ip2location.com/download?token=YOUR_TOKEN&file=PX2LITEBIN}"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

die() {
    log "ERROR: $*" >&2
    exit 1
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

install_dependencies() {
    log "Installing dependencies..."
    if command_exists dnf; then
        dnf update -y
        dnf install -y curl wget git unzip postgresql-server postgresql-contrib redis nginx
    elif command_exists yum; then
        yum update -y
        yum install -y curl wget git unzip postgresql-server postgresql-contrib redis nginx
    else
        die "No supported package manager found (dnf/yum)"
    fi
}

install_go() {
    if command_exists go && go version | grep -q "${GO_VERSION}"; then
        log "Go ${GO_VERSION} already installed"
        return
    fi

    log "Installing Go ${GO_VERSION}..."
    local arch="amd64"
    local go_tar="go${GO_VERSION}.linux-${arch}.tar.gz"
    cd /tmp
    rm -rf /usr/local/go
    wget -q "https://go.dev/dl/${go_tar}" || die "Failed to download Go"
    tar -C /usr/local -xzf "${go_tar}"
    rm -f "${go_tar}"

    cat > /etc/profile.d/go.sh <<EOF
export PATH=\$PATH:/usr/local/go/bin
export GOPROXY=https://goproxy.cn,direct
EOF
    source /etc/profile.d/go.sh
    go version || die "Go installation failed"
}

setup_postgresql() {
    log "Setting up PostgreSQL..."
    if command_exists postgresql-setup; then
        postgresql-setup --initdb || true
    elif [ -f /usr/pgsql-*/bin/postgresql-setup ]; then
        /usr/pgsql-*/bin/postgresql-setup --initdb || true
    fi

    systemctl enable postgresql
    systemctl start postgresql

    # Wait for PostgreSQL to be ready
    sleep 3

    # Create database and user if not exists
    sudo -u postgres psql -c "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1 || \
        sudo -u postgres psql -c "CREATE DATABASE ${DB_NAME};"

    sudo -u postgres psql -c "ALTER USER ${DB_USER} WITH PASSWORD '${DB_PASSWORD}';"

    # Update pg_hba.conf for local md5 auth
    local pg_hba
    pg_hba=$(find /var/lib/pgsql -name pg_hba.conf 2>/dev/null | head -n1)
    if [ -n "${pg_hba}" ]; then
        sed -i 's/^local\s\+all\s\+all\s\+peer/local   all             all                                     md5/' "${pg_hba}"
        sed -i 's/^host\s\+all\s\+all\s\+127.0.0.1\/32\s\+ident/host    all             all             127.0.0.1\/32            md5/' "${pg_hba}"
        systemctl restart postgresql
    fi
}

setup_redis() {
    log "Setting up Redis..."
    systemctl enable redis
    systemctl start redis
}

create_user() {
    if ! id -u "${APP_USER}" >/dev/null 2>&1; then
        log "Creating user ${APP_USER}..."
        useradd -r -s /bin/false -d "${INSTALL_DIR}" "${APP_USER}"
    fi
}

deploy_project() {
    log "Deploying project to ${INSTALL_DIR}..."
    if [ -d "${INSTALL_DIR}/.git" ]; then
        cd "${INSTALL_DIR}"
        git pull origin main || die "Failed to pull latest code"
    else
        rm -rf "${INSTALL_DIR}"
        git clone "${REPO_URL}" "${INSTALL_DIR}" || die "Failed to clone repository"
    fi
    chown -R "${APP_USER}:${APP_USER}" "${INSTALL_DIR}"
}

build_service() {
    log "Building risk-engine service..."
    cd "${INSTALL_DIR}/backend/risk-engine"
    export PATH=$PATH:/usr/local/go/bin
    export GOPROXY=https://goproxy.cn,direct
    go mod download || die "Failed to download Go modules"
    go build -o risk-engine ./cmd/server/main.go || die "Failed to build risk-engine"
    chown "${APP_USER}:${APP_USER}" risk-engine
    mkdir -p logs
    chown -R "${APP_USER}:${APP_USER}" logs
}

update_ipdb() {
    log "Updating IP databases..."
    cd "${INSTALL_DIR}"
    export IP2LOCATION_URL
    export IP2PROXY_URL
    bash tools/update-ipdb/update-ipdb.sh || log "Warning: IP database update failed, will retry later"
    chown -R "${APP_USER}:${APP_USER}" binfiles
}

configure_service() {
    log "Configuring systemd service..."
    cat > /etc/systemd/system/efilter.service <<EOF
[Unit]
Description=efilter risk engine
After=network.target postgresql.service redis.service

[Service]
Type=simple
User=${APP_USER}
Group=${APP_USER}
WorkingDirectory=${INSTALL_DIR}/backend/risk-engine
ExecStart=${INSTALL_DIR}/backend/risk-engine/risk-engine
Restart=always
RestartSec=5
Environment="PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin"

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    systemctl enable efilter
}

configure_nginx() {
    log "Configuring Nginx..."
    cat > /etc/nginx/conf.d/efilter.conf <<EOF
server {
    listen 80;
    server_name _;

    access_log /var/log/nginx/efilter-access.log;
    error_log /var/log/nginx/efilter-error.log;

    location / {
        proxy_pass http://127.0.0.1:${API_PORT};
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF

    systemctl enable nginx
    systemctl restart nginx
}

start_service() {
    log "Starting efilter service..."
    systemctl restart efilter
    sleep 3
    systemctl status efilter --no-pager || die "Service failed to start"
}

main() {
    log "Starting efilter deployment..."

    install_dependencies
    install_go
    setup_postgresql
    setup_redis
    create_user
    deploy_project
    build_service
    update_ipdb
    configure_service
    configure_nginx
    start_service

    log "Deployment completed!"
    log "Dashboard: http://YOUR_SERVER_IP/"
    log "API: http://YOUR_SERVER_IP/api/v1/"
}

main "$@"
