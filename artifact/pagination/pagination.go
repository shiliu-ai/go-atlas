package pagination

import "github.com/gin-gonic/gin"

const (
	DefaultPage = 1
	DefaultSize = 20
	MaxSize     = 100
)

// Request represents pagination parameters from the client.
type Request struct {
	Page int `form:"page" json:"page"`
	Size int `form:"size" json:"size"`
}

// Normalize ensures page and size are within valid bounds.
func (r *Request) Normalize() {
	if r.Page < 1 {
		r.Page = DefaultPage
	}
	if r.Size < 1 {
		r.Size = DefaultSize
	}
	if r.Size > MaxSize {
		r.Size = MaxSize
	}
}

// Offset returns the SQL-style offset for the current page.
func (r *Request) Offset() int {
	return (r.Page - 1) * r.Size
}

// Response is the paginated result envelope.
type Response struct {
	List  any   `json:"list"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}

// NewResponse creates a paginated response.
func NewResponse(list any, total int64, req Request) *Response {
	return &Response{
		List:  list,
		Total: total,
		Page:  req.Page,
		Size:  req.Size,
	}
}

// FromContext extracts and normalizes pagination parameters from gin.Context.
func FromContext(c *gin.Context) Request {
	var req Request
	_ = c.ShouldBindQuery(&req)
	req.Normalize()
	return req
}
