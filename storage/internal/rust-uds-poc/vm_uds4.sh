#!/bin/bash
# Re-run of the UDS sequential cells after the zero-copy reader EOF fix (32k style).
set -x
BUCKET_REGIONAL=cpriti-thickthin-poc-asse1
ZONE=$(curl -s -H Metadata-Flavor:Google http://metadata.google.internal/computeMetadata/v1/instance/zone | awk -F/ '{print $NF}')
RES=gs://$BUCKET_REGIONAL/results/uds4-$ZONE; OUT=/var/log/bench-uds4; mkdir -p $OUT
exec > >(tee -a $OUT/startup.log) 2>&1
stage() { echo "=== STAGE: $1 $(date -u) ==="; echo "$1" > $OUT/STAGE; gsutil -q cp $OUT/STAGE $RES/STAGE || true; }
run() { local name=$1; shift; echo "##### $name $(date -u)"; "$@" 2>&1 | tee $OUT/$name.txt; gsutil -q cp $OUT/$name.txt $RES/ || true; }
export HOME=/root PATH=/root/.cargo/bin:/usr/local/go/bin:$PATH GOFLAGS=-mod=mod GOPATH=/root/go GOCACHE=/root/.cache/go-build
pkill -f gcs-core-server; sleep 1
stage "fetch-build"
cd /opt/poc2 && gsutil -q cp gs://$BUCKET_REGIONAL/poc/poc_uds4.tgz /tmp/p4.tgz && tar -xzf /tmp/p4.tgz
export GCS_CORE_SERVER=/opt/poc2/gcs_uds/server/target/release/gcs-core-server
cd /opt/poc2/gothin && cp /tmp/musl-server udsthin/bin/gcs-core-server-linux-amd64 2>/dev/null; CGO_ENABLED=0 go build -o bin/bench2_uds ./cmd/bench2_uds
B=$BUCKET_REGIONAL; L=regional; SIZES=1MiB,16MiB,128MiB,512MiB; ITERS=10
for round in 1 2; do
  stage "uds-seq-r$round"
  run ${L}_seq_uds_json_r$round       env UDS_TRANSPORT=json ./bin/bench2_uds -bucket $B -sizes $SIZES -iters $ITERS -list-iters 10
  run ${L}_seq_uds_json_proto_r$round env UDS_TRANSPORT=json UDS_ZC=0 ./bin/bench2_uds -bucket $B -sizes $SIZES -iters $ITERS -list-iters 10
  run ${L}_seq_uds_grpc_r$round       env UDS_TRANSPORT=grpc UDS_WRITE_TRANSPORT=json ./bin/bench2_uds -bucket $B -sizes $SIZES -iters $ITERS -list-iters 10
done
stage "conns-sweep"
threads() { local name=$1; shift; "$@" > $OUT/$name.txt 2>&1 & local pid=$!; local max=0; local smax=0
  while kill -0 $pid 2>/dev/null; do local t=$(ls /proc/$pid/task 2>/dev/null | wc -l); [ "$t" -gt "$max" ] && max=$t
    local sp=$(pgrep -n -f gcs-core-server); if [ -n "$sp" ]; then local st=$(ls /proc/$sp/task 2>/dev/null | wc -l); [ "$st" -gt "$smax" ] && smax=$st; fi; sleep 0.2; done; wait $pid
  echo "max_os_threads_go=$max max_os_threads_sidecar=$smax" | tee -a $OUT/$name.txt; cat $OUT/$name.txt; gsutil -q cp $OUT/$name.txt $RES/ || true; }
for par in 64 256; do
  for conns in 4 16 64 256; do
    threads ${L}_par${par}_conns${conns}_uds_json env UDS_TRANSPORT=json UDS_CONNS=$conns ./bin/bench2_uds -bucket $B -sizes 16MiB -iters 256 -parallel $par -list-iters 0 -styles whole
  done
  threads ${L}_par${par}_native_h1_again env NATIVE_HTTP1=1 /opt/poc2/gothin/bin/bench2_native -bucket $B -sizes 16MiB -iters 256 -parallel $par -list-iters 0 -styles whole
done
stage "upload"; gsutil -q cp -r $OUT $RES/; echo DONE > $OUT/STAGE; gsutil -q cp $OUT/STAGE $RES/STAGE
