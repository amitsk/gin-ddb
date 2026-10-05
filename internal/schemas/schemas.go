// Package schemas validates request bodies and query params per spec §6.
// Every failure returns normative apperrors.Detail entries for the
// 400 VALIDATION_ERROR envelope; status codes stay in the handlers.
package schemas

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"gin-ddb/internal/apperrors"
	"gin-ddb/internal/models"
	"gin-ddb/internal/pagination"
)

// Kind selects the record schema for body decoding.
type Kind string

const (
	KindStatic  Kind = "static"
	KindDynamic Kind = "dynamic"
	KindResort  Kind = "resort"
)

var (
	datePattern     = regexp.MustCompile(`^\d{2}\/\d{2}\/\d{2}$`)
	liftTimePattern = regexp.MustCompile(`^\d{1,2}:\d{2}$`)
	decimalPattern  = regexp.MustCompile(`^[0-9]+$`)

	liftStatuses = map[string]bool{"Open": true, "Closed": true, "Pending": true}
	avalanches   = map[string]bool{
		"Low": true, "Moderate": true, "Considerable": true,
		"High": true, "Extreme": true,
	}
)

// allowedFields per kind (closed schemas: anything else → unknown field).
var allowedFields = map[Kind]map[string]bool{
	KindStatic: {
		"Lift": true, "Metadata": true, "IsDetachable": true,
		"VerticalFeet": true, "LiftTime": true,
	},
	KindDynamic: {
		"Lift": true, "Metadata": true, "TotalUniqueLiftRiders": true,
		"SnowCount": true, "LiftStatus": true, "AvalancheDanger": true,
	},
	KindResort: {
		"Lift": true, "Metadata": true, "TotalRiders": true,
		"TotalSnow": true, "AvalancheDanger": true, "OpenLifts": true,
	},
}

var requiredFields = map[Kind][]string{
	KindStatic:  {"Lift", "Metadata", "IsDetachable", "VerticalFeet", "LiftTime"},
	KindDynamic: {"Lift", "Metadata", "TotalUniqueLiftRiders", "SnowCount", "LiftStatus", "AvalancheDanger"},
	KindResort:  {"Lift", "Metadata", "TotalRiders", "TotalSnow", "AvalancheDanger", "OpenLifts"},
}

// rawMap parses the body into untyped fields.
func rawMap(body []byte) (map[string]any, []apperrors.Detail) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, []apperrors.Detail{{Field: "body", Message: "invalid JSON"}}
	}
	if m == nil {
		return nil, []apperrors.Detail{{Field: "body", Message: "invalid JSON"}}
	}
	return m, nil
}

func asInt(v any) (int, bool) {
	f, ok := v.(float64)
	if !ok || math.Trunc(f) != f {
		return 0, false
	}
	return int(f), true
}

func checkLift(v any) (any, string) {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return nil, "must be a non-empty string"
	}
	return strings.TrimSpace(s), ""
}

func checkMetadata(kind Kind) func(any) (any, string) {
	return func(v any) (any, string) {
		s, ok := v.(string)
		if !ok {
			return nil, "must match pattern for this route"
		}
		switch kind {
		case KindStatic:
			if s != "static" {
				return nil, `must match pattern ^static$`
			}
		case KindResort:
			if s != "resort" {
				return nil, `must match pattern ^resort$`
			}
		default:
			if !datePattern.MatchString(s) {
				return nil, `must match pattern ^\d{2}\/\d{2}\/\d{2}$`
			}
		}
		return s, ""
	}
}

func checkBool(v any) (any, string) {
	b, ok := v.(bool)
	if !ok {
		return nil, "invalid value: must be a boolean"
	}
	return b, ""
}

func checkPositiveInt(v any) (any, string) {
	n, ok := asInt(v)
	if !ok {
		return nil, "must be an integer > 0"
	}
	if n <= 0 {
		return nil, "must be > 0"
	}
	return n, ""
}

func checkNonNegativeInt(v any) (any, string) {
	n, ok := asInt(v)
	if !ok || n < 0 {
		return nil, "must be an integer >= 0"
	}
	return n, ""
}

func checkLiftTime(v any) (any, string) {
	s, ok := v.(string)
	if !ok || !liftTimePattern.MatchString(s) {
		return nil, `must match pattern ^\d{1,2}:\d{2}$`
	}
	return s, ""
}

func checkEnum(set map[string]bool) func(any) (any, string) {
	return func(v any) (any, string) {
		s, ok := v.(string)
		if !ok || !set[s] {
			return nil, "invalid value"
		}
		return s, ""
	}
}

func checkOpenLifts(v any) (any, string) {
	list, ok := v.([]any)
	if !ok {
		return nil, "must be an integer >= 0"
	}
	out := make([]int, 0, len(list))
	for _, e := range list {
		n, ok := asInt(e)
		if !ok || n < 0 {
			return nil, "must be an integer >= 0"
		}
		out = append(out, n)
	}
	return out, ""
}

var fieldChecks = map[string]func(Kind) func(any) (any, string){
	"Lift":                  func(Kind) func(any) (any, string) { return checkLift },
	"Metadata":              checkMetadata,
	"IsDetachable":          func(Kind) func(any) (any, string) { return checkBool },
	"VerticalFeet":          func(Kind) func(any) (any, string) { return checkPositiveInt },
	"LiftTime":              func(Kind) func(any) (any, string) { return checkLiftTime },
	"TotalUniqueLiftRiders": func(Kind) func(any) (any, string) { return checkNonNegativeInt },
	"SnowCount":             func(Kind) func(any) (any, string) { return checkNonNegativeInt },
	"TotalRiders":           func(Kind) func(any) (any, string) { return checkNonNegativeInt },
	"TotalSnow":             func(Kind) func(any) (any, string) { return checkNonNegativeInt },
	"LiftStatus":            func(Kind) func(any) (any, string) { return checkEnum(liftStatuses) },
	"AvalancheDanger":       func(Kind) func(any) (any, string) { return checkEnum(avalanches) },
	"OpenLifts":             func(Kind) func(any) (any, string) { return checkOpenLifts },
}

// validateFields runs unknown-field and per-field checks, returning
// normalized values. When requireAll is set, missing required fields
// are reported; otherwise only present fields are checked.
func validateFields(kind Kind, m map[string]any, requireAll bool) (map[string]any, []apperrors.Detail) {
	var details []apperrors.Detail
	for name := range m {
		if !allowedFields[kind][name] {
			details = append(details, apperrors.Detail{Field: name, Message: "unknown field"})
		}
	}
	if requireAll {
		for _, name := range requiredFields[kind] {
			if _, ok := m[name]; !ok {
				details = append(details, apperrors.Detail{Field: name, Message: "required"})
			}
		}
	}
	values := map[string]any{}
	for name, raw := range m {
		check, ok := fieldChecks[name]
		if !ok {
			continue
		}
		v, msg := check(kind)(raw)
		if msg != "" {
			details = append(details, apperrors.Detail{Field: name, Message: msg})
			continue
		}
		values[name] = v
	}
	return values, details
}

// DecodeCreate validates a POST body and returns the typed record.
func DecodeCreate(kind Kind, body []byte) (any, []apperrors.Detail) {
	m, details := rawMap(body)
	if details != nil {
		return nil, details
	}
	values, details := validateFields(kind, m, true)
	if len(details) > 0 {
		return nil, details
	}
	switch kind {
	case KindStatic:
		return models.StaticRecord{
			Lift:         values["Lift"].(string),
			Metadata:     values["Metadata"].(string),
			IsDetachable: values["IsDetachable"].(bool),
			VerticalFeet: values["VerticalFeet"].(int),
			LiftTime:     values["LiftTime"].(string),
		}, nil
	case KindDynamic:
		return models.DynamicRecord{
			Lift:                  values["Lift"].(string),
			Metadata:              values["Metadata"].(string),
			TotalUniqueLiftRiders: values["TotalUniqueLiftRiders"].(int),
			SnowCount:             values["SnowCount"].(int),
			LiftStatus:            values["LiftStatus"].(string),
			AvalancheDanger:       values["AvalancheDanger"].(string),
		}, nil
	default:
		return models.ResortRecord{
			Lift:            values["Lift"].(string),
			Metadata:        values["Metadata"].(string),
			TotalRiders:     values["TotalRiders"].(int),
			TotalSnow:       values["TotalSnow"].(int),
			AvalancheDanger: values["AvalancheDanger"].(string),
			OpenLifts:       values["OpenLifts"].([]int),
		}, nil
	}
}

// DecodeUpdate validates a PUT body and returns updatable fields.
// Lift/Metadata are identity (checked by CheckIdentity), never updates.
func DecodeUpdate(kind Kind, body []byte) (map[string]any, []apperrors.Detail) {
	m, details := rawMap(body)
	if details != nil {
		return nil, details
	}
	values, details := validateFields(kind, m, false)
	delete(values, "Lift")
	delete(values, "Metadata")
	if len(values) == 0 && len(details) == 0 {
		return nil, []apperrors.Detail{{Field: "body", Message: "empty update: no updatable fields"}}
	}
	if len(details) > 0 {
		return nil, details
	}
	return values, nil
}

// CheckIdentity enforces §4.1: the path is authoritative; body keys may
// be absent but must match when present.
func CheckIdentity(pathLift, pathMeta string, body []byte) []apperrors.Detail {
	m, details := rawMap(body)
	if details != nil {
		return details
	}
	var out []apperrors.Detail
	if raw, ok := m["Lift"]; ok {
		s, isStr := raw.(string)
		if !isStr || strings.TrimSpace(s) != strings.TrimSpace(pathLift) {
			out = append(out, apperrors.Detail{
				Field:   "Lift",
				Message: fmt.Sprintf("must match path %q", pathLift),
			})
		}
	}
	if raw, ok := m["Metadata"]; ok {
		s, isStr := raw.(string)
		if !isStr || strings.TrimSpace(s) != strings.TrimSpace(pathMeta) {
			out = append(out, apperrors.Detail{
				Field:   "Metadata",
				Message: fmt.Sprintf("must match path %q", pathMeta),
			})
		}
	}
	return out
}

// ListQuery is the validated collection query: limit + opaque cursor.
type ListQuery struct {
	Limit    int
	Token    string
	HasToken bool
}

// ParseListQuery validates limit/nextToken per §§6,10.
func ParseListQuery(q url.Values) (ListQuery, []apperrors.Detail) {
	lq := ListQuery{Limit: 20}
	var details []apperrors.Detail
	if raw := q.Get("limit"); raw != "" {
		if !decimalPattern.MatchString(raw) {
			details = append(details, apperrors.Detail{Field: "limit", Message: "must be an integer > 0"})
		} else if n, _ := strconv.Atoi(raw); n < 1 {
			details = append(details, apperrors.Detail{Field: "limit", Message: "must be > 0"})
		} else if n > 100 {
			lq.Limit = 100
		} else {
			lq.Limit = n
		}
	}
	if _, ok := q["nextToken"]; ok {
		if _, err := pagination.Decode(q.Get("nextToken")); err != nil {
			details = append(details, apperrors.Detail{Field: "nextToken", Message: "malformed token"})
		} else {
			lq.Token, lq.HasToken = q.Get("nextToken"), true
		}
	}
	for _, legacy := range []string{"lastEvaluatedLift", "lastEvaluatedMetadata", "lastEvaluatedKey"} {
		if _, ok := q[legacy]; ok {
			details = append(details, apperrors.Detail{Field: legacy, Message: "use nextToken"})
		}
	}
	return lq, details
}

// RidersQuery adds the by-riders bounds to a ListQuery.
type RidersQuery struct {
	ListQuery
	Min *int
	Max *int
}

// ParseRidersQuery validates minRiders/maxRiders per §6.
func ParseRidersQuery(q url.Values) (RidersQuery, []apperrors.Detail) {
	lq, details := ParseListQuery(q)
	rq := RidersQuery{ListQuery: lq}
	for _, name := range []string{"minRiders", "maxRiders"} {
		raw := q.Get(name)
		if raw == "" {
			continue
		}
		if !decimalPattern.MatchString(raw) {
			details = append(details, apperrors.Detail{Field: name, Message: "must be an integer >= 0"})
			continue
		}
		n, _ := strconv.Atoi(raw)
		if name == "minRiders" {
			rq.Min = &n
		} else {
			rq.Max = &n
		}
	}
	if rq.Min != nil && rq.Max != nil && *rq.Min > *rq.Max {
		details = append(details, apperrors.Detail{Field: "minRiders", Message: "minRiders must be <= maxRiders"})
	}
	return rq, details
}
