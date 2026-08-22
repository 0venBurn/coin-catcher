package scraper

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// stagedLoad describes one "COPY rows into a temp staging table, then
// INSERT..SELECT..upsert into the real table" step; execStagedLoads is the
// single execution path for all of them. The upsert SQL reads exclusively
// from the staging table, so an empty row set makes the whole step a no-op
// and it is skipped.
type stagedLoad struct {
	label   string   // error-message prefix, e.g. "staged items"
	table   string   // temp table name used by COPY and the upsert
	create  string   // CREATE TEMP TABLE DDL; ON COMMIT DROP is appended
	columns []string // COPY column order
	rows    [][]any
	upsert  string // INSERT INTO <real table> SELECT ... FROM <temp table>
}

// execStagedLoads runs each load inside the caller's transaction: create the
// temp table, COPY the rows in (the fast bulk path), then apply the upsert.
// Everything shares one transaction so a failure leaves no staging table or
// partial rows behind; ON COMMIT DROP cleans up on success. Labels appear in
// every error so a failed stage names exactly which step broke.
func execStagedLoads(ctx context.Context, tx pgx.Tx, loads []stagedLoad) error {
	for _, load := range loads {
		if len(load.rows) == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, load.create+" ON COMMIT DROP"); err != nil {
			return fmt.Errorf("create %s staging table: %w", load.label, err)
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{load.table}, load.columns, pgx.CopyFromRows(load.rows)); err != nil {
			return fmt.Errorf("copy %s: %w", load.label, err)
		}
		if _, err := tx.Exec(ctx, load.upsert); err != nil {
			return fmt.Errorf("upsert %s: %w", load.label, err)
		}
	}
	return nil
}
