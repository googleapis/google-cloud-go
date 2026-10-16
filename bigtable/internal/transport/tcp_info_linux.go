// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux

package internal

import (
	"errors"
	"net"
	"time"

	"golang.org/x/sys/unix"
)

// tcpStateName maps the linux tcp_info state byte to a short human label.
// Numbers come from include/net/tcp_states.h; TCP_ESTABLISHED=1, etc.
var tcpStateName = [...]string{
	1:  "ESTABLISHED",
	2:  "SYN_SENT",
	3:  "SYN_RECV",
	4:  "FIN_WAIT1",
	5:  "FIN_WAIT2",
	6:  "TIME_WAIT",
	7:  "CLOSE",
	8:  "CLOSE_WAIT",
	9:  "LAST_ACK",
	10: "LISTEN",
	11: "CLOSING",
	12: "NEW_SYN_RECV",
}

// caStateName maps tcp_info's ca_state byte to the short CA-state label.
// Values from include/uapi/linux/tcp.h:
//
//	Open     — no loss, normal operation
//	Disorder — got a dupACK or SACK, kernel is watching for real loss
//	CWR      — cwnd reduced by ECN CE mark; still cwr'ing
//	Recovery — fast retransmit in progress
//	Loss     — RTO fired (worst); cwnd collapsed to 1 MSS
var caStateName = [...]string{
	0: "Open",
	1: "Disorder",
	2: "CWR",
	3: "Recovery",
	4: "Loss",
}

// readTCPInfo pulls struct tcp_info from the socket via
// getsockopt(SOL_TCP, TCP_INFO). This is a read-only kernel syscall — it
// never blocks on the network, never touches the socket's data path, and
// never takes any userspace lock. Cost is sub-microsecond.
func readTCPInfo(c *net.TCPConn) (TCPInfoSnapshot, error) {
	if c == nil {
		return TCPInfoSnapshot{}, errors.New("nil *net.TCPConn")
	}
	raw, err := c.SyscallConn()
	if err != nil {
		return TCPInfoSnapshot{}, annotateReadErr("SyscallConn", err)
	}
	var info *unix.TCPInfo
	var soErr error
	ctrlErr := raw.Control(func(fd uintptr) {
		info, soErr = unix.GetsockoptTCPInfo(int(fd), unix.IPPROTO_TCP, unix.TCP_INFO)
	})
	if ctrlErr != nil {
		return TCPInfoSnapshot{}, annotateReadErr("raw.Control", ctrlErr)
	}
	if soErr != nil {
		return TCPInfoSnapshot{}, annotateReadErr("getsockopt(TCP_INFO)", soErr)
	}
	return tcpInfoToSnapshot(info), nil
}

// tcpInfoToSnapshot converts the kernel struct into our platform-agnostic
// snapshot. Kernel unit conventions:
//   - Rtt / Rttvar / Min_rtt / Rto / Ato : microseconds
//   - Last_data_recv / Last_data_sent    : milliseconds
//   - Busy_time / Rwnd_limited / Sndbuf_limited : microseconds
//   - Total_rto_time                     : milliseconds
func tcpInfoToSnapshot(info *unix.TCPInfo) TCPInfoSnapshot {
	state := ""
	if int(info.State) < len(tcpStateName) {
		state = tcpStateName[info.State]
	}
	caState := ""
	if int(info.Ca_state) < len(caStateName) {
		caState = caStateName[info.Ca_state]
	}

	var retransRatio float64
	if info.Bytes_sent > 0 {
		retransRatio = float64(info.Bytes_retrans) / float64(info.Bytes_sent) * 100
	}

	return TCPInfoSnapshot{
		State:   state,
		CAState: caState,
		Backoff: uint32(info.Backoff),

		RTT:    time.Duration(info.Rtt) * time.Microsecond,
		RTTVar: time.Duration(info.Rttvar) * time.Microsecond,
		MinRTT: time.Duration(info.Min_rtt) * time.Microsecond,

		MSS:         info.Snd_mss,
		PMTU:        info.Pmtu,
		SndCwnd:     info.Snd_cwnd,
		SndSsthresh: info.Snd_ssthresh,
		SndWnd:      info.Snd_wnd,
		RcvWnd:      info.Rcv_wnd,

		Retransmits:  uint32(info.Retransmits),
		Retrans:      info.Retrans,
		TotalRetrans: info.Total_retrans,
		Lost:         info.Lost,
		Sacked:       info.Sacked,
		Unacked:      info.Unacked,
		Reordering:   info.Reordering,
		ReordSeen:    info.Reord_seen,
		DsackDups:    info.Dsack_dups,
		DeliveredCE:  info.Delivered_ce,
		RcvOooPack:   info.Rcv_ooopack,

		RTO:                time.Duration(info.Rto) * time.Microsecond,
		ATO:                time.Duration(info.Ato) * time.Microsecond,
		Probes:             uint32(info.Probes),
		TotalRTO:           uint32(info.Total_rto),
		TotalRTORecoveries: uint32(info.Total_rto_recoveries),
		TotalRTOTime:       time.Duration(info.Total_rto_time) * time.Millisecond,

		SegsOut:       info.Segs_out,
		SegsIn:        info.Segs_in,
		DataSegsOut:   info.Data_segs_out,
		DataSegsIn:    info.Data_segs_in,
		BytesSent:     info.Bytes_sent,
		BytesAcked:    info.Bytes_acked,
		BytesRetrans:  info.Bytes_retrans,
		BytesReceived: info.Bytes_received,
		Delivered:     info.Delivered,
		DeliveryRate:  info.Delivery_rate,
		PacingRate:    info.Pacing_rate,
		NotsentBytes:  info.Notsent_bytes,
		BusyTime:      time.Duration(info.Busy_time) * time.Microsecond,
		RwndLimited:   time.Duration(info.Rwnd_limited) * time.Microsecond,
		SndbufLimited: time.Duration(info.Sndbuf_limited) * time.Microsecond,
		Rehash:        info.Rehash,

		LastDataRecv: time.Duration(info.Last_data_recv) * time.Millisecond,
		LastDataSent: time.Duration(info.Last_data_sent) * time.Millisecond,

		RetransRatioPct: retransRatio,
	}
}

// isDeadConn recognizes the errno returned when the fd has been closed
// out from under us — gRPC dropped its ref, the kernel released the fd,
// and our getsockopt lost the race. Callers (Snapshot) treat these as
// "prune this entry" rather than "surface error."
func isDeadConn(err error) bool {
	return errors.Is(err, unix.EBADF) || errors.Is(err, unix.ENOTCONN) ||
		errors.Is(err, net.ErrClosed)
}
