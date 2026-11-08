package health

import (
	"net/http"

	"github.com/gin-gonic/gin"

	rhandler "github.com/Dopesonic/order-service/internal/app/handler/http"
)

type handler struct{}

func NewHandler() rhandler.Health {
	return &handler{}
}

func (h *handler) LastCheck(c *gin.Context) {
	c.String(http.StatusOK, "OK")
}
