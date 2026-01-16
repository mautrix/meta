package bloks

import (
	_ "embed"
	"encoding/json"
)

//go:embed minify.json
var unminifierJson []byte

type Unminifier struct {
	Functions  map[BloksFunctionID]BloksFunctionID                        `json:"functions"`
	Components map[BloksComponentID]BloksComponentID                      `json:"components"`
	Properties map[BloksComponentID]map[BloksAttributeID]BloksAttributeID `json:"properties"`

	Variables map[BloksVariableID]BloksVariableID `json:"-"`
}

var cachedUnminifier *Unminifier

func GetUnminifier(bundle *BloksBundle) (*Unminifier, error) {
	if cachedUnminifier == nil {
		var u Unminifier
		err := json.Unmarshal(unminifierJson, &u)
		if err != nil {
			return nil, err
		}
		cachedUnminifier = &u
	}
	u := *cachedUnminifier
	u.Variables = map[BloksVariableID]BloksVariableID{}
	for _, datum := range bundle.Layout.Payload.Variables {
		if len(datum.Info.Name) > 0 {
			u.Variables[datum.ID] = BloksVariableID(datum.Info.Name)
		}
	}
	return &u, nil
}
