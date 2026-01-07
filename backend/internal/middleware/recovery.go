package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"tuyul/backend/internal/util"
	"tuyul/backend/pkg/logger"

	"github.com/gin-gonic/gin"
)

// Recovery middleware recovers from panics and returns a 500 error
func Recovery(log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Get request ID
				requestID, _ := c.Get("request_id")

				// Log the panic with stack trace
				log.WithFields(map[string]interface{}{
					"request_id": requestID,
					"panic":      err,
					"stack":      string(debug.Stack()),
				}).Error("Panic recovered", fmt.Errorf("%v", err))

				// Return error response
				util.SendCustomError(c, http.StatusInternalServerError,
					util.ErrCodeInternal, "Internal server error")

				c.Abort()
			}
		}()

		c.Next()
	}
}

