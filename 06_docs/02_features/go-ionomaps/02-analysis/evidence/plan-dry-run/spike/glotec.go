// PLAN dry-run spike (watchpost 0.19.0, D-124): placeholder physics of equivalent cost, written to measure
// cost only. Not the method go-ionomaps implements; see 07-readiness/dry-run.md in watchpost.
package spike

import "encoding/json"

// Typed GloTEC decode: only the fields the app needs.
type GloTEC struct {
	TimeTag  string          `json:"time_tag"`
	Features []GloTECFeature `json:"features"`
}

type GloTECFeature struct {
	Geometry struct {
		Coordinates [2]float64 `json:"coordinates"`
	} `json:"geometry"`
	Properties struct {
		NmF2        float64 `json:"NmF2"`
		HmF2        float64 `json:"hmF2"`
		QualityFlag int     `json:"quality_flag"`
	} `json:"properties"`
}

func DecodeTyped(data []byte, out *GloTEC) error { return json.Unmarshal(data, out) }

func DecodeGeneric(data []byte) (map[string]any, error) {
	var m map[string]any
	err := json.Unmarshal(data, &m)
	return m, err
}
