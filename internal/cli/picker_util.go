package cli

// clampSel keeps a list selection in bounds, defaulting to the first row when
// the list is empty or the incoming index is invalid.
func clampSel[T any](sel int, items []T) int {
	if len(items) == 0 {
		return 0
	}
	if sel < 0 {
		return 0
	}
	if sel >= len(items) {
		return len(items) - 1
	}
	return sel
}

func trimLastRune(text string) string {
	runes := []rune(text)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}
