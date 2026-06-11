package httpapi

import "strconv"

// parsePage normalizes the cursor + limit query params: cursor decodes to a
// decimal int64 (0 = first page), limit clamps to [1,100] with default 20.
func parsePage(cursorP *Cursor, limitP *Limit) (int64, int) {
	limit := 20
	if limitP != nil && *limitP > 0 && *limitP <= 100 {
		limit = *limitP
	}
	var cursor int64
	if cursorP != nil && *cursorP != "" {
		cursor, _ = strconv.ParseInt(*cursorP, 10, 64)
	}
	return cursor, limit
}
