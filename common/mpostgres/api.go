package mpostgres

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/LerianStudio/midaz/common"
	"github.com/bxcodec/dbresolver/v2"
)

type Table struct {
	Name    string
	db      dbresolver.DB
	Columns []string
}

// Create inserts a new record into the specified table.
func Create(ctx context.Context, t *Table, data map[string]any) (int64, error) {
	if len(data) == 0 {
		return 0, errors.New("no data provided for insertion")
	}

	keys := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	values := make([]any, 0, len(data))

	i := 1

	for k, v := range data {
		if !common.Contains(t.Columns, k) {
			return 0, errors.New("invalid column for table")
		}

		keys = append(keys, k)
		placeholders = append(placeholders, "$"+strconv.Itoa(i))
		values = append(values, v)
		i++
	}

	sqlStatement := `INSERT INTO ` + t.Name + ` (` + strings.Join(keys, ", ") + `) VALUES (` + strings.Join(placeholders, ", ") + `) RETURNING id`

	var lastInsertID int64

	if err := t.db.QueryRowContext(ctx, sqlStatement, values...).Scan(&lastInsertID); err != nil {
		return 0, err
	}

	return lastInsertID, nil
}

// Update safely updates records in a specified table
func Update(ctx context.Context, t *Table, id int64, data map[string]any) error {
	if len(data) == 0 {
		return errors.New("no data provided to update")
	}

	setClauses := make([]string, 0, len(data))
	values := make([]any, 0, len(data)+1)

	i := 1

	for k, v := range data {
		if !common.Contains(t.Columns, k) {
			return fmt.Errorf("invalid column name: %s", k)
		}

		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", k, i))
		values = append(values, v)
		i++
	}

	values = append(values, id)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE id = $%d", t.Name, strings.Join(setClauses, ", "), i)
	if _, err := t.db.ExecContext(ctx, query, values...); err != nil {
		return err
	}

	return nil
}

// Delete removes a record identified by its ID.
func Delete(ctx context.Context, t *Table, id int64) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", t.Name)
	if _, err := t.db.ExecContext(ctx, query, id); err != nil {
		return err
	}

	return nil
}

// FindAll fetches records from a PostgreSQL table
func FindAll(ctx context.Context, t *Table, dest any, conditions string, args ...any) error {
	query := `SELECT * FROM ` + t.Name
	if conditions != "" {
		query += " WHERE " + conditions
	}

	rows, err := t.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	sliceVal := reflect.ValueOf(dest).Elem()
	elemType := sliceVal.Type().Elem()

	for rows.Next() {
		elem := reflect.New(elemType).Interface()
		if err := rows.Scan(elem); err != nil {
			return err
		}

		sliceVal.Set(reflect.Append(sliceVal, reflect.ValueOf(elem).Elem()))
	}

	return rows.Err()
}

// Count returns the number of rows in the table
func Count(ctx context.Context, t *Table, conditions string, args ...any) (int64, error) {
	query := `SELECT COUNT(*) FROM ` + t.Name
	if conditions != "" {
		query += " WHERE " + conditions
	}

	var count int64

	err := t.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

// FindByID finds a row by ID
func FindByID(ctx context.Context, t *Table, id int64, dest any) error {
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = $1", t.Name)

	return t.db.QueryRowContext(ctx, query, id).Scan(dest)
}
