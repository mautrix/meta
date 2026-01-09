package main

type Interpreter struct {
	//
}

func NewInterpreter(b *BloksBundle) *Interpreter {
	return &Interpreter{}
}

func (i *Interpreter) Execute(form *BloksScriptNode) (*BloksScriptLiteral, error) {
	return nil, nil
}
