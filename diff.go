package auditlog

import (
	"encoding/json"
	"reflect"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

// FieldChange represents a single field transition in an audit log entry.
type FieldChange struct {
	Old any `json:"old"`
	New any `json:"new"`
}

func buildIgnoredSet(fields []string) map[string]bool {
	set := make(map[string]bool, len(fields))
	for _, f := range fields {
		set[f] = true
	}
	return set
}

// buildCreateDiff maps every non-empty field to { "old": null, "new": value }.
func buildCreateDiff(record *core.Record, ignored map[string]bool) map[string]FieldChange {
	changes := make(map[string]FieldChange)
	for _, field := range record.Collection().Fields {
		name := field.GetName()
		if ignored[name] || field.GetHidden() {
			continue
		}
		val := normalizeValue(record.Get(name))
		if isEmptyValue(val) {
			continue
		}
		changes[name] = FieldChange{Old: nil, New: val}
	}
	return changes
}

// buildUpdateDiff maps every changed field to { "old": oldVal, "new": newVal }.
func buildUpdateDiff(oldRecord, newRecord *core.Record, ignored map[string]bool) map[string]FieldChange {
	changes := make(map[string]FieldChange)
	if oldRecord == nil {
		return changes
	}
	for _, field := range newRecord.Collection().Fields {
		name := field.GetName()
		if ignored[name] || field.GetHidden() {
			continue
		}
		oldVal := normalizeValue(oldRecord.Get(name))
		newVal := normalizeValue(newRecord.Get(name))
		if reflect.DeepEqual(oldVal, newVal) {
			continue
		}
		changes[name] = FieldChange{Old: oldVal, New: newVal}
	}
	return changes
}

// buildDeleteDiff maps every non-empty field at deletion time to { "old": value, "new": null }.
func buildDeleteDiff(record *core.Record, ignored map[string]bool) map[string]FieldChange {
	changes := make(map[string]FieldChange)
	for _, field := range record.Collection().Fields {
		name := field.GetName()
		if ignored[name] || field.GetHidden() {
			continue
		}
		val := normalizeValue(record.Get(name))
		if isEmptyValue(val) {
			continue
		}
		changes[name] = FieldChange{Old: val, New: nil}
	}
	return changes
}

func normalizeValue(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case types.DateTime:
		if val.IsZero() {
			return nil
		}
		return val.String()
	case *types.DateTime:
		if val == nil || val.IsZero() {
			return nil
		}
		return val.String()
	case types.JSONRaw:
		if len(val) == 0 {
			return nil
		}
		var parsed any
		if err := json.Unmarshal(val, &parsed); err == nil {
			return parsed
		}
		return string(val)
	case []byte:
		if len(val) == 0 {
			return nil
		}
		var parsed any
		if err := json.Unmarshal(val, &parsed); err == nil {
			return parsed
		}
		return string(val)
	case []string:
		if len(val) == 0 {
			return nil
		}
		out := make([]string, len(val))
		copy(out, val)
		return out
	}

	return v
}

func isEmptyValue(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return val == ""
	case bool:
		return !val
	case float64:
		return val == 0
	case int:
		return val == 0
	case int64:
		return val == 0
	case []string:
		return len(val) == 0
	case []any:
		return len(val) == 0
	case map[string]any:
		return len(val) == 0
	}
	return false
}
