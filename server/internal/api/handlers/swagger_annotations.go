package handlers

// This file contains shared Swagger model definitions and examples

// Common error response
// swagger:model ErrorResponse
type ErrorResponse struct {
	// Error message
	// example: invalid request
	Error string `json:"error"`
}

// Success message response
// swagger:model SuccessResponse
type SuccessResponse struct {
	// Success message
	// example: operation completed successfully
	Message string `json:"message"`
}

// Pagination info
// swagger:model PaginationInfo
type PaginationInfo struct {
	// Total number of items
	// example: 100
	Total int `json:"total"`
	// Number of items per page
	// example: 50
	Limit int `json:"limit"`
	// Number of items to skip
	// example: 0
	Offset int `json:"offset"`
}
