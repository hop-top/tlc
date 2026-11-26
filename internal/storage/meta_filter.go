package storage

import (
	"fmt"
	"sort"

	"hop.top/tlc/internal/core"
)

// MetaFilterClauses builds one WHERE clause per entry of filters, each
// comparing the string form of the top-level meta member to the wanted
// value. The clauses are meant to be ANDed. column names the JSON meta
// column ("meta", "tracks.meta") and is spliced into the SQL, so it must
// be a constant of the caller's, never user input.
//
// Keys are validated against core.ValidateMetaFilterKey and then bound,
// as a JSON path, like the values: nothing a caller supplies is ever
// part of the SQL text. A key that fails validation yields an error
// wrapping core.ErrInvalidMetaFilterKey.
//
// A member matches on its scalar string form: text as is, numbers as
// SQLite renders them ("5", "1.5"), booleans as "true" / "false". Null,
// absent, object and array members never match. Rows whose meta is not
// valid JSON never match rather than failing the query.
//
// Clauses come out sorted by key so the statement is deterministic.
func MetaFilterClauses(column string, filters map[string]string) ([]string, []any, error) {
	if len(filters) == 0 {
		return nil, nil, nil
	}
	if err := core.ValidateMetaFilters(filters); err != nil {
		return nil, nil, err
	}
	keys := make([]string, 0, len(filters))
	for k := range filters {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	clause := fmt.Sprintf(
		`CASE WHEN json_valid(%[1]s) THEN `+
			`json_type(%[1]s, ?) IN ('text','integer','real','true','false') AND `+
			`CASE json_type(%[1]s, ?) WHEN 'true' THEN 'true' WHEN 'false' THEN 'false' `+
			`ELSE CAST(json_extract(%[1]s, ?) AS TEXT) END = ? `+
			`ELSE 0 END`,
		column,
	)
	clauses := make([]string, 0, len(keys))
	args := make([]any, 0, 4*len(keys))
	for _, k := range keys {
		path := `$."` + k + `"`
		clauses = append(clauses, clause)
		args = append(args, path, path, path, filters[k])
	}
	return clauses, args, nil
}
