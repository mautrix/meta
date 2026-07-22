package bloks

import (
	"fmt"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type BloksDoc struct {
	Name        string
	ClientDocID string
	RootField   string
}

var BloksActionDocIOS BloksDoc = BloksDoc{
	Name:        "MSGBloksActionRootQuery",
	ClientDocID: "15577570885164679244236938926",
	RootField:   "bloks_action",
}

var BloksActionDocAndroid BloksDoc = BloksDoc{
	Name:        "MSGBloksActionRootQuery",
	ClientDocID: "11994080429475996425688397159",
	RootField:   "bloks_action",
}

func GetBloksActionDoc(p types.Platform) (*BloksDoc, error) {
	switch p {
	case types.MessengerLiteIOS:
		return &BloksActionDocIOS, nil
	case types.MessengerLiteAndroid:
		return &BloksActionDocAndroid, nil
	default:
		return nil, fmt.Errorf("platform %s doesn't support bloks", p.String())
	}
}

var BloksAppDocIOS BloksDoc = BloksDoc{
	Name:        "MSGBloksAppRootQuery",
	ClientDocID: "28114594639880354457944446921",
	RootField:   "bloks_app",
}

var BloksAppDocAndroid BloksDoc = BloksDoc{
	Name:        "MSGBloksAppRootQuery",
	ClientDocID: "10537346110468165520737721487",
	RootField:   "bloks_app",
}

func GetBloksAppDoc(p types.Platform) (*BloksDoc, error) {
	switch p {
	case types.MessengerLiteIOS:
		return &BloksAppDocIOS, nil
	case types.MessengerLiteAndroid:
		return &BloksAppDocAndroid, nil
	default:
		return nil, fmt.Errorf("platform %s doesn't support bloks", p.String())
	}
}
