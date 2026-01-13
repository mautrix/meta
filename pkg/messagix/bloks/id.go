package bloks

import "fmt"

type BloksScriptID string
type BloksPayloadID string
type BloksTemplateID string
type BloksComponentID string
type BloksClassID string
type BloksAttributeID string
type BloksFunctionID string
type BloksVariableID string

func (i BloksAttributeID) ToInt() (int, bool) {
	s := []rune(i)
	if len(s) != 1 {
		return 0, false
	}
	if s[0] < ' ' || s[0] > '\u00ff' {
		return 0, false
	}
	return int(s[0] - ' '), true
}

func (i BloksAttributeID) ToTag() string {
	idx, ok := i.ToInt()
	if ok {
		return fmt.Sprintf("index=%d", idx)
	}
	return fmt.Sprintf("name=%s", i)
}
