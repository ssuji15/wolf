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
# 5. Build Worker image
# --------------------------------------------------

 docker build -t worker "$HOME/wolf-worker/"
 docker save -o worker.tar worker:latest
 ctr -n default images import worker.tar
 rm worker.tar

# --------------------------------------------------
# 6. Initialize Apparmor profile & secomp profile
# --------------------------------------------------

mkdir -p "$DATA_DIR"
 cp "$WOLF_DIR/config/app_armour/worker-profile" /etc/apparmor.d/
 apparmor_parser -r /etc/apparmor.d/worker-profile
 aa-enforce /etc/apparmor.d/worker-profile

cp "$WOLF_DIR/config/secomp.json" "$DATA_DIR/secomp.json"

# --------------------------------------------------
# 6. Build Go services
# --------------------------------------------------
echo "==> Building Go services"

cd "$WOLF_DIR"
go mod tidy

echo "   - Building web_server"
go build -o web_server .

echo "   - Building sandbox_manager"
go build -o sandbox_manager ./internal/sandbox_manager/

# --------------------------------------------------
# 7. Install binaries
# --------------------------------------------------
echo "==> Installing binaries"

 install -m 0755 web_server /usr/local/bin/web_server
 install -m 0755 sandbox_manager /usr/local/bin/sandbox_manager

mkdir -p "$DATA_DIR/jobs"

# --------------------------------------------------
# 8. Install systemd units
# --------------------------------------------------
echo "==> Installing systemd services"

 cp "$WOLF_DIR/config/systemd/env" /etc/default/app
 cp "$WOLF_DIR/config/systemd/server.service" /etc/systemd/system/web_server.service
 cp "$WOLF_DIR/config/systemd/sandbox_manager.service" /etc/systemd/system/sandbox_manager.service

 systemctl daemon-reload
 systemctl enable web_server sandbox_manager
 systemctl restart web_server sandbox_manager

echo "========================================"
echo " Wolf dev environment is UP!"
echo "========================================"
