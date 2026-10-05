package schemas

import (
	"net/url"
	"strings"
	"testing"

	"gin-ddb/internal/apperrors"
	"gin-ddb/internal/models"
)

func queryOf(raw string) url.Values {
	q, err := url.ParseQuery(raw)
	if err != nil {
		panic(err)
	}
	return q
}

func mustParseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	q, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad test query %q: %v", raw, err)
	}
	return q
}

func TestDecodeCreateStaticValid(t *testing.T) {
	rec, details := DecodeCreate(KindStatic, []byte(
		`{"Lift":"Summit Express","Metadata":"static","IsDetachable":false,"VerticalFeet":2500,"LiftTime":"8:00"}`))
	if len(details) != 0 {
		t.Fatalf("details = %v, want none", details)
	}
	s, ok := rec.(models.StaticRecord)
	if !ok {
		t.Fatalf("record type = %T, want StaticRecord", rec)
	}
	if s.Lift != "Summit Express" || s.VerticalFeet != 2500 || s.LiftTime != "8:00" || s.IsDetachable {
		t.Errorf("record = %+v, want fixture values", s)
	}
}

func TestDecodeCreateDynamicValid(t *testing.T) {
	rec, details := DecodeCreate(KindDynamic, []byte(
		`{"Lift":"Summit Express","Metadata":"01/15/24","TotalUniqueLiftRiders":1250,"SnowCount":48,"LiftStatus":"Open","AvalancheDanger":"Low"}`))
	if len(details) != 0 {
		t.Fatalf("details = %v, want none", details)
	}
	if _, ok := rec.(models.DynamicRecord); !ok {
		t.Fatalf("record type = %T, want DynamicRecord", rec)
	}
}

func TestDecodeCreateResortValid(t *testing.T) {
	rec, details := DecodeCreate(KindResort, []byte(
		`{"Lift":"Summit Resort","Metadata":"resort","TotalRiders":8500,"TotalSnow":42,"AvalancheDanger":"Moderate","OpenLifts":[1,2,3,4,5,6,7,8,9,10,11,12]}`))
	if len(details) != 0 {
		t.Fatalf("details = %v, want none", details)
	}
	r, ok := rec.(models.ResortRecord)
	if !ok {
		t.Fatalf("record type = %T, want ResortRecord", rec)
	}
	if len(r.OpenLifts) != 12 {
		t.Errorf("OpenLifts len = %d, want 12", len(r.OpenLifts))
	}
}

func TestDecodeCreateFailures(t *testing.T) {
	cases := []struct {
		name    string
		kind    Kind
		body    string
		field   string
		message string
	}{
		{"malformed json", KindStatic, `{"Lift":`, "body", "invalid JSON"},
		{"missing required", KindStatic,
			`{"Lift":"A","Metadata":"static","IsDetachable":false,"LiftTime":"8:00"}`,
			"VerticalFeet", "required"},
		{"unknown field", KindStatic,
			`{"Lift":"A","Metadata":"static","IsDetachable":false,"VerticalFeet":1,"LiftTime":"8:00","Nope":1}`,
			"Nope", "unknown field"},
		{"blank lift", KindStatic,
			`{"Lift":"   ","Metadata":"static","IsDetachable":false,"VerticalFeet":1,"LiftTime":"8:00"}`,
			"Lift", "must be a non-empty string"},
		{"bad metadata kind", KindStatic,
			`{"Lift":"A","Metadata":"resort","IsDetachable":false,"VerticalFeet":1,"LiftTime":"8:00"}`,
			"Metadata", "must match pattern"},
		{"bad date", KindDynamic,
			`{"Lift":"A","Metadata":"1/15/24","TotalUniqueLiftRiders":1,"SnowCount":1,"LiftStatus":"Open","AvalancheDanger":"Low"}`,
			"Metadata", "must match pattern"},
		{"truthy bool", KindStatic,
			`{"Lift":"A","Metadata":"static","IsDetachable":"true","VerticalFeet":1,"LiftTime":"8:00"}`,
			"IsDetachable", "invalid value"},
		{"zero vertical", KindStatic,
			`{"Lift":"A","Metadata":"static","IsDetachable":false,"VerticalFeet":0,"LiftTime":"8:00"}`,
			"VerticalFeet", "must be > 0"},
		{"bad lifttime", KindStatic,
			`{"Lift":"A","Metadata":"static","IsDetachable":false,"VerticalFeet":1,"LiftTime":"8"}`,
			"LiftTime", "must match pattern"},
		{"float riders", KindDynamic,
			`{"Lift":"A","Metadata":"01/15/24","TotalUniqueLiftRiders":1250.5,"SnowCount":1,"LiftStatus":"Open","AvalancheDanger":"Low"}`,
			"TotalUniqueLiftRiders", "must be an integer >= 0"},
		{"negative snow", KindDynamic,
			`{"Lift":"A","Metadata":"01/15/24","TotalUniqueLiftRiders":1,"SnowCount":-1,"LiftStatus":"Open","AvalancheDanger":"Low"}`,
			"SnowCount", "must be an integer >= 0"},
		{"bad status", KindDynamic,
			`{"Lift":"A","Metadata":"01/15/24","TotalUniqueLiftRiders":1,"SnowCount":1,"LiftStatus":"open","AvalancheDanger":"Low"}`,
			"LiftStatus", "invalid value"},
		{"bad avalanche", KindDynamic,
			`{"Lift":"A","Metadata":"01/15/24","TotalUniqueLiftRiders":1,"SnowCount":1,"LiftStatus":"Open","AvalancheDanger":"Severe"}`,
			"AvalancheDanger", "invalid value"},
		{"float openlifts", KindResort,
			`{"Lift":"A","Metadata":"resort","TotalRiders":1,"TotalSnow":1,"AvalancheDanger":"Low","OpenLifts":[1.5]}`,
			"OpenLifts", "must be an integer >= 0"},
		{"negative openlifts", KindResort,
			`{"Lift":"A","Metadata":"resort","TotalRiders":1,"TotalSnow":1,"AvalancheDanger":"Low","OpenLifts":[-1]}`,
			"OpenLifts", "must be an integer >= 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, details := DecodeCreate(tc.kind, []byte(tc.body))
			if len(details) == 0 {
				t.Fatalf("details empty, want entry for %s", tc.field)
			}
			found := false
			for _, d := range details {
				if d.Field == tc.field && containsMsg(d.Message, tc.message) {
					found = true
				}
			}
			if !found {
				t.Errorf("details = %v, want field %q message containing %q", details, tc.field, tc.message)
			}
		})
	}
}

func TestDecodeUpdateEmpty(t *testing.T) {
	for _, body := range []string{`{}`, `{"Lift":"A","Metadata":"static"}`} {
		if _, details := DecodeUpdate(KindStatic, []byte(body)); !hasMsg(details, "Lift", "empty update") &&
			!hasMsg(details, "body", "empty update") {
			t.Errorf("DecodeUpdate(%s) details = %v, want empty update", body, details)
		}
	}
}

func TestDecodeUpdatePartial(t *testing.T) {
	updates, details := DecodeUpdate(KindStatic, []byte(`{"VerticalFeet":3000}`))
	if len(details) != 0 {
		t.Fatalf("details = %v, want none", details)
	}
	if updates["VerticalFeet"] != 3000 {
		t.Errorf("updates = %v, want VerticalFeet=3000", updates)
	}
}

func TestCheckIdentity(t *testing.T) {
	if details := CheckIdentity("A", "static", []byte(`{"Lift":"A","Metadata":"static"}`)); len(details) != 0 {
		t.Errorf("matching keys details = %v, want none", details)
	}
	if details := CheckIdentity("A", "static", []byte(`{"Lift":"B"}`)); !hasMsg(details, "Lift", "must match path") {
		t.Errorf("mismatched Lift details = %v, want must match path", details)
	}
	if details := CheckIdentity("A", "static", []byte(`{"Metadata":"resort"}`)); !hasMsg(details, "Metadata", "must match path") {
		t.Errorf("mismatched Metadata details = %v, want must match path", details)
	}
	if details := CheckIdentity("A", "static", []byte(`{"VerticalFeet":1}`)); len(details) != 0 {
		t.Errorf("absent keys details = %v, want none (path supplies them)", details)
	}
}

func TestParseListQuery(t *testing.T) {
	q, details := ParseListQuery(mustParseQuery(t, ""))
	if len(details) != 0 || q.Limit != 20 || q.HasToken {
		t.Errorf("defaults = %+v, %v; want limit 20, no token", q, details)
	}
	q, _ = ParseListQuery(mustParseQuery(t, "limit=500"))
	if q.Limit != 100 {
		t.Errorf("limit=500 → %d, want cap 100", q.Limit)
	}
	for _, raw := range []string{"limit=0x10", "limit=1.5", "limit=abc", "limit=0", "limit=-3"} {
		if _, details := ParseListQuery(mustParseQuery(t, raw)); !hasMsg(details, "limit", "") {
			t.Errorf("ParseListQuery(%q) details = %v, want limit error", raw, details)
		}
	}
	if _, details := ParseListQuery(mustParseQuery(t, "nextToken=!!!")); !hasMsg(details, "nextToken", "malformed token") {
		t.Errorf("bad token details = %v, want malformed token", details)
	}
	if _, details := ParseListQuery(mustParseQuery(t, "lastEvaluatedLift=A")); !hasMsg(details, "lastEvaluatedLift", "use nextToken") {
		t.Errorf("legacy param details = %v, want use nextToken", details)
	}
}

func TestParseRidersQuery(t *testing.T) {
	rq, details := ParseRidersQuery(mustParseQuery(t, "minRiders=10&maxRiders=50"))
	if len(details) != 0 || *rq.Min != 10 || *rq.Max != 50 {
		t.Errorf("filters = %+v, %v; want 10..50", rq, details)
	}
	if _, details := ParseRidersQuery(mustParseQuery(t, "minRiders=60&maxRiders=50")); !hasMsg(details, "minRiders", "minRiders must be <= maxRiders") {
		t.Errorf("inverted range details = %v, want range error", details)
	}
	if _, details := ParseRidersQuery(mustParseQuery(t, "minRiders=1.5")); !hasMsg(details, "minRiders", "") {
		t.Errorf("float minRiders details = %v, want error", details)
	}
}

func hasMsg(details []apperrors.Detail, field, msg string) bool {
	for _, d := range details {
		if d.Field == field && (msg == "" || containsMsg(d.Message, msg)) {
			return true
		}
	}
	return false
}

func containsMsg(s, sub string) bool { return strings.Contains(s, sub) }
