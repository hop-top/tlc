package cli

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"text/template"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"hop.top/kit/go/console/output"
)

// Record output.
//
// One entity (task, track) has ONE record type. `list` and `show` build
// the same struct, so a field present in one is present in the other, and
// every format renders from the same JSON document: json, yaml and csv
// cannot disagree about which fields exist or what they are called.
//
// The flags follow gh:
//
//	--json f1,f2   select fields (bare --json lists them and fails)
//	--jq expr      filter the selected document
//	--template t   Go text/template over the selected records
//
// Nulls are kept: a record carries every field, with null for unset.

const (
	flagRecordJSON = "json"
	flagRecordJQ   = "jq"
	flagTemplate   = "template"
)

// registerRecordFlags adds --json and --jq to a command whose output is
// record-shaped, and wires the bare `--json` field listing. --template is
// the kit-owned global and is not redeclared.
func registerRecordFlags(cmd *cobra.Command, fields []string) {
	cmd.Flags().StringSlice(flagRecordJSON, nil,
		"Output JSON with the specified fields (comma-separated; see `--json` fields below)")
	cmd.Flags().String(flagRecordJQ, "",
		"Filter the JSON output with a jq expression (requires --json or --format json)")
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		if err != nil && strings.Contains(err.Error(), "flag needs an argument: --json") {
			return usageErr(fmt.Errorf(
				"specify one or more comma-separated fields for --json:\n  %s",
				strings.Join(fields, "\n  ")))
		}
		return err
	})
}

// recordOpts is the resolved output request for a record command.
type recordOpts struct {
	format string
	sel    []string
	jq     string
	tmpl   string
}

// structured reports whether this request is served by the record
// pipeline rather than a command's own table renderer.
func (o recordOpts) structured() bool {
	switch o.format {
	case formatJSON, formatYAML, formatCSV:
		return true
	}
	return o.tmpl != ""
}

// resolveRecordOpts reads --json, --jq, --template and the effective
// format. --json implies json and so does --jq; naming another format
// beside them is an error rather than a silent winner.
func resolveRecordOpts(cmd *cobra.Command, avail []string) (recordOpts, error) {
	o := recordOpts{format: viper.GetString("output.format")}
	o.tmpl, _ = cmd.Flags().GetString(flagTemplate)      //nolint:errcheck // flag may be absent on bare test roots
	o.jq, _ = cmd.Flags().GetString(flagRecordJQ)        //nolint:errcheck // flag may be absent on bare test roots
	sel, _ := cmd.Flags().GetStringSlice(flagRecordJSON) //nolint:errcheck // flag may be absent on bare test roots

	explicit := false
	if f := cmd.Flags().Lookup("format"); f != nil && f.Changed {
		explicit = true
		o.format = f.Value.String()
	}

	for _, s := range sel {
		for _, p := range strings.Split(s, ",") {
			if p = strings.TrimSpace(p); p != "" {
				o.sel = append(o.sel, p)
			}
		}
	}
	jsonFlag := cmd.Flags().Changed(flagRecordJSON)
	if jsonFlag && len(o.sel) == 0 {
		return o, usageErr(fmt.Errorf(
			"specify one or more comma-separated fields for --json:\n  %s",
			strings.Join(avail, "\n  ")))
	}
	if len(o.sel) > 0 {
		valid := make(map[string]bool, len(avail))
		for _, a := range avail {
			valid[a] = true
		}
		for _, s := range o.sel {
			if !valid[s] {
				return o, usageErr(fmt.Errorf(
					"unknown --json field %q; available fields:\n  %s",
					s, strings.Join(avail, "\n  ")))
			}
		}
	}

	switch {
	case jsonFlag || o.jq != "":
		if explicit && o.format != formatJSON {
			return o, usageErr(fmt.Errorf(
				"--json/--jq produce JSON and cannot be combined with --format %s", o.format))
		}
		if o.tmpl == "" {
			o.format = formatJSON
		}
	}
	if o.jq != "" && o.tmpl != "" {
		return o, usageErr(errors.New("--jq and --template are mutually exclusive"))
	}
	if cols := colsFromFlags(cmd); len(cols) > 0 && o.structured() {
		return o, usageErr(fmt.Errorf(
			"--cols selects table columns; for --format %s use --json <fields>", o.format))
	}
	return o, nil
}

// fieldsOf lists the JSON keys of struct type t in declaration order,
// flattening embedded structs the way encoding/json does.
func fieldsOf(t reflect.Type) []string {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
			out = append(out, fieldsOf(f.Type)...)
			continue
		}
		if name == "-" || !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out = append(out, name)
	}
	return out
}

// orderedRecord is a JSON object that keeps its key order.
type orderedRecord struct {
	keys []string
	vals map[string]json.RawMessage
}

func (o orderedRecord) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k) //nolint:errcheck // string marshal cannot fail
		b.Write(kb)
		b.WriteByte(':')
		v := o.vals[k]
		if len(v) == 0 {
			v = json.RawMessage("null")
		}
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// toRecords converts a record struct (or slice of them) into ordered
// records restricted to sel (all of avail when sel is empty).
func toRecords(v any, avail, sel []string) ([]orderedRecord, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode records: %w", err)
	}
	var rows []map[string]json.RawMessage
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
		err = json.Unmarshal(raw, &rows)
	} else {
		var one map[string]json.RawMessage
		err = json.Unmarshal(raw, &one)
		rows = []map[string]json.RawMessage{one}
	}
	if err != nil {
		return nil, fmt.Errorf("decode records: %w", err)
	}
	keys := avail
	if len(sel) > 0 {
		keys = sel
	}
	out := make([]orderedRecord, len(rows))
	for i, r := range rows {
		out[i] = orderedRecord{keys: keys, vals: r}
	}
	return out, nil
}

// writeRecords renders records (slice or single struct) per o. single
// marks a one-entity document (show), emitted as an object, not an array.
func writeRecords(w io.Writer, o recordOpts, avail []string, v any, single bool) error {
	recs, err := toRecords(v, avail, o.sel)
	if err != nil {
		return err
	}
	if o.format == formatCSV && o.tmpl == "" {
		return writeCSV(w, recs)
	}
	var doc any = recs
	if single && len(recs) == 1 {
		doc = recs[0]
	}
	if recs == nil {
		doc = []orderedRecord{}
	}
	return writeDoc(w, o, doc, recs)
}

// writeDoc renders an arbitrary document. items feeds --template's .Items.
func writeDoc(w io.Writer, o recordOpts, doc any, items []orderedRecord) error {
	switch {
	case o.tmpl != "":
		return writeTemplate(w, o.tmpl, doc, items)
	case o.jq != "":
		return writeJQ(w, o.jq, doc)
	case o.format == formatYAML:
		return writeYAMLDoc(w, doc)
	default:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(doc) //nolint:wrapcheck // plain encoder error
	}
}

// writeYAMLDoc renders doc as YAML through its JSON form, so the YAML
// carries exactly the JSON keys, order and values.
func writeYAMLDoc(w io.Writer, doc any) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	var n yaml.Node
	if err := yaml.Unmarshal(raw, &n); err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	clearYAMLStyle(&n)
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(&n); err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	return enc.Close() //nolint:wrapcheck // plain encoder error
}

func clearYAMLStyle(n *yaml.Node) {
	n.Style = 0
	for _, c := range n.Content {
		clearYAMLStyle(c)
	}
}

func writeJQ(w io.Writer, expr string, doc any) error {
	q, err := gojq.Parse(expr)
	if err != nil {
		return usageErr(fmt.Errorf("invalid --jq expression: %w", err))
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode jq input: %w", err)
	}
	var input any
	if err := json.Unmarshal(raw, &input); err != nil {
		return fmt.Errorf("decode jq input: %w", err)
	}
	iter := q.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			return nil
		}
		if e, isErr := v.(error); isErr {
			return fmt.Errorf("--jq: %w", e)
		}
		if s, isStr := v.(string); isStr {
			_, _ = fmt.Fprintln(w, s)
			continue
		}
		b, mErr := json.MarshalIndent(v, "", "  ")
		if mErr != nil {
			return fmt.Errorf("--jq: %w", mErr)
		}
		_, _ = fmt.Fprintln(w, string(b))
	}
}

// writeTemplate runs --template over {Items, Cols, Data}: the same input
// contract kit's own --template uses, with Items keyed by JSON field name.
func writeTemplate(w io.Writer, src string, doc any, items []orderedRecord) error {
	t, err := template.New("output").Parse(src)
	if err != nil {
		return usageErr(fmt.Errorf("parse template: %w", err))
	}
	maps := make([]map[string]any, len(items))
	var cols []string
	for i, it := range items {
		raw, _ := it.MarshalJSON() //nolint:errcheck // marshal of raw messages
		if err := json.Unmarshal(raw, &maps[i]); err != nil {
			return fmt.Errorf("decode template item: %w", err)
		}
		cols = it.keys
	}
	var data any
	raw, _ := json.Marshal(doc)    //nolint:errcheck // marshal of raw messages
	_ = json.Unmarshal(raw, &data) //nolint:errcheck // raw came from json.Marshal
	input := struct {
		Items []map[string]any
		Cols  []string
		Data  any
	}{maps, cols, data}
	if err := t.Execute(w, input); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}
	return nil
}

// writeCSV emits one header row (the field names, in record order) and one
// row per record. Cells: null is empty, scalars are literal, arrays of
// scalars join with ";", anything nested is compact JSON.
func writeCSV(w io.Writer, recs []orderedRecord) error {
	cw := csv.NewWriter(w)
	if len(recs) == 0 {
		return nil
	}
	if err := cw.Write(recs[0].keys); err != nil {
		return fmt.Errorf("write csv: %w", err)
	}
	for _, r := range recs {
		row := make([]string, len(r.keys))
		for i, k := range r.keys {
			row[i] = csvCell(r.vals[k])
		}
		if err := cw.Write(row); err != nil {
			return fmt.Errorf("write csv: %w", err)
		}
	}
	cw.Flush()
	return cw.Error() //nolint:wrapcheck // plain writer error
}

func csvCell(raw json.RawMessage) string {
	var v any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			switch s := e.(type) {
			case string:
				parts[i] = s
			case float64, bool:
				parts[i] = fmt.Sprint(s)
			default:
				return string(raw)
			}
		}
		return strings.Join(parts, ";")
	case map[string]any:
		return string(raw)
	default:
		return strings.TrimSpace(string(raw))
	}
}

// usageErr reports err as a kit usage error (exit code 2).
func usageErr(err error) error {
	return output.UsageError(err.Error())
}

// applyRecordFormat makes --json/--jq select the json format for the whole
// command, so every code path that reads output.format agrees.
func applyRecordFormat(cmd *cobra.Command) {
	if cmd.Flags().Changed(flagRecordJSON) || cmd.Flags().Changed(flagRecordJQ) {
		viper.Set("output.format", formatJSON)
	}
}
