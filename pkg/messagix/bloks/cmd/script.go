package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

type BloksScriptNode struct {
	BloksScriptNodeContent
}

var ParseEndOfFuncall = errors.New("end of funcall")

func (node *BloksScriptNode) ParseAny(code string, start int) (int, error) {
	for idx := start; idx < len(code); idx++ {
		switch code[idx] {
		case '\t', ' ':
			continue
		case '(':
			funcall := BloksScriptFuncall{}
			node.BloksScriptNodeContent = &funcall
			return funcall.Parse(code, idx)
		case ')':
			return idx, ParseEndOfFuncall
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
		case '\t', ' ':
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
		// Apparently comma is allowed in function names ?????????
		if code[idx] == ',' {
			continue
		}
		if code[idx] == ' ' || code[idx] == '(' || code[idx] == ')' {
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
		if errors.Is(err, ParseEndOfFuncall) {
			for idx := next; idx < len(code); idx++ {
				switch code[idx] {
				case '\t', ' ':
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

func (call *BloksScriptFuncall) Print(indent string) error {
	fmt.Printf("%s(%s\n", indent, call.Function)
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
		case '\t', ' ':
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
		if code[idx] == '.' {
			decimal = true
			continue
		}
		if idx == start {
			break
		}
		switch code[idx] {
		case ' ', '(', ')':
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
	return start, fmt.Errorf("unknown char %q", code[start])
}

func (lit *BloksScriptLiteral) Print(indent string) error {
	str, err := json.Marshal(lit.BloksJavascriptValue)
	if err != nil {
		return err
	}
	fmt.Printf("%s%s", indent, str)
	return nil
}
