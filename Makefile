# ===================================================
# Wolf Local Dev Environment Makefile
# ===================================================

VM_NAME ?= wolf-dev
DIR = /home/ubuntu
SCRIPT_DIR = $(DIR)/wolf/deploy/scripts
SSH = multipass exec $(VM_NAME) --
BUILD ?= 0

.PHONY: init create clone up shell destroy

# --------------------------------------------------
# Main target: create VM, clone files, run setup
# --------------------------------------------------
init: create clone up
	@echo "Wolf dev environment ready"

# --------------------------------------------------
# Create Multipass VM
# --------------------------------------------------
UNAME_S := $(shell uname -s)

# Detect total CPUs and memory
ifeq ($(UNAME_S),Linux)
  TOTAL_CPUS := $(shell nproc)
  TOTAL_MEM_GB := $(shell free -g | awk '/^Mem:/{print $$2}')
else ifeq ($(UNAME_S),Darwin)
  TOTAL_CPUS := $(shell sysctl -n hw.ncpu)
  TOTAL_MEM_GB := $(shell sysctl -n hw.memsize | awk '{printf "%.0f\n", $$1/1024/1024/1024}')
else
  $(error Unsupported OS: $(UNAME_S))
endif

# Calculate 75% of total, capped at max 12
VM_CPUS := $(shell awk 'BEGIN { v=int($(TOTAL_CPUS)*0.75); print (v < 12 ? v : 12) }')
VM_MEM_GB := $(shell awk 'BEGIN { v=int($(TOTAL_MEM_GB)*0.75); print (v < 12 ? v : 12) }')
VM_MEM := $(VM_MEM_GB)G

create:
	@echo "==> Creating Multipass VM ($(VM_NAME)) ($(VM_CPUS)) ($(VM_MEM))"
	@multipass info $(VM_NAME) >/dev/null 2>&1 || \
	multipass launch \
	  --name $(VM_NAME) \
	  --memory $(VM_MEM) \
	  --disk 20G \
	  --cpus $(VM_CPUS)

# --------------------------------------------------
# Clone project files to VM
# --------------------------------------------------
clone:
	@echo "==> Cloning project files"
	$(SSH) rm -r wolf || true
	$(SSH) git clone https://github.com/ssuji15/wolf.git

# --------------------------------------------------
# Run dev setup script inside VM
# --------------------------------------------------
up:
	@echo "==> Running dev setup as root inside VM"
	$(SSH) sudo chmod +x $(SCRIPT_DIR)/bootstrap-vm-dev.sh $(SCRIPT_DIR)/dev-setup.sh
	$(SSH) sudo bash $(SCRIPT_DIR)/dev-setup.sh $(BUILD)

# --------------------------------------------------
# Open a shell inside the VM
# --------------------------------------------------
shell:
	multipass shell $(VM_NAME)

# --------------------------------------------------
# Destroy VM
# --------------------------------------------------
destroy:
	multipass delete $(VM_NAME)
	multipass purge
