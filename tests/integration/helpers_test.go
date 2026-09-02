//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

const testDatabaseURLEnv = "TEST_DATABASE_URL"

func openTestDatabase(t *testing.T) *sql.DB {
	t.Helper()

	databaseURL := os.Getenv(testDatabaseURLEnv)
	if databaseURL == "" {
		t.Skipf("%s is not set; integration test requires PostgreSQL", testDatabaseURLEnv)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	adminDB, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	if err := adminDB.PingContext(ctx); err != nil {
		_ = adminDB.Close()
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	schemaName := "goreflow_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := adminDB.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(schemaName)); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create test schema: %v", err)
	}

	testDB, err := sql.Open("postgres", databaseURLWithSearchPath(t, databaseURL, schemaName))
	if err != nil {
		dropTestSchema(t, adminDB, schemaName)
		_ = adminDB.Close()
		t.Fatalf("open isolated PostgreSQL schema: %v", err)
	}

	t.Cleanup(func() {
		_ = testDB.Close()
		dropTestSchema(t, adminDB, schemaName)
		_ = adminDB.Close()
	})

	migration, err := os.ReadFile(migrationPath(t))
	if err != nil {
		t.Fatalf("read jobs migration: %v", err)
	}
	if _, err := testDB.ExecContext(ctx, string(migration)); err != nil {
		t.Fatalf("apply jobs migration: %v", err)
	}

	return testDB
}

func databaseURLWithSearchPath(t *testing.T, databaseURL string, schemaName string) string {
	t.Helper()

	parsedURL, err := url.Parse(databaseURL)
	if err != nil || parsedURL.Scheme == "" {
		t.Fatalf("%s must be a PostgreSQL URL: %v", testDatabaseURLEnv, err)
	}

	query := parsedURL.Query()
	query.Set("search_path", schemaName)
	parsedURL.RawQuery = query.Encode()

	return parsedURL.String()
}

func migrationPath(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test path")
	}

	return filepath.Join(filepath.Dir(filename), "..", "..", "migrations", "0001_create_jobs.up.sql")
}

func dropTestSchema(t *testing.T, db *sql.DB, schemaName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := db.ExecContext(ctx, "DROP SCHEMA "+pq.QuoteIdentifier(schemaName)+" CASCADE"); err != nil {
		t.Errorf("drop test schema %s: %v", schemaName, err)
	}
}

func jsonEqual(left json.RawMessage, right json.RawMessage) bool {
	var leftValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		return false
	}

	var rightValue any
	if err := json.Unmarshal(right, &rightValue); err != nil {
		return false
	}

	return reflect.DeepEqual(leftValue, rightValue)
}
