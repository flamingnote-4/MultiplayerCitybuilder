package main

import (
	"time"

	gui "github.com/gen2brain/raylib-go/raygui"
	rl "github.com/gen2brain/raylib-go/raylib"
)

type CustomTextBox struct {
	Rect        rl.Rectangle
	Text        string
	MaxLength   int
	CursorPos   int
	Focused     bool
	ShowCursor  bool
	CursorBlink time.Time
}

func NewCustomTextBox(x, y, w, h float32, maxLen int) *CustomTextBox {
	return &CustomTextBox{
		Rect:        rl.NewRectangle(x, y, w, h),
		MaxLength:   maxLen,
		CursorBlink: time.Now(),
		ShowCursor:  true,
	}
}

func (tb *CustomTextBox) Update() bool {
	mousePos := rl.GetMousePosition()
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		tb.Focused = rl.CheckCollisionPointRec(mousePos, tb.Rect)
		if tb.Focused {

			relativeX := mousePos.X - (tb.Rect.X + 5)

			tb.CursorPos = 0
			for i := 1; i <= len(tb.Text); i++ {
				width := float32(rl.MeasureText(tb.Text[:i], 20))
				if width >= relativeX {
					tb.CursorPos = i - 1
					break
				}
				tb.CursorPos = i
			}
			if tb.CursorPos > len(tb.Text) {
				tb.CursorPos = len(tb.Text)
			}
		}
	}

	if !tb.Focused {
		tb.ShowCursor = false
		return false
	}

	changed := false

	for {
		key := rl.GetCharPressed()
		if key == 0 {
			break
		}
		if len(tb.Text) < tb.MaxLength {
			r := rune(key)

			tb.Text = tb.Text[:tb.CursorPos] + string(r) + tb.Text[tb.CursorPos:]
			tb.CursorPos++
			changed = true
		}
	}

	if rl.IsKeyPressed(rl.KeyBackspace) {
		if tb.CursorPos > 0 && len(tb.Text) > 0 {
			tb.Text = tb.Text[:tb.CursorPos-1] + tb.Text[tb.CursorPos:]
			tb.CursorPos--
			changed = true
		}
	}
	if rl.IsKeyPressed(rl.KeyDelete) {
		if tb.CursorPos < len(tb.Text) {
			tb.Text = tb.Text[:tb.CursorPos] + tb.Text[tb.CursorPos+1:]
			changed = true
		}
	}
	if rl.IsKeyPressed(rl.KeyLeft) {
		if tb.CursorPos > 0 {
			tb.CursorPos--
		}
	}
	if rl.IsKeyPressed(rl.KeyRight) {
		if tb.CursorPos < len(tb.Text) {
			tb.CursorPos++
		}
	}

	if time.Since(tb.CursorBlink) > 500*time.Millisecond {
		tb.ShowCursor = !tb.ShowCursor
		tb.CursorBlink = time.Now()
	}

	return changed
}

func (tb *CustomTextBox) Draw() {

	rl.DrawRectangleRec(tb.Rect, rl.White)
	rl.DrawRectangleLinesEx(tb.Rect, 2, rl.Gray)

	textX := tb.Rect.X + 5
	textY := tb.Rect.Y + (tb.Rect.Height / 2) - 10

	gui.Label(rl.NewRectangle(float32(textX), float32(textY), 200, 20), tb.Text)

	if tb.Focused && tb.ShowCursor {
		substr := tb.Text[:tb.CursorPos]
		measure := rl.MeasureTextEx(textFont, substr, 28.0, 0.0)
		cursorX := textX + float32(measure.X)
		cursorY := textY

		rl.DrawLine(int32(cursorX), int32(cursorY), int32(cursorX), int32(cursorY+20), rl.Black)
	}
}
