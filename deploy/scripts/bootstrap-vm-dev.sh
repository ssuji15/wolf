#!/usr/bin/env bash
set -euo pipefail

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) GO_ARCH="amd64" ;;
  aarch64|arm64) GO_ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

apt-get update -y
apt-get upgrade -y

apt-get install -y \
  ca-certificates \
  curl \
  gnupg \
  lsb-release \
  software-properties-common \
  build-essential \
  wget \
  netcat-openbsd \
  apparmor \
  apparmor-utils \
  postgresql-client

# --------------------------------------------------
# Docker + containerd
# --------------------------------------------------
echo "==> Installing Docker"

apt-get remove -y docker docker-engine docker.io containerd runc || true
rm -f /etc/apt/sources.list.d/docker.list
rm -f /etc/apt/keyrings/docker.gpg
install -m 0755 -d /etc/apt/keyrings

curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
| gpg --dearmor -o /etc/apt/keyrings/docker.gpg

chmod a+r /etc/apt/keyrings/docker.gpg
echo \
"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
$(lsb_release -cs) stable" \
| tee /etc/apt/sources.list.d/docker.list > /dev/null

apt-get update -y

# Install — add || true if you want to continue even if it partially fails
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin || {
echo "!!! Docker installation packages failed — check logs !!!"
journalctl -u apt-daily.service --since "10 minutes ago"   # or just cat /var/log/apt/*
exit 1
}
sudo usermod -aG docker $USER


echo "==> Enabling Docker"
systemctl enable docker
systemctl start docker

# --------------------------------------------------
# AppArmor
# --------------------------------------------------
echo "==> Enabling AppArmor"
systemctl enable apparmor
systemctl start apparmor

# --------------------------------------------------
# Go
# --------------------------------------------------
GO_VERSION="1.25.5"
if ! command -v go >/dev/null; then
  echo "==> Installing Go ${GO_VERSION}"
  curl -LO "https://go.dev/dl/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
  rm -rf /usr/local/go
  tar -C /usr/local -xzf "go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
  rm "go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"

  echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
  chmod +x /etc/profile.d/go.sh
  export PATH=$PATH:/usr/local/go/bin

fi

# --------------------------------------------------
# MinIO client (mc)
# --------------------------------------------------
echo "==> Installing MinIO client (mc)"
if ! command -v mc >/dev/null; then
  curl -Lo /usr/local/bin/mc \
    "https://dl.min.io/client/mc/release/linux-${GO_ARCH}/mc"
  chmod +x /usr/local/bin/mc
fi

# --------------------------------------------------
# CRUN
# --------------------------------------------------

CONTAINERD_CONFIG="/etc/containerd/config.toml"
CRUN_BIN="/usr/bin/crun"
CRUN_VERSION="1.26"
BINARY="crun-${CRUN_VERSION}-linux-${GO_ARCH}"
URL="https://github.com/containers/crun/releases/download/${CRUN_VERSION}/${BINARY}"

echo "Downloading crun ${CRUN_VERSION} for ${GO_ARCH}..."
echo "→ ${URL}"
curl -L -o "${CRUN_BIN}" "${URL}"
chmod +x "${CRUN_BIN}"

cp /home/ubuntu/wolf/deploy/config/config.toml "$CONTAINERD_CONFIG"

systemctl daemon-reexec
systemctl restart containerd
systemctl enable containerd

# --------------------------------------------------
# Final sanity checks
# --------------------------------------------------
echo "==> Versions"
docker --version
docker compose version
go version
mc --version
psql --version

echo "========================================"
echo " Bootstrap complete."
echo "========================================"
