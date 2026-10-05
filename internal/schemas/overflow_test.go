package schemas

import (
	"testing"
)

func TestOverflowingIntegersAre400(t *testing.T) {
	huge := "99999999999999999999"
	if _, details := ParseListQuery(queryOf("limit=" + huge)); !hasMsg(details, "limit", "") {
		t.Errorf("limit overflow details = %v, want limit error", details)
	}
	if _, details := ParseRidersQuery(queryOf("minRiders=" + huge)); !hasMsg(details, "minRiders", "") {
		t.Errorf("minRiders overflow details = %v, want error", details)
	}
	if _, details := ParseRidersQuery(queryOf("maxRiders=" + huge)); !hasMsg(details, "maxRiders", "") {
		t.Errorf("maxRiders overflow details = %v, want error", details)
	}
}

func TestPresentEmptyParamsAre400(t *testing.T) {
	if _, details := ParseListQuery(queryOf("limit=")); !hasMsg(details, "limit", "") {
		t.Errorf("empty limit details = %v, want limit error", details)
	}
	if _, details := ParseRidersQuery(queryOf("minRiders=")); !hasMsg(details, "minRiders", "") {
		t.Errorf("empty minRiders details = %v, want error", details)
	}
}
