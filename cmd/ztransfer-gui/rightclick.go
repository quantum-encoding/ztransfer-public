package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// rightClickable wraps any canvas object so it can respond to right-click (secondary tap).
type rightClickable struct {
	widget.BaseWidget
	child     fyne.CanvasObject
	onRightTap func(pos fyne.Position, abs fyne.Position)
}

func newRightClickable(child fyne.CanvasObject, onRightTap func(pos fyne.Position, abs fyne.Position)) *rightClickable {
	r := &rightClickable{child: child, onRightTap: onRightTap}
	r.ExtendBaseWidget(r)
	return r
}

func (r *rightClickable) CreateRenderer() fyne.WidgetRenderer {
	return &rightClickableRenderer{obj: r}
}

func (r *rightClickable) TappedSecondary(ev *fyne.PointEvent) {
	if r.onRightTap != nil {
		r.onRightTap(ev.Position, ev.AbsolutePosition)
	}
}

type rightClickableRenderer struct {
	obj *rightClickable
}

func (r *rightClickableRenderer) Layout(size fyne.Size) {
	r.obj.child.Resize(size)
	r.obj.child.Move(fyne.NewPos(0, 0))
}

func (r *rightClickableRenderer) MinSize() fyne.Size {
	return r.obj.child.MinSize()
}

func (r *rightClickableRenderer) Refresh() {
	r.obj.child.Refresh()
}

func (r *rightClickableRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.obj.child}
}

func (r *rightClickableRenderer) Destroy() {}
