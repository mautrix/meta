package main

import "fmt"

type BloksScriptID string
type BloksDatumID string
type BloksPayloadID string
type BloksTemplateID string
type BloksComponentID string
type BloksClassID string
type BloksAttributeID string
type BloksFunctionID string

func (i BloksAttributeID) ToInt() (int, error) {
	s := []rune(i)
	if len(s) != 1 {
		return 0, fmt.Errorf("bad attribute length %q", s)
	}
	if s[0] < ' ' || s[0] > '\u00ff' {
		return 0, fmt.Errorf("bad attribute char")
	}
	return int(s[0] - ' '), nil
}
