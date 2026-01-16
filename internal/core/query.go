package core

type Operator string

const (
	OpEq      Operator = ":"
	OpEqual   Operator = "="
	OpNotEq   Operator = "!="
	OpGt      Operator = ">"
	OpGte     Operator = ">="
	OpLt      Operator = "<"
	OpLte     Operator = "<="
	OpContains Operator = "~"
	OpNotCont  Operator = "!~"
	OpStart   Operator = "^"
	OpEnd     Operator = "$"
)

type FieldFilter struct {
	Field    string
	Operator Operator
	Value    interface{}
}

type Query struct {
	Filters []FieldFilter
	Search  string // Full-text search term
	SortBy  string
	SortDirection string // "asc" or "desc"
	Limit   int
	Offset  int
}
