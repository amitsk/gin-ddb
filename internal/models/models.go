// Package models holds the three SkiLifts record kinds per spec §3.
// Field names and requiredness are locked; schemas enforce them.
package models

// StaticRecord is the fixed properties of one lift (Metadata = "static").
type StaticRecord struct {
	Lift         string `json:"Lift" dynamodbav:"Lift"`
	Metadata     string `json:"Metadata" dynamodbav:"Metadata"`
	IsDetachable bool   `json:"IsDetachable" dynamodbav:"IsDetachable"`
	VerticalFeet int    `json:"VerticalFeet" dynamodbav:"VerticalFeet"`
	LiftTime     string `json:"LiftTime" dynamodbav:"LiftTime"`
}

// DynamicRecord is one day snapshot for one lift (Metadata = MM/DD/YY).
type DynamicRecord struct {
	Lift                  string `json:"Lift" dynamodbav:"Lift"`
	Metadata              string `json:"Metadata" dynamodbav:"Metadata"`
	TotalUniqueLiftRiders int    `json:"TotalUniqueLiftRiders" dynamodbav:"TotalUniqueLiftRiders"`
	SnowCount             int    `json:"SnowCount" dynamodbav:"SnowCount"`
	LiftStatus            string `json:"LiftStatus" dynamodbav:"LiftStatus"`
	AvalancheDanger       string `json:"AvalancheDanger" dynamodbav:"AvalancheDanger"`
}

// ResortRecord is the aggregate for one resort area (Metadata = "resort").
type ResortRecord struct {
	Lift            string `json:"Lift" dynamodbav:"Lift"`
	Metadata        string `json:"Metadata" dynamodbav:"Metadata"`
	TotalRiders     int    `json:"TotalRiders" dynamodbav:"TotalRiders"`
	TotalSnow       int    `json:"TotalSnow" dynamodbav:"TotalSnow"`
	AvalancheDanger string `json:"AvalancheDanger" dynamodbav:"AvalancheDanger"`
	OpenLifts       []int  `json:"OpenLifts" dynamodbav:"OpenLifts"`
}

// Key returns the DynamoDB key pair shared by all record kinds.
func Key(lift, metadata string) map[string]string {
	return map[string]string{"Lift": lift, "Metadata": metadata}
}
