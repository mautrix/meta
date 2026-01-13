package bloks

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

type BloksScriptNode struct {
	BloksScriptNodeContent
}

var ErrParseEndOfFuncall = errors.New("end of funcall")

func (node *BloksScriptNode) ParseAny(code string, start int) (int, error) {
	for idx := start; idx < len(code); idx++ {
		switch code[idx] {
		case '\t', ' ', ',':
			continue
		case '(':
			funcall := BloksScriptFuncall{}
			node.BloksScriptNodeContent = &funcall
			return funcall.Parse(code, idx)
		case ')':
			return idx, ErrParseEndOfFuncall
		default:
			var literal BloksScriptLiteral
			node.BloksScriptNodeContent = &literal
			return literal.Parse(code, idx)
		}
	}
	return len(code), fmt.Errorf("eof at toplevel")
}

type BloksScriptNodeContent interface {
	Parse(code string, start int) (int, error)
	Unminify(m *Unminifier)
	Print(indent string) error
}

type BloksScriptFuncall struct {
	Function BloksFunctionID
	Args     []BloksScriptNode
}

func (call *BloksScriptFuncall) Parse(code string, start int) (int, error) {
	start += 1
	for idx := start; idx < len(code); idx++ {
		switch code[idx] {
		case '\t', ' ', ',':
			continue
		}
		start = idx
		break
	}
	end := start
	for idx := start; idx < len(code); idx++ {
		if code[idx] >= 'a' && code[idx] <= 'z' {
			continue
		}
		if code[idx] >= 'A' && code[idx] <= 'Z' {
			continue
		}
		if code[idx] >= '0' && code[idx] <= '9' {
			continue
		}
		// Some more chars are used in the unminified version
		if code[idx] == '.' || code[idx] == '_' {
			continue
		}
		if code[idx] == ' ' || code[idx] == '(' || code[idx] == ')' || code[idx] == ',' {
			if idx == start {
				return idx, fmt.Errorf("bad char %q instead of func name", code[idx])
			}
			end = idx
			break
		}
		return idx, fmt.Errorf("unexpected char %q in func name %q", code[idx], code[start:idx])
	}
	if start == end {
		return len(code), fmt.Errorf("eof during func name")
	}
	call.Function = BloksFunctionID(code[start:end])
	call.Args = []BloksScriptNode{}
	next := end
	for {
		arg := BloksScriptNode{}
		var err error
		next, err = arg.ParseAny(code, next)
		if errors.Is(err, ErrParseEndOfFuncall) {
			for idx := next; idx < len(code); idx++ {
				switch code[idx] {
				case '\t', ' ', ',':
					continue
				case ')':
					return idx + 1, nil
				}
				return idx, fmt.Errorf("eof during close paren")
			}
		}
		if err != nil {
			return next, err
		}
		call.Args = append(call.Args, arg)
	}
}

func (call *BloksScriptFuncall) Unminify(m *Unminifier) {
	if real, ok := m.Functions[call.Function]; ok && len(real) > 0 {
		call.Function = real
	}
	for _, arg := range call.Args {
		arg.Unminify(m)
	}
}

func (call *BloksScriptFuncall) Print(indent string) error {
	fmt.Printf("%s(%s", indent, call.Function)
	if len(call.Args) >= 1 {
		fmt.Printf("\n")
	}
	for idx, arg := range call.Args {
		if idx > 0 {
			fmt.Printf("\n")
		}
		arg.Print(indent + "  ")
	}
	fmt.Printf(")")
	return nil
}

type BloksScriptLiteral struct {
	BloksJavascriptValue
}

func (lit *BloksScriptLiteral) Parse(code string, start int) (int, error) {
	for idx := start; idx < len(code); idx++ {
		switch code[idx] {
		case '\t', ' ', ',':
			continue
		}
		start = idx
		break
	}
	decimal := false
	for idx := start; idx < len(code); idx++ {
		if code[idx] >= '0' && code[idx] <= '9' {
			continue
		}
		if code[idx] == '-' {
			continue
		}
		if code[idx] == '.' {
			decimal = true
			continue
		}
		if idx == start {
			break
		}
		switch code[idx] {
		case ' ', '(', ')', ',':
			if decimal {
				val, err := strconv.ParseFloat(code[start:idx], 64)
				if err != nil {
					return idx, err
				}
				lit.BloksJavascriptValue = val
			} else {
				val, err := strconv.ParseInt(code[start:idx], 10, 64)
				if err != nil {
					return idx, err
				}
				lit.BloksJavascriptValue = val
			}
			return idx, nil
		}
		return idx, fmt.Errorf("unexpected char %q in numeric literal", code[idx])
	}
	if code[start] == '"' {
		idx := start + 1
		chars := []byte{}
		for idx < len(code) {
			switch code[idx] {
			case '\\':
				if idx+1 >= len(code) {
					return len(code), fmt.Errorf("backslash at eof")
				}
				// feel free to implement escape sequences properly if you wanna
				chars = append(chars, code[idx+1])
				idx += 2
				continue
			case '"':
				lit.BloksJavascriptValue = string(chars)
				return idx + 1, nil
			}
			chars = append(chars, code[idx])
			idx += 1
		}
		return idx, fmt.Errorf("unterminated string literal")
	}
	if start+4 < len(code) && code[start:start+4] == "null" {
		return start + 4, nil
	}
	if start+4 < len(code) && code[start:start+4] == "true" {
		lit.BloksJavascriptValue = true
		return start + 4, nil
	}
	if start+5 < len(code) && code[start:start+5] == "false" {
		lit.BloksJavascriptValue = false
		return start + 5, nil
	}
	fmt.Printf("context: %s\n", code)
	return start, fmt.Errorf("unknown char %q", code[start])
}

func (lit *BloksScriptLiteral) Unminify(m *Unminifier) {
	str, ok := lit.Value().(string)
	if !ok {
		return
	}
	if real, ok := m.Variables[BloksVariableID(str)]; ok && len(real) > 0 {
		lit.BloksJavascriptValue = string(real)
	}
}

func (lit *BloksScriptLiteral) Print(indent string) error {
	str, err := json.Marshal(lit.BloksJavascriptValue)
	if err != nil {
		return err
	}
	fmt.Printf("%s%s", indent, str)
	return nil
}

func (lit *BloksScriptLiteral) Value() any {
	return lit.BloksJavascriptValue
}

func BloksLiteralOf(val any) *BloksScriptLiteral {
	if _, ok := val.(*BloksScriptLiteral); ok {
		panic("logic error, constructing nested literal")
	}
	return &BloksScriptLiteral{
		BloksJavascriptValue(val),
	}
}

var BloksNull = BloksLiteralOf(nil)

type BloksIllegalValue struct{}

var BloksNothing = BloksLiteralOf(&BloksIllegalValue{})

func (lit *BloksScriptLiteral) IsTruthy() bool {
	switch val := lit.BloksJavascriptValue.(type) {
	case bool:
		return val
	case string:
		return len(val) > 0
	case int:
		return val != 0
	case nil:
		return false
	}
	return true
}

func (lit *BloksScriptLiteral) Flatten(facebookify bool) any {
	switch lit := lit.Value().(type) {
	case map[string]*BloksScriptLiteral:
		res := map[string]any{}
		for key, val := range lit {
			res[key] = val.Flatten(facebookify)
		}
		return res
	case []*BloksScriptLiteral:
		res := []any{}
		for _, val := range lit {
			res = append(res, val)
		}
		return res
	case bool:
		if !facebookify {
			return lit
		}
		if lit {
			return 1
		}
		return 0
	default:
		return lit
	}
}
