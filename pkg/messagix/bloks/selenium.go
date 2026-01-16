package bloks

import (
	"context"
	"fmt"
)

func (bb *BloksBundle) FindDescendant(pred func(*BloksTreeComponent) bool) *BloksTreeComponent {
	return bb.Layout.Payload.Tree.FindDescendant(pred)
}

func (btn *BloksTreeNode) FindDescendant(pred func(*BloksTreeComponent) bool) *BloksTreeComponent {
	if comp, ok := btn.BloksTreeNodeContent.(*BloksTreeComponent); ok {
		return comp.FindDescendant(pred)
	}
	if comps, ok := btn.BloksTreeNodeContent.(*BloksTreeComponentList); ok {
		for _, comp := range *comps {
			if match := comp.FindDescendant(pred); match != nil {
				return match
			}
		}
	}
	return nil
}

func (comp *BloksTreeComponent) FindDescendant(pred func(*BloksTreeComponent) bool) *BloksTreeComponent {
	if pred(comp) {
		return comp
	}
	for _, subnode := range comp.Attributes {
		if match := subnode.FindDescendant(pred); match != nil {
			return match
		}
	}
	return nil
}

func (comp *BloksTreeComponent) FindAncestor(pred func(*BloksTreeComponent) bool) *BloksTreeComponent {
	for comp != nil {
		if pred(comp) {
			return comp
		}
		comp = comp.parent
	}
	return nil
}

func (comp *BloksTreeComponent) FindCousin(pred func(*BloksTreeComponent) bool) (found *BloksTreeComponent) {
	comp.FindAncestor(func(comp *BloksTreeComponent) bool {
		found = comp.FindDescendant(pred)
		return found != nil
	})
	return
}

func (comp *BloksTreeComponent) FindContainingButton() *BloksTreeComponent {
	return comp.FindCousin(func(comp *BloksTreeComponent) bool {
		return comp.ComponentID == "bk.components.FoaTouchExtension"
	})
}

func FilterByAttribute(compid BloksComponentID, attr string, value string) func(comp *BloksTreeComponent) bool {
	return func(comp *BloksTreeComponent) bool {
		if comp.ComponentID != compid {
			return false
		}
		return comp.GetAttribute(attr) == value
	}
}

func (comp *BloksTreeComponent) GetAttribute(name string) string {
	value, ok := comp.Attributes[BloksAttributeID(name)].BloksTreeNodeContent.(*BloksTreeLiteral)
	if !ok {
		return ""
	}
	str, ok := value.BloksJavascriptValue.(string)
	if !ok {
		return ""
	}
	return str
}

func (input *BloksTreeComponent) FillInput(ctx context.Context, interp *Interpreter, text string) error {
	if input == nil {
		return fmt.Errorf("no such input")
	}
	err := input.SetTextContent(text)
	if err != nil {
		return err
	}
	onChanged, ok := input.Attributes["on_text_change"].BloksTreeNodeContent.(*BloksTreeScript)
	if !ok {
		return fmt.Errorf("no on_text_change script")
	}
	_, err = interp.Evaluate(InterpBindThis(ctx, input), &onChanged.AST)
	if err != nil {
		return fmt.Errorf("on_text_change: %w", err)
	}
	return err
}

func (button *BloksTreeComponent) TapButton(ctx context.Context, interp *Interpreter) error {
	if button == nil {
		return fmt.Errorf("no such button")
	}
	onTouchDown, ok := button.Attributes["on_touch_down"].BloksTreeNodeContent.(*BloksTreeScript)
	if !ok {
		return fmt.Errorf("no on_touch_down script")
	}
	onTouchUp, ok := button.Attributes["on_touch_up"].BloksTreeNodeContent.(*BloksTreeScript)
	if !ok {
		return fmt.Errorf("no on_touch_up script")
	}
	_, err := interp.Evaluate(InterpBindThis(ctx, button), &onTouchDown.AST)
	if err != nil {
		return fmt.Errorf("on_touch_down: %w", err)
	}
	_, err = interp.Evaluate(InterpBindThis(ctx, button), &onTouchUp.AST)
	if err != nil {
		return fmt.Errorf("on_touch_up: %w", err)
	}
	return nil
}
