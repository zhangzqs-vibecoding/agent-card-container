package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed *.sql
var files embed.FS

func Apply(ctx context.Context, database *sql.DB) error {
	connection, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	defer connection.Close()
	if _, err := connection.ExecContext(
		ctx,
		`SELECT pg_advisory_lock(hashtext('agentcard_schema_migrations'))`,
	); err != nil {
		return err
	}
	defer func() {
		_, _ = connection.ExecContext(
			context.Background(),
			`SELECT pg_advisory_unlock(hashtext('agentcard_schema_migrations'))`,
		)
	}()
	if _, err := connection.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (
		  name text PRIMARY KEY,
		  applied_at timestamptz NOT NULL DEFAULT now()
		)`,
	); err != nil {
		return err
	}
	entries, err := files.ReadDir(".")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var applied bool
		if err := connection.QueryRowContext(
			ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`,
			name,
		).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		source, err := files.ReadFile(name)
		if err != nil {
			return err
		}
		migration := strings.TrimSpace(string(source))
		migration = strings.TrimSpace(strings.TrimPrefix(migration, "BEGIN;"))
		migration = strings.TrimSpace(strings.TrimSuffix(migration, "COMMIT;"))
		transaction, err := connection.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, migration); err != nil {
			_ = transaction.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := transaction.ExecContext(
			ctx,
			`INSERT INTO schema_migrations (name) VALUES ($1)`,
			name,
		); err != nil {
			_ = transaction.Rollback()
			return err
		}
		if err := transaction.Commit(); err != nil {
			return err
		}
	}
	return nil
}
