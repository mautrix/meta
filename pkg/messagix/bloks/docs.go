package bloks

import (
	"fmt"

	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

type BloksPlatform int

type BloksDoc struct {
	Name        string
	ClientDocID string
	RootField   string
	UseNT       bool
	Version     string
}

var BloksActionDocIOS BloksDoc = BloksDoc{
	Name:        "BKActionRootQuery",
	ClientDocID: "398678489416863104368276688721",
	RootField:   "bloks_action",
	UseNT:       false,
	Version:     BloksVersionIOS,
}

var BloksActionDocAndroid BloksDoc = BloksDoc{
	Name:        "MSGBloksActionRootQuery",
	ClientDocID: "11994080429475996425688397159",
	RootField:   "bloks_action",
	UseNT:       true,
	Version:     BloksVersionAndroid,
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
	Name:        "BKAppRootQuery",
	ClientDocID: "25250723151340651749066595013",
	RootField:   "bloks_app",
	UseNT:       false,
	Version:     BloksVersionIOS,
}

var BloksAppDocAndroid BloksDoc = BloksDoc{
	Name:        "MSGBloksAppRootQuery",
	ClientDocID: "10537346110468165520737721487",
	RootField:   "bloks_app",
	UseNT:       true,
	Version:     BloksVersionAndroid,
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
