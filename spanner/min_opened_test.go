/*
Copyright 2025 Google LLC

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

package spanner

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"testing"
)

// TestMinOpenedZeroConcurrentDML tests that concurrent DML operations work correctly
// when MinOpened=0 is set.
func TestMinOpenedZeroConcurrentDML(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	logger := log.Default()
	logger.SetOutput(io.Discard)

	sessionPoolConfig := DefaultSessionPoolConfig
	sessionPoolConfig.MinOpened = 0
	sessionPoolConfig.MaxOpened = 100

	client, _, cleanup := prepareIntegrationTest(ctx, t, sessionPoolConfig, []string{
		`CREATE TABLE TestTable (id INT64 NOT NULL, value STRING(MAX)) PRIMARY KEY(id)`,
	})
	defer cleanup()

	const concurrency = 10
	const iterations = 20
	const max = 100

	var initial []*Mutation
	for i := range max {
		initial = append(initial, InsertMap("TestTable", map[string]any{
			"id":    int64(i),
			"value": fmt.Sprintf("value-%d", i),
		}))
	}
	_, err := client.Apply(ctx, initial)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errors := make(chan error, concurrency*iterations)

	for g := range concurrency {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for i := range iterations {
				_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
					stmt := Statement{
						SQL: "UPDATE TestTable SET value = @p1 WHERE id = @p2",
						Params: map[string]any{
							"p1": fmt.Sprintf("value-%d-%d", goroutineID, i),
							"p2": int64((goroutineID*iterations + i) % max),
						},
					}
					rowCount, err := tx.Update(ctx, stmt)
					if err != nil {
						return err
					}
					if rowCount != 1 {
						return fmt.Errorf("expected 1 row affected, got %d", rowCount)
					}
					return nil
				})
				if err != nil {
					errors <- fmt.Errorf("goroutine %d iteration %d: %w", goroutineID, i, err)
				}
			}
		}(g)
	}

	wg.Wait()
	close(errors)

	var allErrors []error
	for err := range errors {
		allErrors = append(allErrors, err)
	}

	if len(allErrors) > 0 {
		t.Errorf("Got %d errors during concurrent DML operations:", len(allErrors))
		for i, err := range allErrors {
			if i >= 10 {
				t.Errorf("  ... and %d more errors", len(allErrors)-10)
				break
			}
			t.Errorf("  %v", err)
		}
	}
}

// TestMinOpenedZeroSequentialDML tests that sequential DML operations work correctly
// when MinOpened=0 is set.
func TestMinOpenedZeroSequentialDML(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	logger := log.Default()
	logger.SetOutput(io.Discard)

	sessionPoolConfig := DefaultSessionPoolConfig
	sessionPoolConfig.MinOpened = 0
	sessionPoolConfig.MaxOpened = 100

	client, _, cleanup := prepareIntegrationTest(ctx, t, sessionPoolConfig, []string{
		`CREATE TABLE TestTable (id INT64 NOT NULL, value STRING(MAX)) PRIMARY KEY(id)`,
	})
	defer cleanup()

	const max = 10

	var initial []*Mutation
	for i := range max {
		initial = append(initial, InsertMap("TestTable", map[string]any{
			"id":    int64(i),
			"value": fmt.Sprintf("value-%d", i),
		}))
	}
	_, err := client.Apply(ctx, initial)
	if err != nil {
		t.Fatal(err)
	}

	// Perform many sequential updates
	for i := range 50 {
		_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			stmt := Statement{
				SQL: "UPDATE TestTable SET value = @p1 WHERE id = @p2",
				Params: map[string]any{
					"p1": fmt.Sprintf("value-%d", i),
					"p2": int64(i % max),
				},
			}
			rowCount, err := tx.Update(ctx, stmt)
			if err != nil {
				return err
			}
			if rowCount != 1 {
				return fmt.Errorf("expected 1 row affected, got %d", rowCount)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}
}

// TestMinOpenedZeroRapidClientCreateClose tests rapid client creation and closing
// with MinOpened=0 to stress the session pool lifecycle.
func TestMinOpenedZeroRapidClientCreateClose(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	logger := log.Default()
	logger.SetOutput(io.Discard)

	sessionPoolConfig := DefaultSessionPoolConfig
	sessionPoolConfig.MinOpened = 0
	sessionPoolConfig.MaxOpened = 100

	client, dbPath, cleanup := prepareIntegrationTest(ctx, t, sessionPoolConfig, []string{
		`CREATE TABLE TestTable (id INT64 NOT NULL, value STRING(MAX)) PRIMARY KEY(id)`,
	})
	defer cleanup()

	const max = 10

	var initial []*Mutation
	for i := range max {
		initial = append(initial, InsertMap("TestTable", map[string]any{
			"id":    int64(i),
			"value": fmt.Sprintf("value-%d", i),
		}))
	}
	_, err := client.Apply(ctx, initial)
	if err != nil {
		t.Fatal(err)
	}

	// Rapidly create clients, do work, and close them
	for cycle := range 10 {
		client, err := createClient(ctx, dbPath, ClientConfig{
			DisableNativeMetrics: true,
			Logger:               logger,
			SessionPoolConfig: SessionPoolConfig{
				MinOpened: 0,
				MaxOpened: 5,
			},
		})
		if err != nil {
			t.Fatalf("cycle %d: failed to create client: %v", cycle, err)
		}

		// Do some DML work
		for i := range 5 {
			_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
				stmt := Statement{
					SQL: "UPDATE TestTable SET value = @p1 WHERE id = @p2",
					Params: map[string]any{
						"p1": fmt.Sprintf("cycle-%d-iter-%d", cycle, i),
						"p2": int64((cycle*5 + i) % max),
					},
				}
				_, err := tx.Update(ctx, stmt)
				return err
			})
			if err != nil {
				client.Close()
				t.Fatalf("cycle %d iteration %d: %v", cycle, i, err)
			}
		}

		client.Close()
	}
}

// TestMinOpenedZeroSessionRecycling tests that session recycling works correctly
// when MinOpened=0, ensuring sessions are properly cleaned up before reuse.
func TestMinOpenedZeroSessionRecycling(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	logger := log.Default()
	logger.SetOutput(io.Discard)

	sessionPoolConfig := DefaultSessionPoolConfig
	sessionPoolConfig.MinOpened = 0
	sessionPoolConfig.MaxOpened = 1

	client, _, cleanup := prepareIntegrationTest(ctx, t, sessionPoolConfig, []string{
		`CREATE TABLE TestTable (id INT64 NOT NULL, value STRING(MAX)) PRIMARY KEY(id)`,
	})
	defer cleanup()

	const max = 20

	var initial []*Mutation
	for i := range max {
		initial = append(initial, InsertMap("TestTable", map[string]any{
			"id":    int64(i),
			"value": fmt.Sprintf("value-%d", i),
		}))
	}
	_, err := client.Apply(ctx, initial)
	if err != nil {
		t.Fatal(err)
	}

	// With MaxOpened=1, all transactions must use the same session.
	// This tests that sessions are properly recycled without state conflicts.
	for i := range max {
		_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			stmt := Statement{
				SQL: "UPDATE TestTable SET value = @p1 WHERE id = @p2",
				Params: map[string]any{
					"p1": fmt.Sprintf("value-%d", i),
					"p2": int64(i),
				},
			}
			_, err := tx.Update(ctx, stmt)
			return err
		})
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}
}
