#!/bin/bash
# Packaging & deployment experiments for the Rust sidecar. Run on the GCE VM
# (Debian 12) as root. Every experiment prints PASS/FAIL plus the raw error so
# the evaluation can quote it verbatim. Nothing here needs network except step 0.
set -u
OUT=${OUT:-/var/log/bench-uds}
mkdir -p $OUT
SRV_GLIBC=${SRV_GLIBC:-/opt/poc/gcs_uds/server/target/release/gcs-core-server}
SRV_MUSL=${SRV_MUSL:-/opt/poc/gcs_uds/server/target/x86_64-unknown-linux-musl/release/gcs-core-server}
GOBIN=${GOBIN:-/opt/poc/gothin/bin/bench2_uds}
log() { echo "### $*"; }
res() { echo "RESULT $1: $2"; }

log "0. artifact sizes and link types"
for f in $SRV_GLIBC $SRV_MUSL $GOBIN /opt/poc/gothin/bin/bench2_native; do
  [ -f "$f" ] && { ls -la $f; file $f | sed 's/,.*BuildID.*//' ; (ldd $f 2>&1 | head -5); strip -o /tmp/stripped $f 2>/dev/null && echo "stripped: $(stat -c %s /tmp/stripped) bytes"; }
done

log "1. glibc symbol requirement of the glibc build"
objdump -T $SRV_GLIBC 2>/dev/null | grep -o 'GLIBC_[0-9.]*' | sort -Vu | tail -3

log "2. FROM scratch emulation: run each server inside an empty chroot (no libc, no /etc/ssl, no /tmp)"
for v in glibc musl; do
  f=$SRV_GLIBC; [ $v = musl ] && f=$SRV_MUSL
  [ -f "$f" ] || { res "scratch-$v" "SKIP (no binary)"; continue; }
  R=/tmp/scratch-$v; rm -rf $R; mkdir -p $R/tmp $R/run
  cp $f $R/server
  # metadata server creds need network + DNS; we only test process start + UDS bind here
  (cd $R && timeout 5 chroot $R /server --socket /run/core.sock --idle-secs 2 --peer-uid-check=false > $R/out.txt 2>&1; echo "exit=$?" >> $R/out.txt)
  head -3 $R/out.txt
  if grep -q READY $R/out.txt; then res "scratch-$v" "PASS (binds UDS inside empty chroot)"; elif [ $v = glibc ]; then res "scratch-$v" "FAIL as expected (dynamic loader /lib64/ld-linux missing in FROM scratch): $(head -1 $R/out.txt)"; else res "scratch-$v" "FAIL: $(head -1 $R/out.txt)"; fi
done

log "3. noexec: extract embedded binary to a noexec tmpfs and exec it (what //go:embed + exec would do)"
mkdir -p /mnt/noexec && mount -t tmpfs -o noexec,size=64m tmpfs /mnt/noexec && cp $SRV_MUSL /mnt/noexec/server && chmod +x /mnt/noexec/server
/mnt/noexec/server --version > /tmp/noexec.txt 2>&1; rc=$?
cat /tmp/noexec.txt; res "noexec-exec" "$([ $rc -ne 0 ] && echo "FAIL as expected (rc=$rc): $(cat /tmp/noexec.txt)" || echo UNEXPECTED-PASS)"
log "3b. memfd_create + fexecve fallback (no filesystem involved)"
cat > /tmp/memfd.py <<'EOF'
import os, sys
data = open(sys.argv[1], 'rb').read()
fd = os.memfd_create('gcs-core', 0)
os.write(fd, data)
try:
    os.execve(f'/proc/self/fd/{fd}', ['gcs-core-server', '--version'], os.environ)
except Exception as e:
    print('memfd exec failed:', e); sys.exit(1)
EOF
python3 /tmp/memfd.py $SRV_MUSL > /tmp/memfd.txt 2>&1; rc=$?; cat /tmp/memfd.txt; res "memfd-exec" "$([ $rc -eq 0 ] && echo "PASS (works on this kernel; blocked where vm.memfd_noexec=2 / seccomp MDWE)" || echo "FAIL rc=$rc")"
echo "vm.memfd_noexec=$(sysctl -n vm.memfd_noexec 2>/dev/null || echo n/a) kernel=$(uname -r)"
umount /mnt/noexec

log "4. read-only root emulation: socket dir on a read-only mount"
mkdir -p /mnt/ro && mount -t tmpfs tmpfs /mnt/ro && mkdir -p /mnt/ro/run && mount -o remount,ro /mnt/ro
timeout 3 $SRV_GLIBC --socket /mnt/ro/run/core.sock --idle-secs 1 > /tmp/ro.txt 2>&1; rc=$?; head -2 /tmp/ro.txt
res "readonly-socket-dir" "$([ $rc -ne 0 ] && echo "FAIL as expected: $(grep -m1 -i 'error\|denied\|read-only' /tmp/ro.txt)" || echo UNEXPECTED-PASS)"
log "4b. abstract socket needs no filesystem"
timeout 3 $SRV_GLIBC --socket @gcs-core-test --idle-secs 1 > /tmp/abs.txt 2>&1; grep -q READY /tmp/abs.txt && res "abstract-socket" "PASS (Linux only; visible to every process in the netns, hence the peer-UID check)" || res "abstract-socket" "FAIL: $(head -1 /tmp/abs.txt)"
umount /mnt/ro 2>/dev/null

log "5. UDS path length limit (sun_path 108 bytes on Linux, 104 on macOS)"
LONG=/tmp/$(printf 'x%.0s' $(seq 1 100))/core.sock; mkdir -p $(dirname $LONG)
timeout 3 $SRV_GLIBC --socket $LONG --idle-secs 1 > /tmp/long.txt 2>&1; rc=$?; echo "path length=${#LONG}"; head -2 /tmp/long.txt
res "uds-path-108" "$([ $rc -ne 0 ] && echo "FAIL as expected: $(head -1 /tmp/long.txt)" || echo UNEXPECTED-PASS)"

log "6. peer-UID check: connect as another user"
$SRV_GLIBC --socket /tmp/peer.sock --idle-secs 30 > /tmp/peer.txt 2>&1 &
sleep 1; chmod 666 /tmp/peer.sock
if id nobody >/dev/null 2>&1; then
  su -s /bin/bash nobody -c "GCS_CORE_SERVER=/nonexistent UDS_SOCKET=/tmp/peer.sock $GOBIN ping" > /tmp/peer-nobody.txt 2>&1; head -3 /tmp/peer-nobody.txt
  grep -qi 'permission\|denied' /tmp/peer-nobody.txt && res "peer-uid" "PASS (other uid rejected)" || res "peer-uid" "CHECK: $(head -1 /tmp/peer-nobody.txt)"
fi
pkill -f 'peer.sock'; sleep 1

log "7. orphan handling: kill -9 the Go parent, sidecar should exit via PR_SET_PDEATHSIG"
rm -f /tmp/orphan.sock
( GCS_CORE_SERVER=$SRV_GLIBC UDS_SOCKET=/tmp/orphan.sock $GOBIN -sizes 16MiB -iters 100 -list-iters 0 -styles whole > /tmp/orphan-go.txt 2>&1 & echo $! > /tmp/orphan.pid )
sleep 4; GOPID=$(cat /tmp/orphan.pid); SIDE=$(pgrep -f 'orphan.sock' | grep -v $GOPID | head -1)
echo "go pid=$GOPID sidecar pid=$SIDE"; kill -9 $GOPID; sleep 2
if [ -n "$SIDE" ] && kill -0 $SIDE 2>/dev/null; then res "orphan-pdeathsig" "FAIL: sidecar $SIDE still alive after parent SIGKILL"; kill $SIDE; else res "orphan-pdeathsig" "PASS (sidecar exited with parent)"; fi
ls -la /tmp/orphan.sock 2>&1 | head -1

log "8. stale socket: leave a dead socket file, start again"
python3 - <<'EOF'
import socket,os
p='/tmp/stale.sock'
try: os.unlink(p)
except FileNotFoundError: pass
s=socket.socket(socket.AF_UNIX); s.bind(p); s.close()  # file exists, nobody listening
EOF
timeout 3 $SRV_GLIBC --socket /tmp/stale.sock --idle-secs 1 > /tmp/stale.txt 2>&1; grep -q READY /tmp/stale.txt && res "stale-socket" "PASS (removed stale file: $(grep -m1 stale /tmp/stale.txt))" || res "stale-socket" "FAIL: $(head -1 /tmp/stale.txt)"

log "9. PID 1 semantics: Go app as init in a new pid namespace, SIGTERM to the app"
rm -f /tmp/pid1.sock
timeout 20 unshare --pid --fork --mount-proc bash -c "GCS_CORE_SERVER=$SRV_GLIBC UDS_SOCKET=/tmp/pid1.sock $GOBIN -sizes 16MiB -iters 50 -list-iters 0 -styles whole > /tmp/pid1-go.txt 2>&1 & sleep 4; ps -eo pid,ppid,comm; kill -TERM \$!; sleep 2; ps -eo pid,ppid,comm" > /tmp/pid1.txt 2>&1
cat /tmp/pid1.txt | tail -12
if grep -q '<defunct>' /tmp/pid1.txt; then res "pid1-zombie" "FAIL: zombie sidecar under PID 1 Go app"; else res "pid1-zombie" "PASS/INFO (see ps output above)"; fi

log "10. idle exit"
rm -f /tmp/idle.sock; timeout 15 $SRV_GLIBC --socket /tmp/idle.sock --idle-secs 2 > /tmp/idle.txt 2>&1; rc=$?
grep -q idle /tmp/idle.txt && res "idle-exit" "PASS (exited after 2s idle, rc=$rc)" || res "idle-exit" "FAIL rc=$rc"

log "11. Go thin client cross-compiles with CGO_ENABLED=0 (the whole point)"
cd /opt/poc/gothin && for t in linux/arm64 darwin/arm64 windows/amd64 linux/s390x freebsd/amd64; do
  GOOS=${t%/*} GOARCH=${t#*/} CGO_ENABLED=0 go build -o /tmp/xc-$(echo $t|tr / -) ./cmd/bench2_uds 2>&1 | head -2 && echo "cross $t: ok $(stat -c %s /tmp/xc-$(echo $t|tr / -)) bytes"
done

log "12. Rust server cross-compile needs per-target toolchains (same as FFI case)"
cd /opt/poc/gcs_uds/server && export PATH=/root/.cargo/bin:$PATH && rustup target add aarch64-unknown-linux-gnu >/dev/null 2>&1; cargo build --release --target aarch64-unknown-linux-gnu 2>&1 | grep -E 'error|failed|Finished' | head -3

log "13. //go:embed size impact: module zip with 6 platform binaries"
mkdir -p /tmp/embedtest && cd /tmp/embedtest && for p in linux-amd64 linux-arm64 linux-musl-amd64 darwin-amd64 darwin-arm64 windows-amd64; do cp /tmp/stripped bin-$p 2>/dev/null || cp $SRV_MUSL bin-$p; done
tar czf module.zip bin-*; echo "6 platform binaries (stripped musl size each): $(stat -c %s module.zip) bytes compressed; single platform: $(stat -c %s bin-linux-amd64) bytes"

cp /tmp/*.txt $OUT/ 2>/dev/null
echo "=== packaging tests done ==="
