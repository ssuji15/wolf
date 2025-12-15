#!/usr/bin/env bash
set -euo pipefail

cd $HOME

# Install go 
curl -LO https://go.dev/dl/go1.25.5.linux-arm64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.25.5.linux-arm64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee /etc/profile.d/go.sh
export PATH="$PATH:/usr/local/go/bin"

# Install docker
sudo apt-get update
sudo apt-get install -y ca-certificates curl gnupg
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
  https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | \
  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo usermod -aG docker $USER

# create docker network
sudo docker network inspect wolf >/dev/null 2>&1 || sudo docker network create wolf

# build worker image
sudo docker build -t worker "$HOME/wolf-worker/"
sudo docker save -o worker.tar worker:latest
sudo ctr -n default images import worker.tar
sudo rm worker.tar

# create data directory
DATA_DIR="$HOME/data"
mkdir -p "$DATA_DIR"
mkdir -p "$DATA_DIR/jetstream/nats-data"
mkdir -p "$DATA_DIR/minio/data"
mkdir -p "$DATA_DIR/pgdata"
mkdir -p "$DATA_DIR/tempo/tempo_data"
mkdir -p "$DATA_DIR/grafana_data"
mkdir -p "$DATA_DIR/alloy"
mkdir -p "$DATA_DIR/prometheus/"
mkdir -p "$DATA_DIR/jobs"
cp "$HOME/wolf/config/jetstream/nats.conf" "$DATA_DIR/jetstream/nats.conf"
cp "$HOME/wolf/config/tempo/tempo.yaml" "$DATA_DIR/tempo/tempo.yaml"
cp "$HOME/wolf/config/alloy/alloy.hcl" "$DATA_DIR/alloy/alloy.hcl"
cp "$HOME/wolf/config/prometheus/prometheus.yml" "$DATA_DIR/prometheus/prometheus.yml"
sudo chown -R 10001:10001 "$DATA_DIR/tempo/tempo_data"
sudo chown -R 472:472 "$DATA_DIR/grafana_data"

# configure apparmour
sudo apt update && sudo apt install -y apparmor apparmor-utils
sudo cp "$HOME/wolf/config/app_armour/worker-profile" /etc/apparmor.d/
sudo apparmor_parser -r /etc/apparmor.d/worker-profile
sudo aa-enforce /etc/apparmor.d/worker-profile

# copy secomp profile
cp "$HOME/wolf/config/secomp.json" "$DATA_DIR/secomp.json"

# run jetstream
sudo docker rm -f natsjs 2>/dev/null || true
sudo docker run -d --name natsjs \
  --network wolf \
  -p 4222:4222 \
  -p 8222:8222 \
  -v "$DATA_DIR/jetstream/nats.conf:/etc/nats/nats.conf" \
  -v "$DATA_DIR/jetstream/nats-data:/data/jetstream" \
  nats:latest \
  -c /etc/nats/nats.conf

# run & configure minio
sudo docker rm -f minio 2>/dev/null || true
sudo docker run -d \
  --name minio \
  --network wolf \
  -p 9000:9000 \
  -p 9090:9090 \
  -v "$DATA_DIR/minio/data:/data" \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin123 \
  quay.io/minio/minio server /data --console-address ":9090"

wget https://dl.min.io/client/mc/release/linux-arm64/mc
chmod +x mc
sudo mv mc /usr/local/bin/
until curl -sf http://localhost:9000/minio/health/ready; do sleep 2; done
mc alias set localminio http://localhost:9000 minioadmin minioadmin123
mc mb --ignore-existing localminio/wolf
mc mb --ignore-existing localminio/tempo-traces

# run tempo
sudo docker rm -f tempo 2>/dev/null || true
sudo docker run -d \
  --name tempo \
  --network wolf \
  -p 3200:3200 \
  -p 4318:4318 \
  -p 4317:4317 \
  -v "$DATA_DIR/tempo/tempo_data:/tmp/tempo" \
  -v "$DATA_DIR/tempo/tempo.yaml:/etc/tempo/tempo.yaml:ro" \
  grafana/tempo:latest \
  --config.file=/etc/tempo/tempo.yaml \
  --distributor.log-received-spans.enabled=true

# run grafana
sudo docker rm -f grafana 2>/dev/null || true
sudo docker run -d --name grafana \
  --network wolf \
  -p 3000:3000 \
  -v "$DATA_DIR/grafana_data:/var/lib/grafana" \
  grafana/grafana

#run alloy
sudo docker rm -f alloy 2>/dev/null || true
sudo docker run -d \
  --name alloy \
  --network wolf \
  -p 8085:8085 \
  -p 8086:8086 \
  -v "$DATA_DIR/alloy/alloy.hcl:/etc/alloy/alloy.hcl:ro" \
  -e ALLOY_LOG_LEVEL=debug \
  grafana/alloy:latest \
  run /etc/alloy/alloy.hcl

#run prometheus
sudo docker rm -f prometheus 2>/dev/null || true
sudo docker run -d \
  --name prometheus \
  --network wolf \
  -p 9095:9090 \
  -v "$DATA_DIR/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml" \
  prom/prometheus \
  --config.file=/etc/prometheus/prometheus.yml \
  --web.enable-remote-write-receiver

# run and configure postgres
sudo docker rm -f postgres 2>/dev/null || true
sudo docker run --name postgres  \
  -e POSTGRES_USER=wolf \
  -e POSTGRES_PASSWORD=wolf123 \
  -e POSTGRES_DB=wolf \
  -p 5432:5432 \
  -v "$DATA_DIR/pgdata:/var/lib/postgresql/" \
  -d postgres:18

sudo apt update && sudo apt install -y postgresql-client
export PGPASSWORD="wolf123"
until pg_isready -h localhost -U wolf; do sleep 2; done
psql -h localhost -U wolf -d wolf -f "$HOME/tables.sql"

sudo cp "$HOME/wolf/config/systemd/env" /etc/default/app

cd "$HOME/wolf"
go mod tidy
go mod vendor

#build and configure webserver
go build -o web_server .
sudo mv web_server /usr/local/bin/
sudo chmod +x /usr/local/bin/web_server
sudo cp "$HOME/wolf/config/systemd/server.service" /etc/systemd/system/web_server.service
sudo systemctl daemon-reload
sudo systemctl enable web_server
sudo systemctl restart web_server

#build and configure sandbox_manager
go build -o sandbox_manager "./internal/sandbox_manager/"
sudo mv sandbox_manager /usr/local/bin/
sudo chmod +x /usr/local/bin/sandbox_manager
sudo cp "$HOME/wolf/config/systemd/sandbox_manager.service" /etc/systemd/system/sandbox_manager.service
sudo systemctl daemon-reload
sudo systemctl enable sandbox_manager
sudo systemctl start sandbox_manager
