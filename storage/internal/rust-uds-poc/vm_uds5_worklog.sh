#!/usr/bin/env bash
# vm_uds5_worklog.sh — Turnkey benchmark runner matching the updated
# "gRPC over UDS Approach" tab (t.rn625s1tqaw2) of the GCS SDK Consolidation Worklog:
#   - APIs: BidiReadObject (gRPC), BidiWriteObject (gRPC), ReadObject (JSON),
#           WriteObject (JSON), ListObjects (gRPC), ListBuckets (gRPC)
#   - Workloads 1–3: Rapid SLIs (Open, Read50x2x4, Random8K)
#   - Workload 4: Read 16 MiB chunks via BidiRead on 48 parallel threads (default 5m)
#   - Workload 5: Write 200 MiB chunks via BidiWrite on 48 parallel threads (default 5m)
#   - Workload 6: ListBucket and ListObject for 100 items
#   - Performance parameters collected:
#       * Latency: P50, P90, P99, Max, Mean (outermost host-language boundary)
#       * Throughput: MiB/s (and MB/s) aggregate wall-clock rate
#       * CPU Utilization: % CPU across vCPUs + CPU-s/GiB + ms/op (Go + Rust sidecar)
#       * Memory Consumption: Peak RSS MiB (Go VmHWM + Rust sidecar VmHWM)
#   - Default topology (per Worklog Configuration spec):
#       * VM: c4-standard-192 (Ubuntu 24.04 LTS), 200 Gbps GVNIC (TIER_1) in us-central1-a
#       * Bucket: gs://cpriti-thickthin-poc-usw1 in us-west1 (non-colocated CloudPath)
#
# Usage:
#   ./vm_uds5_worklog.sh provision-vm   # creates us-west1 bucket + us-central1-a c4-standard-192 VM
#   ./vm_uds5_worklog.sh                # runs all 6 workloads on the VM and saves logs to $OUT
set -euo pipefail

BUCKET="${BUCKET:-cpriti-thickthin-poc-usw1}"
BUCKET_LOCATION="${BUCKET_LOCATION:-us-west1}"
VM_NAME="${VM_NAME:-thickthin-poc-uds5}"
VM_ZONE="${VM_ZONE:-us-central1-a}"
VM_MACHINE_TYPE="${VM_MACHINE_TYPE:-c4-standard-192}"
PROJECT="${PROJECT:-$(gcloud config get-value project 2>/dev/null || echo gcs-go-sdk-playground)}"
SUSTAINED_DUR="${SUSTAINED_DUR:-5m}"
SUSTAINED_THREADS="${SUSTAINED_THREADS:-48}"
ITERS="${ITERS:-10}"
OUT="${OUT:-/tmp/results/uds5-worklog}"
RES="${RES:-gs://${BUCKET}/results/uds5-worklog}"

if [[ "${1:-}" == "provision-vm" ]]; then
  export CLOUDSDK_AUTH_ACCESS_TOKEN="${CLOUDSDK_AUTH_ACCESS_TOKEN:-$(gcloud auth application-default print-access-token 2>/dev/null || true)}"
  echo "=== Ensuring bucket gs://${BUCKET} exists in ${BUCKET_LOCATION} ==="
  if ! gcloud storage buckets describe "gs://${BUCKET}" --project="${PROJECT}" >/dev/null 2>&1; then
    gcloud storage buckets create "gs://${BUCKET}" \
      --project="${PROJECT}" \
      --location="${BUCKET_LOCATION}" \
      --uniform-bucket-level-access
  fi
  echo "=== Creating GCE VM ${VM_NAME} (${VM_MACHINE_TYPE}, Ubuntu 24.04, 200 Gbps GVNIC TIER_1) in ${VM_ZONE} ==="
  for z in "${VM_ZONE}" us-central1-b us-central1-f us-central1-c; do
    echo "Trying zone ${z}..."
    if gcloud compute instances create "${VM_NAME}" \
      --project="${PROJECT}" \
      --zone="${z}" \
      --machine-type="${VM_MACHINE_TYPE}" \
      --image-family=ubuntu-2404-lts-amd64 \
      --image-project=ubuntu-os-cloud \
      --boot-disk-size=200GB \
      --boot-disk-type=hyperdisk-balanced \
      --network-interface=nic-type=GVNIC \
      --network-performance-configs=total-egress-bandwidth-tier=TIER_1 \
      --metadata-from-file=startup-script="$0" \
      --scopes=cloud-platform; then
      echo "=== Created ${VM_NAME} in zone ${z} ==="
      exit 0
    fi
  done
  exit 1
fi

mkdir -p "$OUT"
exec > >(tee -a "$OUT/startup.log") 2>&1
stage() {
  echo "=== STAGE: $1 $(date -u +%Y-%m-%dT%H:%M:%SZ) ==="
  echo "$1" > "$OUT/STAGE"
  gcloud storage cp "$OUT/STAGE" "$RES/STAGE" 2>/dev/null || gsutil -q cp "$OUT/STAGE" "$RES/STAGE" 2>/dev/null || true
}
sync_out() {
  gcloud storage cp "$OUT"/* "$RES/" 2>/dev/null || gsutil -q cp -r "$OUT"/* "$RES/" 2>/dev/null || true
}

stage "init-ops-agent"

# Ensure Google Cloud Ops Agent is installed so agent.googleapis.com/memory/percent_used is reported.
if ! systemctl is-active --quiet google-cloud-ops-agent 2>/dev/null; then
  echo "=== Installing Google Cloud Ops Agent for agent.googleapis.com/memory/percent_used ==="
  curl -sSO https://dl.google.com/cloudagents/add-google-cloud-ops-agent-repo.sh
  sudo bash add-google-cloud-ops-agent-repo.sh --also-install || true
  rm -f add-google-cloud-ops-agent-repo.sh
fi
sudo systemctl restart google-cloud-ops-agent 2>/dev/null || true

INSTANCE_ID="$(curl -sf -H 'Metadata-Flavor: Google' http://metadata.google.internal/computeMetadata/v1/instance/id || true)"

query_gcm_metrics() {
  local start_iso="$1"
  local end_iso="$2"
  local tag="$3"
  if [[ -z "$INSTANCE_ID" ]]; then
    return 0
  fi
  local tok
  tok="$(curl -sf -H 'Metadata-Flavor: Google' http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])' || true)"
  for metric in "compute.googleapis.com/instance/cpu/utilization" "agent.googleapis.com/memory/percent_used"; do
    local safe_m="${metric//\//_}"
    curl -sf -H "Authorization: Bearer ${tok}" \
      --get "https://monitoring.googleapis.com/v3/projects/${PROJECT}/timeSeries" \
      --data-urlencode "filter=metric.type=\"${metric}\" AND resource.labels.instance_id=\"${INSTANCE_ID}\"" \
      --data-urlencode "interval.startTime=${start_iso}" \
      --data-urlencode "interval.endTime=${end_iso}" \
      > "$OUT/gcm_${tag}_${safe_m}.json" || true
  done
}

verify_gcm_metrics_ready() {
  if [[ -z "$INSTANCE_ID" ]]; then
    return 0
  fi
  echo "=== Verifying Cloud Monitoring emission for instance_id=${INSTANCE_ID} ==="
  for attempt in $(seq 1 24); do
    local start_iso end_iso
    start_iso="$(date -u -d '15 minutes ago' +%Y-%m-%dT%H:%M:%SZ)"
    end_iso="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    query_gcm_metrics "$start_iso" "$end_iso" "preflight"
    local cpu_ok mem_ok
    cpu_ok="$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(len(d.get("timeSeries",[])))' "$OUT/gcm_preflight_compute.googleapis.com_instance_cpu_utilization.json" 2>/dev/null || echo 0)"
    mem_ok="$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(len(d.get("timeSeries",[])))' "$OUT/gcm_preflight_agent.googleapis.com_memory_percent_used.json" 2>/dev/null || echo 0)"
    echo "  [attempt ${attempt}/24] cpu_series=${cpu_ok} mem_series=${mem_ok}"
    if [[ "$cpu_ok" -gt 0 && "$mem_ok" -gt 0 ]]; then
      echo "=== Verified BOTH compute.googleapis.com/instance/cpu/utilization AND agent.googleapis.com/memory/percent_used are actively emitted! ==="
      return 0
    fi
    sleep 10
  done
  echo "WARNING: Timed out waiting for preflight Cloud Monitoring series; proceeding with benchmark run."
}

ROOT="$(cd "$(dirname "$0")" && pwd)"
if [[ ! -d "$ROOT/gcs_uds" && -f /opt/poc5/vm_uds5_worklog.sh ]]; then
  ROOT="/opt/poc5"
elif [[ ! -d "$ROOT/gcs_uds" ]]; then
  mkdir -p /opt/poc5
  cd /opt/poc5
  gcloud storage cp "gs://${BUCKET}/poc/poc_uds5.tgz" /tmp/poc_uds5.tgz || gsutil -q cp "gs://${BUCKET}/poc/poc_uds5.tgz" /tmp/poc_uds5.tgz
  tar -xzf /tmp/poc_uds5.tgz
  ROOT="/opt/poc5"
fi
if [[ ! -x "$ROOT/gcs_uds/server/target/release/gcs-core-server" ]]; then
  cd "$ROOT/gcs_uds/server"
  RUSTFLAGS="--cfg google_cloud_unstable_storage_bidi" cargo build --release
fi
export GCS_CORE_SERVER="$ROOT/gcs_uds/server/target/release/gcs-core-server"

cd "$ROOT/gothin"
if [[ ! -x bin/rapid_bench || ! -x bin/bench2_native || ! -x bin/bench2_uds ]]; then
  mkdir -p bin
  CGO_ENABLED=0 go build -o bin/bench2_native ./cmd/bench2_native
  CGO_ENABLED=0 go build -o bin/bench2_uds    ./cmd/bench2_uds
  CGO_ENABLED=0 go build -o bin/rapid_bench   ./cmd/rapid_bench
fi

verify_gcm_metrics_ready
sync_out

stage "seed-objects"
echo "=== 0. Seed 200MiB & 100MiB Rapid objects + 100 list objects ==="
./bin/rapid_bench prep -bucket="$BUCKET" -prefix=rapid
sync_out

stage "w1236-rapid-and-list"
echo "=== 1. Workloads 1-3 (Open, Read50x2x4, Random8K) & Workload 6 (List 100 objects & buckets) ==="
for r in 1 2; do
  ./bin/rapid_bench native -bucket="$BUCKET" -project="$PROJECT" -iters="$ITERS" \
    | tee "$OUT/rapid_w1236_native_grpc_r${r}.txt"
  ./bin/rapid_bench native-json -bucket="$BUCKET" -project="$PROJECT" -iters="$ITERS" \
    | tee "$OUT/rapid_w1236_native_json_r${r}.txt"
  pkill -x gcs-core-server 2>/dev/null || true
  UDS_SOCKET=/tmp/rapid.sock UDS_ZC=1 UDS_TRANSPORT=grpc ./bin/rapid_bench uds -bucket="$BUCKET" -project="$PROJECT" -iters="$ITERS" \
    | tee "$OUT/rapid_w1236_uds_grpc_r${r}.txt"
  pkill -x gcs-core-server 2>/dev/null || true
  UDS_SOCKET=/tmp/rapidj.sock UDS_ZC=1 UDS_TRANSPORT=json ./bin/rapid_bench uds -bucket="$BUCKET" -project="$PROJECT" -iters="$ITERS" \
    | tee "$OUT/rapid_w1236_uds_json_r${r}.txt"
  sync_out
done

stage "w4-bidiread48-native"
echo "=== 2. Workload 4: Read 16 MiB chunks of 200 MiB via BidiRead on ${SUSTAINED_THREADS} threads for ${SUSTAINED_DUR} ==="
W4_NAT_START="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
./bin/rapid_bench native -bucket="$BUCKET" -sli=bidiread48 \
  -sustained-threads="$SUSTAINED_THREADS" -sustained-dur="$SUSTAINED_DUR" \
  | tee "$OUT/w4_bidiread48_native_grpc.txt"
W4_NAT_END="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
sync_out

stage "w4-bidiread48-uds"
pkill -x gcs-core-server 2>/dev/null || true
W4_UDS_START="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
UDS_SOCKET=/tmp/w4.sock UDS_ZC=1 UDS_CONNS=32 ./bin/rapid_bench uds -bucket="$BUCKET" -sli=bidiread48 \
  -sustained-threads="$SUSTAINED_THREADS" -sustained-dur="$SUSTAINED_DUR" \
  | tee "$OUT/w4_bidiread48_uds_grpc.txt"
W4_UDS_END="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
sync_out

stage "w5-bidiwrite48-native"
echo "=== 3. Workload 5: Write 200 MiB objects via BidiWrite/WriteObject on ${SUSTAINED_THREADS} threads for ${SUSTAINED_DUR} ==="
W5_NAT_START="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
./bin/rapid_bench native -bucket="$BUCKET" -sli=bidiwrite48 \
  -sustained-threads="$SUSTAINED_THREADS" -sustained-dur="$SUSTAINED_DUR" \
  | tee "$OUT/w5_bidiwrite48_native_grpc.txt"
W5_NAT_END="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
sync_out

stage "w5-bidiwrite48-uds"
pkill -x gcs-core-server 2>/dev/null || true
W5_UDS_START="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
UDS_SOCKET=/tmp/w5.sock UDS_ZC=1 UDS_CONNS=32 UDS_WRITE_TRANSPORT="${UDS_WRITE_TRANSPORT:-grpc}" \
  ./bin/rapid_bench uds -bucket="$BUCKET" -sli=bidiwrite48 \
  -sustained-threads="$SUSTAINED_THREADS" -sustained-dur="$SUSTAINED_DUR" \
  | tee "$OUT/w5_bidiwrite48_uds.txt"
W5_UDS_END="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
sync_out

stage "w1r3-and-shm"
echo "=== 4. W1R3 Sequential & Shm Comparison (with tuned HTTP/2 windows & 8 MiB UDS buffers) ==="
NATIVE_HTTP1=1 ./bin/bench2_native -bucket="$BUCKET" -sizes=1MiB,16MiB,128MiB -iters="$ITERS" -list-max=100 \
  | tee "$OUT/w1r3_native_h1.txt"
NATIVE_GRPC=1 ./bin/bench2_native -bucket="$BUCKET" -sizes=1MiB,16MiB,128MiB -iters="$ITERS" -list-max=100 \
  | tee "$OUT/w1r3_native_grpc.txt"

pkill -x gcs-core-server 2>/dev/null || true
UDS_TRANSPORT=json ./bin/bench2_uds -bucket="$BUCKET" -sizes=1MiB,16MiB,128MiB -iters="$ITERS" -list-max=100 \
  | tee "$OUT/w1r3_uds_json_tuned.txt"
pkill -x gcs-core-server 2>/dev/null || true
UDS_TRANSPORT=grpc UDS_WRITE_TRANSPORT=grpc ./bin/bench2_uds -bucket="$BUCKET" -sizes=1MiB,16MiB,128MiB -iters="$ITERS" -list-max=100 \
  | tee "$OUT/w1r3_uds_grpc_tuned.txt"

pkill -x gcs-core-server 2>/dev/null || true
BUCKET="$BUCKET" ITERS="$ITERS" UDS_SOCKET=/tmp/shm.sock UDS_TRANSPORT=json ./bin/bench2_uds shm \
  | tee "$OUT/shm_json_tuned.txt"
pkill -x gcs-core-server 2>/dev/null || true
BUCKET="$BUCKET" ITERS="$ITERS" UDS_SOCKET=/tmp/shm2.sock UDS_TRANSPORT=grpc ./bin/bench2_uds shm \
  | tee "$OUT/shm_grpc_tuned.txt"
sync_out

stage "export-gcm-metrics"
echo "=== 5. Exporting Cloud Monitoring Metrics (compute.googleapis.com/instance/cpu/utilization & agent.googleapis.com/memory/percent_used) ==="
sleep 75
NOW_ISO="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
query_gcm_metrics "$W4_NAT_START" "$W4_NAT_END" "w4_native"
query_gcm_metrics "$W4_UDS_START" "$W4_UDS_END" "w4_uds"
query_gcm_metrics "$W5_NAT_START" "$W5_NAT_END" "w5_native"
query_gcm_metrics "$W5_UDS_START" "$W5_UDS_END" "w5_uds"
query_gcm_metrics "$W4_NAT_START" "$NOW_ISO" "full_run"
sync_out
stage "DONE"

echo "=== DONE: results in $OUT and $RES ==="
