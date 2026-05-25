package response

import "github.com/gin-gonic/gin"

// Body 统一响应结构
type Body struct {
	Code int         `json:"code"` // 0 成功，非 0 失败
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

// OK 成功响应
func OK(c *gin.Context, data interface{}) {
	c.JSON(200, Body{Code: 0, Msg: "ok", Data: data})
}

// Page 分页响应
func Page(c *gin.Context, total int64, items interface{}) {
	c.JSON(200, Body{Code: 0, Msg: "ok", Data: gin.H{"total": total, "items": items}})
}

// Fail 失败响应
func Fail(c *gin.Context, httpStatus int, msg string) {
	c.JSON(httpStatus, Body{Code: httpStatus, Msg: msg})
}

// BadRequest 参数错误
func BadRequest(c *gin.Context, msg string) {
	Fail(c, 400, msg)
}

// ServerError 服务器错误
func ServerError(c *gin.Context, msg string) {
	Fail(c, 500, msg)
}
