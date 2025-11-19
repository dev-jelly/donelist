package timeline

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// Cursor represents a time-based pagination cursor
type Cursor struct {
	Time   time.Time `json:"time"`   // Time of the last item in the current page
	Offset int       `json:"offset"` // Offset within items at the same time (for tie-breaking)
}

// EncodeCursor encodes a cursor to a base64 string
func EncodeCursor(c *Cursor) (string, error) {
	if c == nil {
		return "", nil
	}

	data, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("failed to marshal cursor: %w", err)
	}

	return base64.URLEncoding.EncodeToString(data), nil
}

// DecodeCursor decodes a base64 cursor string
func DecodeCursor(encoded string) (*Cursor, error) {
	if encoded == "" {
		return nil, nil
	}

	data, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to decode cursor: %w", err)
	}

	var cursor Cursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cursor: %w", err)
	}

	return &cursor, nil
}

// PaginatedEnhancedDayView represents a paginated enhanced day view
type PaginatedEnhancedDayView struct {
	*EnhancedDayView
	Pagination *PaginationMeta `json:"pagination"`
}

// PaginationMeta contains pagination metadata
type PaginationMeta struct {
	Limit      int     `json:"limit"`                 // Items per page
	HasNext    bool    `json:"has_next"`              // Whether there are more pages
	NextCursor *string `json:"next_cursor,omitempty"` // Cursor for next page
	TotalCount int     `json:"total_count"`           // Total number of items (blocks or checkins)
}

// PaginateBlocks paginates time blocks based on cursor and limit
func PaginateBlocks(blocks []*TimeBlock, cursorStr string, limit int) ([]*TimeBlock, *PaginationMeta, error) {
	if limit <= 0 {
		limit = 48 // Default: half-hour blocks for a day
	}

	// Decode cursor if provided
	cursor, err := DecodeCursor(cursorStr)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid cursor: %w", err)
	}

	// Find starting position
	startIdx := 0
	if cursor != nil {
		// Find the first block after the cursor time
		for i, block := range blocks {
			if block.StartTime.After(cursor.Time) ||
				(block.StartTime.Equal(cursor.Time) && i > cursor.Offset) {
				startIdx = i
				break
			}
		}
	}

	// Check if we're at the end
	if startIdx >= len(blocks) {
		return []*TimeBlock{}, &PaginationMeta{
			Limit:      limit,
			HasNext:    false,
			TotalCount: len(blocks),
		}, nil
	}

	// Calculate end index
	endIdx := startIdx + limit
	hasNext := endIdx < len(blocks)
	if endIdx > len(blocks) {
		endIdx = len(blocks)
	}

	// Get the page of blocks
	pageBlocks := blocks[startIdx:endIdx]

	// Create next cursor if there are more blocks
	var nextCursor *string
	if hasNext && len(pageBlocks) > 0 {
		lastBlock := pageBlocks[len(pageBlocks)-1]
		nextCur := &Cursor{
			Time:   lastBlock.StartTime,
			Offset: endIdx - 1, // Position of the last block
		}
		encoded, err := EncodeCursor(nextCur)
		if err == nil {
			nextCursor = &encoded
		}
	}

	meta := &PaginationMeta{
		Limit:      limit,
		HasNext:    hasNext,
		NextCursor: nextCursor,
		TotalCount: len(blocks),
	}

	return pageBlocks, meta, nil
}

// PaginateCheckins paginates check-ins within a day based on cursor and limit
func PaginateCheckins(checkins []*CheckinWithMeta, cursorStr string, limit int) ([]*CheckinWithMeta, *PaginationMeta, error) {
	if limit <= 0 {
		limit = 50 // Default limit for check-ins
	}

	// Decode cursor if provided
	cursor, err := DecodeCursor(cursorStr)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid cursor: %w", err)
	}

	// Find starting position
	startIdx := 0
	if cursor != nil {
		for i, c := range checkins {
			if c.CheckinTime.After(cursor.Time) ||
				(c.CheckinTime.Equal(cursor.Time) && i > cursor.Offset) {
				startIdx = i
				break
			}
		}
	}

	// Check if we're at the end
	if startIdx >= len(checkins) {
		return []*CheckinWithMeta{}, &PaginationMeta{
			Limit:      limit,
			HasNext:    false,
			TotalCount: len(checkins),
		}, nil
	}

	// Calculate end index
	endIdx := startIdx + limit
	hasNext := endIdx < len(checkins)
	if endIdx > len(checkins) {
		endIdx = len(checkins)
	}

	// Get the page
	pageCheckins := checkins[startIdx:endIdx]

	// Create next cursor
	var nextCursor *string
	if hasNext && len(pageCheckins) > 0 {
		lastCheckin := pageCheckins[len(pageCheckins)-1]
		nextCur := &Cursor{
			Time:   lastCheckin.CheckinTime,
			Offset: endIdx - 1,
		}
		encoded, err := EncodeCursor(nextCur)
		if err == nil {
			nextCursor = &encoded
		}
	}

	meta := &PaginationMeta{
		Limit:      limit,
		HasNext:    hasNext,
		NextCursor: nextCursor,
		TotalCount: len(checkins),
	}

	return pageCheckins, meta, nil
}
