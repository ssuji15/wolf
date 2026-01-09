#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export WOLF_DIR="$HOME/wolf"
export DATA_DIR="$HOME/data"

echo "========================================"
echo " Wolf local development setup"
echo "========================================"

# --------------------------------------------------
# 1. Bootstrap VM
# --------------------------------------------------
echo "==> Bootstrapping VM"
"$ROOT_DIR/bootstrap-vm-dev.sh"
source /etc/profile.d/go.sh

# --------------------------------------------------
# 2. Docker Compose up
# --------------------------------------------------
echo "==> Starting infrastructure (docker compose)"
cd "$WOLF_DIR/deploy/"

docker compose -f "docker-compose-local.yml" pull
docker compose -f "docker-compose-local.yml" up -d minio 

echo 'Waiting for MinIO to be ready...';
until curl -s http://localhost:9000/minio/health/live >/dev/null 2>&1; do
    sleep 2;
done;
echo 'MinIO ready, creating buckets...';
mc alias set local http://localhost:9000 minioadmin minioadmin123;
mc mb --ignore-existing local/wolf;
mc mb --ignore-existing local/tempo-traces;

docker compose -f "docker-compose-local.yml" up -d tempo nats postgres prometheus grafana alloy

# --------------------------------------------------
# 3. Wait for services to be up
# --------------------------------------------------
echo "==> Waiting for services to become ready"

wait_for_port () {
  local name="$1"
  local host="$2"
  local port="$3"

  echo -n "   - $name "
  until nc -z "$host" "$port"; do
    echo -n "."
    sleep 2
  done
  echo " ready"
}

wait_for_port "Postgres" localhost 5432
wait_for_port "NATS" localhost 4222
wait_for_port "MinIO" localhost 9000

# --------------------------------------------------
# 4. Initialize Postgres schema
# --------------------------------------------------
echo "==> Initializing Postgres schema"

if ! command -v psql >/dev/null; then
   apt-get install -y postgresql-client
fi

export PGPASSWORD="wolf123"
until pg_isready -h localhost -U wolf; do sleep 2; done
psql -h localhost -U wolf -d wolf -f "$WOLF_DIR/tables.sql"

# --------------------------------------------------
# 5. Pull Worker image
# --------------------------------------------------

 docker pull ghcr.io/ssuji15/wolf/wolf-worker:latest
 ctr image pull ghcr.io/ssuji15/wolf/wolf-worker:latest

# --------------------------------------------------
# 6. Initialize Apparmor profile & secomp profile
# --------------------------------------------------

mkdir -p "$DATA_DIR"
 cp "$WOLF_DIR/deploy/config/app_armour/worker-profile" /etc/apparmor.d/
 apparmor_parser -r /etc/apparmor.d/worker-profile
 aa-enforce /etc/apparmor.d/worker-profile

cp "$WOLF_DIR/deploy/config/secomp.json" "$DATA_DIR/secomp.json"

# --------------------------------------------------
# 6. Download Go services
# --------------------------------------------------
echo "==> Downloading Go services"

case "$(uname -s)" in
    Linux*)     OS=linux ;;
    Darwin*)    OS=darwin ;;
    CYGWIN*|MINGW*|MSYS*) OS=windows ;;
    *)          echo "Unsupported OS: $(uname -s)"; exit 1 ;;
esac

# Detect architecture
case "$(uname -m)" in
    x86_64)    ARCH=amd64 ;;
    arm64|aarch64) ARCH=arm64 ;;
    *)         echo "Unsupported architecture: $(uname -m)"; exit 1 ;;
esac
pwd
VERSION=1.0.1
BINARIES=("wolf_server" "sandbox_manager")
for BINARY in "${BINARIES[@]}"; do
    ASSET="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
    URL="https://github.com/ssuji15/wolf/releases/download/v$VERSION/$ASSET"

    echo "Downloading $ASSET..."
    curl -L -o "$ASSET" "$URL"

    echo "Extracting $ASSET..."
    tar -xzf "$ASSET"
    rm "$ASSET"
done

# --------------------------------------------------
# 7. Install binaries
# --------------------------------------------------
echo "==> Installing binaries"

 install -m 0755 wolf_server /usr/local/bin/wolf_server
 install -m 0755 sandbox_manager /usr/local/bin/sandbox_manager

mkdir -p "$DATA_DIR/jobs"

# --------------------------------------------------
# 8. Install systemd units
# --------------------------------------------------
echo "==> Installing systemd services"

 cp "$WOLF_DIR/deploy/config/systemd/env" /etc/default/app
 cp "$WOLF_DIR/deploy/config/systemd/server.service" /etc/systemd/system/wolf_server.service
 cp "$WOLF_DIR/deploy/config/systemd/sandbox_manager.service" /etc/systemd/system/sandbox_manager.service

 systemctl daemon-reload
 systemctl enable wolf_server sandbox_manager
 systemctl restart wolf_server sandbox_manager

echo "========================================"
echo " Wolf dev environment is UP!"
echo "========================================"
