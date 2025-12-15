# Low-Latency Secure Code Execution Engine (Go)
A **secure, low-latency, code execution system** built in Go, optimized for **isolation, throughput, and p99 latency**.

This project demonstrates **distributed systems**, **sandboxing**, **observability**, and **failure-safe execution**, with deliberate tradeoffs between **security, performance, and reliability**.

---

## Highlights

- Isolation-per-worker execution using **containerd, AppArmor, Seccomp, and cgroups**
- Backpressure-aware scheduling with explicit worker availability tracking
- At-least-once job execution semantics via **NATS JetStream**
- End-to-end observability with tracing and metrics across all components
- Failure-safe execution: worker crashes, retries, and persistence handled correctly
- Extensible architecture supporting future runtimes
---

## Key Features

### 🔒 Secure Isolation
- One Job per Worker
- Enforced using:
  - cgroups (CPU, memory, PIDs)
  - AppArmor and Seccomp (syscall restrictions)
- No network access for workers
- Fork bombs and resource exhaustion prevented

---

### 🧠 Backpressure-Aware Scheduling
- Sandbox Managers only pull jobs **when workers are available**
- Prevents queue flooding and latency spikes
- Explicit worker lifecycle and reservation semantics

---

### 🔁 At-Least-Once Execution
- Jobs acknowledged in JetStream **only after successful persistence**
- Automatic retries on:
  - worker crashes
  - transient failures
- Guarantees durable and correct results

---

### ⚡ Fast IPC
- Unix Domain Sockets + gRPC between Sandbox Manager and Worker
- Low overhead, local communication
- Designed for high-frequency job dispatch

---

### 🔍 Observability
- End-to-end tracing:
  - Web Server → Queue → Sandbox Manager → Worker → Storage
- Metrics per stage:
  - queue wait time
  - execution time
  - storage latency
- Enables root-cause analysis:
  > *“Why did Job X take 3 seconds?”*

---

## 🔄 Job Flow

1. User submits job to **Web Server**
2. Code stored in **MinIO**, metadata stored in **Postgres**
3. Job ID published to **NATS JetStream**
4. Sandbox Manager:
   - waits for idle worker
   - assigns job via Unix Domain Socket
5. Worker executes job inside sandboxed container
6. Output persisted to MinIO
7. Job status updated in Postgres
8. JetStream message acknowledged only after successful persistence

---

## 📊 Observability & Tracing

- Traces can be correlated using request IDs and job IDs.
- Traces span across all components
- Prometheus metrics derived from span data
- Supports p50 / p95 / p99 latency analysis
## ⚡ Performance (Local)
### Constraints
| Metric | Value |
|------|------|
| Max input code size | 1 MB |
| Max execution timeout | 2 seconds |
| Workers | 3 |
| VM | 10 cores ( for entire stack) |
| Worker startup | < 100 ms (pre-warmed) |

| Type | p50 | p95 | p99 |
|-----|-----|-----|-----|
|Minio/Uploads|
|Minio/Downloads|
|QueueTime|
|Container Execution time|
|Postgres/Insert|
|Postgres/Get|
|Jetstream/Publish|


> Latency is dominated by execution and storage I/O, not scheduling overhead.

## 🧠 Design Reasoning

- **Isolation vs latency:**  
  Pre-warmed containers provide strong isolation without cold-start penalties.
- **Backpressure over batching:**  
  Prevents latency collapse and keeps p99 predictable.
- **Explicit failure semantics:**  
  Jobs are acknowledged only after durability guarantees.
- **Observability-first design:**  
  Tracing informs both debugging and system evolution.

## 📦 Tech Stack

- Go
- containerd / docker
- NATS JetStream
- Postgres
- MinIO
- OpenTelemetry
- Prometheus / Grafana / alloy / tempo 
- AppArmor / Seccomp / cgroups

## 💻 Getting Started

This project can be run locally using **Multipass**.
### 1. Start the environment
Deploy all components inside a Multipass VM:
```bash
make init
```
### 2. Find the VM IP address
Get the IP address of the running VM:
```
multipass info <vmname>
``` 

### 3. Submit a job
Send a job to the Web Server using a POST request:
```
curl -X POST http://<VM-IP>:8080/job \
  -H "Content-Type: application/json" \
  -d '{
    "code": "#include <bits/stdc++.h>\nusing namespace std;\n\nint main() {\n    cout << \"Hello World\";\n    return 0;\n}",
    "executionEngine": "c++"
  }'
```
Replace <VM-IP> with the IP address obtained from multipass info.

#### Example Response
```
{
    "id": "50c4d683-d15c-46b4-ad49-1b9a234e1ad3",
    "executionEngine": "c++",
    "codePath": "jobs/50c4d683-d15c-46b4-ad49-1b9a234e1ad3/code.bin",
    "codeHash": "e2a7639a3394c64f1db109a97c7a3dd37ada455640b3fa70e11599991a956ed9",
    "status": "Pending",
    "creationTime": "2025-12-16T00:50:23.505604197+05:30",
    "retryCount": 0,
    "tags": null
}
```

### 4. Get job status
Retrieve the job status:
```
curl http://<VM-IP>:8080/job/<job-id>
```

#### Example Response
```
{
    "id": "50c4d683-d15c-46b4-ad49-1b9a234e1ad3",
    "executionEngine": "c++",
    "codePath": "jobs/50c4d683-d15c-46b4-ad49-1b9a234e1ad3/code.bin",
    "codeHash": "e2a7639a3394c64f1db109a97c7a3dd37ada455640b3fa70e11599991a956ed9",
    "status": "Completed",
    "outputPath": "jobs/50c4d683-d15c-46b4-ad49-1b9a234e1ad3/output.log",
    "creationTime": "2025-12-16T00:50:23.505604197+05:30",
    "endTime": "2025-12-16T00:50:24.125136564+05:30",
    "retryCount": 0,
    "outputHash": "fc18dad7662e24cb9d7820aeaff799143d07f81b0d63a9303af6d1f19dd99811",
    "tags": null
}
```

### 5. Download Output
Download the output once the job status is 'completed'
```
curl http://<VM-IP>:8080/job/<job-id>/output
```
#### Example Response
```
Hello World
```

