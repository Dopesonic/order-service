package rhandler

import "github.com/gin-gonic/gin"

type Health interface {
	LastCheck(*gin.Context)
}
