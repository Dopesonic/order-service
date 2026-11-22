package rprocessor

import (
	"net/http"
	"net/http/pprof"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	rhandler "github.com/Dopesonic/order-service/internal/app/handler/http"
)

func vGenericRegHealthCheck(r *gin.Engine, h rhandler.Health) {
	r.GET("/health", h.LastCheck)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
}

func vGenericRegProf(r *gin.Engine) {
	r.GET("/debug/pprof/", gin.WrapF(pprof.Index))
	r.GET("/debug/pprof/cmdline", gin.WrapF(pprof.Cmdline))
	r.GET("/debug/pprof/profile", gin.WrapF(pprof.Profile))
	r.GET("/debug/pprof/symbol", gin.WrapF(pprof.Symbol))
	r.POST("/debug/pprof/symbol", gin.WrapF(pprof.Symbol))
	r.GET("/debug/pprof/trace", gin.WrapF(pprof.Trace))

	for _, name := range []string{"allocs", "block", "goroutine", "heap", "mutex", "threadcreate"} {
		r.GET("/debug/pprof/"+name, gin.WrapH(pprof.Handler(name)))
	}
}

func handleNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
}
