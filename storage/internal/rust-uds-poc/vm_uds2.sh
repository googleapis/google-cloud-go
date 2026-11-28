#!/bin/bash
# v2 pass (after fairness review): native gRPC pool=NumCPU + BidiReads + metrics off;
# Rust single-RPC BidiRead; repeated-bytes chunks; zero-copy decode (default) vs proto decode;
# per-caller sessions; shared-memory data plane; embedded (musl) sidecar binary.
set -x
md() { curl -sf -H Metadata-Flavor:Google "http://metadata.google.internal/computeMetadata/v1/instance/attributes/$1" 2>/dev/null; }
BUCKET_REGIONAL=cpriti-thickthin-poc-asse1
BUCKET_ZONAL=$(md bucket-zonal); SKIP_REGIONAL=$(md skip-regional)
ZONE=$(curl -s -H Metadata-Flavor:Google http://metadata.google.internal/computeMetadata/v1/instance/zone | awk -F/ '{print $NF}')
RES=gs://$BUCKET_REGIONAL/results/uds2-$ZONE
OUT=/var/log/bench-uds2; mkdir -p $OUT
exec > >(tee -a $OUT/startup.log) 2>&1
echo "=== START v2 $(date -u) host=$(hostname) zone=$ZONE ==="
stage() { echo "=== STAGE: $1 $(date -u) ==="; echo "$1" > $OUT/STAGE; gsutil -q cp $OUT/STAGE $RES/STAGE || true; }
run() { local name=$1; shift; echo "##### $name $(date -u)"; "$@" 2>&1 | tee $OUT/$name.txt; gsutil -q cp $OUT/$name.txt $RES/ || true; }
export HOME=/root PATH=/root/.cargo/bin:/usr/local/go/bin:$PATH GOFLAGS=-mod=mod GOPATH=/root/go GOCACHE=/root/.cache/go-build
command -v cargo >/dev/null || { stage "FAILED-no-toolchain"; exit 1; }
pkill -f gcs-core-server; sleep 1

stage "fetch-build"
rm -rf /opt/poc2 && mkdir -p /opt/poc2 && cd /opt/poc2 && gsutil -q cp gs://$BUCKET_REGIONAL/poc/poc_uds2.tgz /tmp/p2.tgz && tar -xzf /tmp/p2.tgz
cd /opt/poc2/gcs_uds/server
# reuse the v1 target dir to avoid a cold build
[ -d /opt/poc/gcs_uds/server/target ] && cp -r /opt/poc/gcs_uds/server/target . 2>/dev/null
/usr/bin/time -f "rust glibc build: wall=%e s" cargo build --release 2>&1 | tail -2
CC_x86_64_unknown_linux_musl=musl-gcc /usr/bin/time -f "rust musl build: wall=%e s" cargo build --release --target x86_64-unknown-linux-musl 2>&1 | tail -2
ls -la target/release/gcs-core-server target/x86_64-unknown-linux-musl/release/gcs-core-server
export GCS_CORE_SERVER=/opt/poc2/gcs_uds/server/target/release/gcs-core-server
cd /opt/poc2/gothin && mkdir -p bin udsthin/bin
cp ../gcs_uds/server/target/x86_64-unknown-linux-musl/release/gcs-core-server udsthin/bin/gcs-core-server-linux-amd64
: > /tmp/empty; CGO_ENABLED=0 go build -o bin/bench2_uds ./cmd/bench2_uds
cp udsthin/bin/gcs-core-server-linux-amd64 /tmp/musl-server; : > udsthin/bin/gcs-core-server-linux-amd64
CGO_ENABLED=0 go build -o bin/bench2_uds_noembed ./cmd/bench2_uds; cp /tmp/musl-server udsthin/bin/gcs-core-server-linux-amd64
CGO_ENABLED=0 go build -o bin/bench2_native ./cmd/bench2_native
CGO_ENABLED=0 go build -o bin/rapid_bench ./cmd/rapid_bench
ls -la bin; echo "embedded size delta: $(( $(stat -c %s bin/bench2_uds) - $(stat -c %s bin/bench2_uds_noembed) )) bytes"

stage "checks"
B=$BUCKET_REGIONAL; [ -n "$SKIP_REGIONAL" ] && B=$BUCKET_ZONAL
run check_json      env BUCKET=$B ./bin/bench2_uds check
run check_grpc      env BUCKET=$B UDS_TRANSPORT=grpc ./bin/bench2_uds check
run check_embedded  env BUCKET=$B UDS_EMBEDDED=1 UDS_SOCKET=/tmp/emb.sock GCS_CORE_SERVER=/nonexistent ./bin/bench2_uds check
run session_tests   env BUCKET=$B UDS_SESSION=1 UDS_SOCKET=/tmp/sess.sock ./bin/bench2_uds session-tests
run ping            env UDS_SOCKET=/tmp/ping.sock UDS_TCP=127.0.0.1:50051 ./bin/bench2_uds ping
run shm_json        env BUCKET=$B ITERS=10 UDS_SOCKET=/tmp/shm.sock ./bin/bench2_uds shm
run shm_grpc        env BUCKET=$B ITERS=10 UDS_TRANSPORT=grpc UDS_SOCKET=/tmp/shmg.sock ./bin/bench2_uds shm
pkill -f gcs-core-server; sleep 1

SIZES=1MiB,16MiB,128MiB,512MiB; ITERS=10
threads() { local name=$1; shift; "$@" > $OUT/$name.txt 2>&1 & local pid=$!; local max=0; local smax=0
  while kill -0 $pid 2>/dev/null; do local t=$(ls /proc/$pid/task 2>/dev/null | wc -l); [ "$t" -gt "$max" ] && max=$t
    local sp=$(pgrep -n -f gcs-core-server); if [ -n "$sp" ]; then local st=$(ls /proc/$sp/task 2>/dev/null | wc -l); [ "$st" -gt "$smax" ] && smax=$st; fi; sleep 0.2; done; wait $pid
  echo "max_os_threads_go=$max max_os_threads_sidecar=$smax" | tee -a $OUT/$name.txt; cat $OUT/$name.txt; gsutil -q cp $OUT/$name.txt $RES/ || true; }
matrix() { local B=$1 L=$2
  for round in 1 2; do
    stage "w1r3-$L-r$round"
    run ${L}_seq_native_h1_r$round     env NATIVE_HTTP1=1 ./bin/bench2_native -bucket $B -sizes $SIZES -iters $ITERS -list-iters 10
    run ${L}_seq_native_h2_r$round     ./bin/bench2_native -bucket $B -sizes $SIZES -iters $ITERS -list-iters 10
    run ${L}_seq_native_grpc_r$round   env NATIVE_GRPC=1 ./bin/bench2_native -bucket $B -sizes $SIZES -iters $ITERS -list-iters 10
    run ${L}_seq_uds_json_r$round      env UDS_TRANSPORT=json ./bin/bench2_uds -bucket $B -sizes $SIZES -iters $ITERS -list-iters 10
    run ${L}_seq_uds_json_proto_r$round env UDS_TRANSPORT=json UDS_ZC=0 ./bin/bench2_uds -bucket $B -sizes $SIZES -iters $ITERS -list-iters 0 -styles whole
    run ${L}_seq_uds_grpc_r$round      env UDS_TRANSPORT=grpc UDS_WRITE_TRANSPORT=json ./bin/bench2_uds -bucket $B -sizes $SIZES -iters $ITERS -list-iters 10
  done
  stage "parallel-$L"
  for par in 16 64 256; do
    threads ${L}_par${par}_native_h1   env NATIVE_HTTP1=1 ./bin/bench2_native -bucket $B -sizes 16MiB -iters 256 -parallel $par -list-iters 0 -styles whole
    threads ${L}_par${par}_native_grpc env NATIVE_GRPC=1 ./bin/bench2_native -bucket $B -sizes 16MiB -iters 256 -parallel $par -list-iters 0 -styles whole
    threads ${L}_par${par}_uds_json    env UDS_TRANSPORT=json ./bin/bench2_uds -bucket $B -sizes 16MiB -iters 256 -parallel $par -list-iters 0 -styles whole
    threads ${L}_par${par}_uds_grpc    env UDS_TRANSPORT=grpc UDS_WRITE_TRANSPORT=json ./bin/bench2_uds -bucket $B -sizes 16MiB -iters 256 -parallel $par -list-iters 0 -styles whole
  done
  stage "rapid-$L"
  ./bin/rapid_bench prep -bucket $B
  for round in 1 2; do
    run ${L}_rapid_native_grpc_r$round ./bin/rapid_bench native -bucket $B -iters 10
    run ${L}_rapid_uds_grpc_r$round    ./bin/rapid_bench uds -bucket $B -iters 10
    run ${L}_rapid_uds_grpc_proto_r$round env UDS_ZC=0 ./bin/rapid_bench uds -bucket $B -iters 10
    run ${L}_rapid_native_json_r$round ./bin/rapid_bench native-json -bucket $B -iters 5 -random-reads 100
  done
}
[ -z "$SKIP_REGIONAL" ] && matrix $BUCKET_REGIONAL regional
if [ -n "$BUCKET_ZONAL" ]; then
  matrix $BUCKET_ZONAL zonal
  stage "zonal-append-writes"
  for round in 1 2; do
    run zonal_seq_native_grpc_append_r$round env NATIVE_GRPC=1 NATIVE_APPEND=1 ./bin/bench2_native -bucket $BUCKET_ZONAL -sizes $SIZES -iters $ITERS -list-iters 0 -styles whole
    run zonal_seq_uds_grpc_append_r$round    env UDS_TRANSPORT=grpc UDS_WRITE_TRANSPORT=grpc ./bin/bench2_uds -bucket $BUCKET_ZONAL -sizes $SIZES -iters $ITERS -list-iters 0 -styles whole
  done
fi

stage "packaging"
pkill -f gcs-core-server; sleep 1
OUT=$OUT SRV_GLIBC=/opt/poc2/gcs_uds/server/target/release/gcs-core-server SRV_MUSL=/opt/poc2/gcs_uds/server/target/x86_64-unknown-linux-musl/release/gcs-core-server GOBIN=/opt/poc2/gothin/bin/bench2_uds_noembed bash /opt/poc2/gcs_uds/packaging_tests.sh 2>&1 | tee $OUT/packaging.txt
echo "### embedded binary on noexec /tmp: extract fails -> memfd fallback"
mkdir -p /mnt/noexec2 && mount -t tmpfs -o noexec,size=128m tmpfs /mnt/noexec2
XDG_CACHE_HOME=/mnt/noexec2 UDS_EMBEDDED=1 UDS_SOCKET=/tmp/emb2.sock GCS_CORE_SERVER=/nonexistent BUCKET=$B ./bin/bench2_uds -sizes 1MiB -iters 1 -list-iters 0 -styles whole 2>&1 | head -4 | tee -a $OUT/packaging.txt
umount /mnt/noexec2; gsutil -q cp $OUT/packaging.txt $RES/

stage "upload"
gsutil -q cp -r $OUT $RES/
echo DONE > $OUT/STAGE; gsutil -q cp $OUT/STAGE $RES/STAGE
echo "=== DONE v2 $(date -u) ==="
