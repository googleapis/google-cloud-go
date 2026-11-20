#!/bin/bash
# Follow-up cells: shared-memory data plane (JSON + gRPC read paths), ping. Reuses the v2 build tree.
set -x
md() { curl -sf -H Metadata-Flavor:Google "http://metadata.google.internal/computeMetadata/v1/instance/attributes/$1" 2>/dev/null; }
BUCKET_REGIONAL=cpriti-thickthin-poc-asse1; BUCKET_ZONAL=$(md bucket-zonal); SKIP_REGIONAL=$(md skip-regional)
ZONE=$(curl -s -H Metadata-Flavor:Google http://metadata.google.internal/computeMetadata/v1/instance/zone | awk -F/ '{print $NF}')
RES=gs://$BUCKET_REGIONAL/results/uds3-$ZONE; OUT=/var/log/bench-uds3; mkdir -p $OUT
exec > >(tee -a $OUT/startup.log) 2>&1
stage() { echo "=== STAGE: $1 $(date -u) ==="; echo "$1" > $OUT/STAGE; gsutil -q cp $OUT/STAGE $RES/STAGE || true; }
run() { local name=$1; shift; echo "##### $name $(date -u)"; "$@" 2>&1 | tee $OUT/$name.txt; gsutil -q cp $OUT/$name.txt $RES/ || true; }
export HOME=/root PATH=/root/.cargo/bin:/usr/local/go/bin:$PATH GOFLAGS=-mod=mod GOPATH=/root/go GOCACHE=/root/.cache/go-build
pkill -f gcs-core-server; sleep 1
stage "fetch-build"
cd /opt/poc2 && gsutil -q cp gs://$BUCKET_REGIONAL/poc/poc_uds3.tgz /tmp/p3.tgz && tar -xzf /tmp/p3.tgz
cd /opt/poc2/gcs_uds/server && cargo build --release 2>&1 | tail -1
export GCS_CORE_SERVER=/opt/poc2/gcs_uds/server/target/release/gcs-core-server
cd /opt/poc2/gothin && cp /tmp/musl-server udsthin/bin/gcs-core-server-linux-amd64 2>/dev/null; CGO_ENABLED=0 go build -o bin/bench2_uds ./cmd/bench2_uds
B=$BUCKET_REGIONAL; L=regional; [ -n "$SKIP_REGIONAL" ] && { B=$BUCKET_ZONAL; L=zonal; }
stage "shm-$L"
for r in 1 2; do
  run ${L}_shm_json_r$r env BUCKET=$B ITERS=10 UDS_SOCKET=/tmp/shm$r.sock ./bin/bench2_uds shm
  run ${L}_shm_grpc_r$r env BUCKET=$B ITERS=10 UDS_TRANSPORT=grpc UDS_SOCKET=/tmp/shmg$r.sock ./bin/bench2_uds shm
done
run ping env UDS_SOCKET=/tmp/ping3.sock UDS_TCP=127.0.0.1:50052 ./bin/bench2_uds ping
run check_grpc_errors env BUCKET=$B UDS_TRANSPORT=grpc ./bin/bench2_uds check
stage "scratch-retest"
# FROM scratch with CA certs present (what a real scratch image ships): does the static musl server start?
M=/opt/poc2/gcs_uds/server/target/x86_64-unknown-linux-musl/release/gcs-core-server
R=/tmp/scratch2; rm -rf $R; mkdir -p $R/tmp $R/run $R/etc/ssl/certs; cp $M $R/server; cp /etc/ssl/certs/ca-certificates.crt $R/etc/ssl/certs/; cp /etc/resolv.conf $R/etc/ 2>/dev/null
(cd $R && timeout 20 chroot $R /server --socket /run/core.sock --idle-secs 3 --peer-uid-check false > $R/out.txt 2>&1; echo "exit=$?" >> $R/out.txt)
cat $R/out.txt | head -5 | tee $OUT/scratch_retest.txt; gsutil -q cp $OUT/scratch_retest.txt $RES/
rm -rf $R; mkdir -p $R/tmp $R/run; cp $M $R/server
(cd $R && timeout 20 chroot $R /server --socket /run/core.sock --idle-secs 3 --peer-uid-check false > $R/out2.txt 2>&1; echo "exit=$?" >> $R/out2.txt)
echo "--- without CA certs:" | tee -a $OUT/scratch_retest.txt; head -4 $R/out2.txt | tee -a $OUT/scratch_retest.txt; gsutil -q cp $OUT/scratch_retest.txt $RES/
stage "upload"; gsutil -q cp -r $OUT $RES/; echo DONE > $OUT/STAGE; gsutil -q cp $OUT/STAGE $RES/STAGE
