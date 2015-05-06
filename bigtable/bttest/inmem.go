/*
Copyright 2015 Google Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

/*
Package bttest contains test helpers for working with the bigtable package.

To use a Server, create it, and then connect to it with no security:
(The project/zone/cluster values are ignored.)
	srv, err := bttest.NewServer()
	...
	client, err := bigtable.NewClient(ctx, proj, zone, cluster,
		bigtable.WithCredentials(nil), bigtable.WithInsecureAddr(srv.Addr))
	...
*/
package bttest // import "google.golang.org/cloud/bigtable/bttest"

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"regexp"
	"sort"
	"strings"
	"sync"

	"golang.org/x/net/context"
	btdpb "google.golang.org/cloud/bigtable/internal/data_proto"
	emptypb "google.golang.org/cloud/bigtable/internal/empty"
	btspb "google.golang.org/cloud/bigtable/internal/service_proto"
	bttdpb "google.golang.org/cloud/bigtable/internal/table_data_proto"
	bttspb "google.golang.org/cloud/bigtable/internal/table_service_proto"
	"google.golang.org/grpc"
)

// Server is an in-memory Cloud Bigtable fake.
// It is unauthenticated, and only a rough approximation.
type Server struct {
	Addr string

	l   net.Listener
	srv *grpc.Server
	s   *server
}

// server is the real implementation of the fake.
// It is a separate and unexported type so the API won't be cluttered with
// methods that are only relevant to the fake's implementation.
type server struct {
	mu     sync.Mutex
	tables map[string]*table // keyed by fully qualified name

	// Any unimplemented methods will cause a panic.
	bttspb.BigtableTableServiceServer
	btspb.BigtableServiceServer
}

// NewServer creates a new Server. The Server will be listening for gRPC connections
// at the address named by the Addr field, without TLS.
func NewServer() (*Server, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	s := &Server{
		Addr: l.Addr().String(),
		l:    l,
		srv:  grpc.NewServer(),
		s: &server{
			tables: make(map[string]*table),
		},
	}
	bttspb.RegisterBigtableTableServiceServer(s.srv, s.s)
	btspb.RegisterBigtableServiceServer(s.srv, s.s)

	go s.srv.Serve(s.l)

	return s, nil
}

// Close shuts down the server.
func (s *Server) Close() {
	s.srv.Stop()
	s.l.Close()
}

func (s *server) CreateTable(ctx context.Context, req *bttspb.CreateTableRequest) (*bttdpb.Table, error) {
	tbl := req.Name + "/tables/" + req.TableId

	s.mu.Lock()
	if _, ok := s.tables[tbl]; ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("table %q already exists", tbl)
	}
	s.tables[tbl] = newTable()
	s.mu.Unlock()

	return &bttdpb.Table{Name: tbl}, nil
}

func (s *server) ListTables(ctx context.Context, req *bttspb.ListTablesRequest) (*bttspb.ListTablesResponse, error) {
	res := &bttspb.ListTablesResponse{}
	prefix := req.Name + "/tables/"

	s.mu.Lock()
	for tbl := range s.tables {
		if strings.HasPrefix(tbl, prefix) {
			res.Tables = append(res.Tables, &bttdpb.Table{Name: tbl})
		}
	}
	s.mu.Unlock()

	return res, nil
}

func (s *server) DeleteTable(ctx context.Context, req *bttspb.DeleteTableRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tables[req.Name]; !ok {
		return nil, fmt.Errorf("no such table %q", req.Name)
	}
	delete(s.tables, req.Name)
	return &emptypb.Empty{}, nil
}

func (s *server) CreateColumnFamily(ctx context.Context, req *bttspb.CreateColumnFamilyRequest) (*bttdpb.ColumnFamily, error) {
	s.mu.Lock()
	tbl, ok := s.tables[req.Name]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no such table %q", req.Name)
	}

	// Check it is unique and record it.
	fam := req.ColumnFamilyId
	tbl.mu.Lock()
	defer tbl.mu.Unlock()
	if _, ok := tbl.families[fam]; ok {
		return nil, fmt.Errorf("family %q already exists", fam)
	}
	tbl.families[fam] = true
	return &bttdpb.ColumnFamily{
		Name: req.Name + "/families/" + fam,
	}, nil
}

func (s *server) ReadRows(req *btspb.ReadRowsRequest, stream btspb.BigtableService_ReadRowsServer) error {
	s.mu.Lock()
	tbl, ok := s.tables[req.TableName]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("no such table %q", req.TableName)
	}

	var start, end string // half-open interval
	if rr := req.RowRange; rr != nil {
		start, end = string(rr.StartKey), string(rr.EndKey)
	} else {
		// A single row read is simply an edge case.
		start = string(req.RowKey)
		end = start + "\x00"
	}

	// Get rows to stream back.
	tbl.mu.RLock()
	si, ei := 0, len(tbl.rows) // half-open interval
	if start != "" {
		si = sort.Search(len(tbl.rows), func(i int) bool { return tbl.rows[i].key >= start })
	}
	if end != "" {
		ei = sort.Search(len(tbl.rows), func(i int) bool { return tbl.rows[i].key >= end })
	}
	if si >= ei {
		tbl.mu.RUnlock()
		return nil
	}
	rows := make([]*row, ei-si)
	copy(rows, tbl.rows[si:ei])
	tbl.mu.RUnlock()

	for _, r := range rows {
		if err := streamRow(stream, r, req.Filter); err != nil {
			return err
		}
	}

	return nil
}

func streamRow(stream btspb.BigtableService_ReadRowsServer, r *row, f *btdpb.RowFilter) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rrr := &btspb.ReadRowsResponse{
		RowKey: []byte(r.key),
	}
	for col, cell := range r.cells {
		i := strings.Index(col, ":") // guaranteed to exist
		fam, col := col[:i], col[i+1:]
		if !includeCell(f, r, fam, col, cell) {
			continue
		}
		// TODO(dsymonds): Apply transformers.
		rrr.Chunks = append(rrr.Chunks, &btspb.ReadRowsResponse_Chunk{
			RowContents: &btdpb.Family{
				Name: fam,
				Columns: []*btdpb.Column{{
					Qualifier: []byte(col),
					Cells: []*btdpb.Cell{{
						// TODO: timestamp
						Value: cell.value,
					}},
				}},
			},
		})
	}
	rrr.Chunks = append(rrr.Chunks, &btspb.ReadRowsResponse_Chunk{CommitRow: true})
	return stream.Send(rrr)
}

func includeCell(f *btdpb.RowFilter, r *row, fam, col string, cell cell) bool {
	if f == nil {
		return true
	}
	// TODO(dsymonds): Implement many more filters.
	switch {
	default:
		log.Printf("WARNING: don't know how to handle filter (ignoring it): %v", f)
		return true
	case len(f.ColumnQualifierRegexFilter) > 0:
		pat := string(f.ColumnQualifierRegexFilter)
		rx, err := regexp.Compile(pat)
		if err != nil {
			log.Printf("Bad column_qualifier_regex_filter pattern %q: %v", pat, err)
			return false
		}
		return rx.MatchString(col)
	}
}

func (s *server) MutateRow(ctx context.Context, req *btspb.MutateRowRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	tbl, ok := s.tables[req.TableName]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no such table %q", req.TableName)
	}

	r := tbl.mutableRow(string(req.RowKey))
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, mut := range req.Mutations {
		switch {
		default:
			return nil, fmt.Errorf("can't handle mutation %v", mut)
		case mut.SetCell != nil:
			set := mut.SetCell
			tbl.mu.RLock()
			famOK := tbl.families[set.FamilyName]
			tbl.mu.RUnlock()
			if !famOK {
				return nil, fmt.Errorf("unknown family %q", set.FamilyName)
			}
			col := fmt.Sprintf("%s:%s", set.FamilyName, set.ColumnQualifier)
			r.cells[col] = cell{value: set.Value}
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *server) ReadModifyWriteRow(ctx context.Context, req *btspb.ReadModifyWriteRowRequest) (*btdpb.Row, error) {
	s.mu.Lock()
	tbl, ok := s.tables[req.TableName]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no such table %q", req.TableName)
	}

	updates := make(map[string]cell) // copy of updated cells; keyed by full column name

	r := tbl.mutableRow(string(req.RowKey))
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rule := range req.Rules {
		key := fmt.Sprintf("%s:%s", rule.FamilyName, rule.ColumnQualifier)
		if len(rule.AppendValue) > 0 {
			r.cells[key] = cell{
				value: append(r.cells[key].value, rule.AppendValue...),
			}
		}
		if rule.IncrementAmount != 0 {
			var v int64
			if val := r.cells[key].value; len(val) > 0 {
				if len(val) != 8 {
					return nil, fmt.Errorf("increment on non-64-bit value")
				}
				v = int64(binary.BigEndian.Uint64(val))
			}
			v += rule.IncrementAmount
			var val [8]byte
			binary.BigEndian.PutUint64(val[:], uint64(v))
			r.cells[key] = cell{
				value: val[:],
			}
		}
		updates[key] = r.cells[key]
	}

	res := &btdpb.Row{
		Key: req.RowKey,
	}
	for col, cell := range updates {
		i := strings.Index(col, ":")
		fam, qual := col[:i], col[i+1:]
		var f *btdpb.Family
		for _, ff := range res.Families {
			if ff.Name == fam {
				f = ff
				break
			}
		}
		if f == nil {
			f = &btdpb.Family{Name: fam}
			res.Families = append(res.Families, f)
		}
		f.Columns = append(f.Columns, &btdpb.Column{
			Qualifier: []byte(qual),
			Cells: []*btdpb.Cell{{
				Value: cell.value,
			}},
		})
	}
	return res, nil
}

type table struct {
	mu       sync.RWMutex
	families map[string]bool // keyed by plain family name
	rows     []*row          // sorted by row key
	rowIndex map[string]*row // indexed by row key
}

func newTable() *table {
	return &table{
		families: make(map[string]bool),
		rowIndex: make(map[string]*row),
	}
}

func (t *table) mutableRow(row string) *row {
	// Try fast path first.
	t.mu.RLock()
	r := t.rowIndex[row]
	t.mu.RUnlock()
	if r != nil {
		return r
	}

	// We probably need to create the row.
	t.mu.Lock()
	r = t.rowIndex[row]
	if r == nil {
		r = newRow(row)
		t.rowIndex[row] = r
		t.rows = append(t.rows, r)
		sort.Sort(byRowKey(t.rows)) // yay, inefficient!
	}
	t.mu.Unlock()
	return r
}

type byRowKey []*row

func (b byRowKey) Len() int           { return len(b) }
func (b byRowKey) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byRowKey) Less(i, j int) bool { return b[i].key < b[j].key }

type row struct {
	key string

	mu    sync.Mutex
	cells map[string]cell // keyed by full column name
}

func newRow(key string) *row {
	return &row{
		key:   key,
		cells: make(map[string]cell),
	}
}

type cell struct {
	value []byte
	// TODO: timestamp, multiple values
}
