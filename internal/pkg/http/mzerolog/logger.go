package mzerolog

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/Dopesonic/order-service/internal/pkg/http/httph"
)

type middleware struct {
	log zerolog.Logger

	fromOptions struct {
		skipper func(r *http.Request) bool
	}
}

func (m *middleware) Callback(c *gin.Context) {
	const (
		tailSuccess = " finished with no error"
		tailFail    = " finished (or aborted) with error"
	)

	start := time.Now()
	c.Next()

	r := c.Request
	err := httph.ErrorGet(r)
	execTime := time.Since(start)

	if m.fromOptions.skipper(r) {
		return
	}

	var mb strings.Builder
	mb.Grow(48 + len(r.RequestURI))
	mb.WriteString(r.Method)
	mb.WriteByte(' ')
	mb.WriteString(r.RequestURI)

	var ev *zerolog.Event
	if err == nil {
		mb.WriteString(tailSuccess)
		ev = m.log.Debug()
	} else {
		mb.WriteString(tailFail)
		ev = m.log.Error()
	}

	ev.Err(err).
		Ctx(r.Context()).
		Str("exec_time", execTime.String()).
		Str("client_ip", c.ClientIP())

	if status := httph.ErrorGetStatusCode(r); status > 0 {
		ev.Int("http_status_code", status)
	}

	ev.Msg(mb.String())
}

func NewMiddleware(opts ...Option) gin.HandlerFunc {
	m := middleware{
		log: log.Logger,
	}
	m.fromOptions.skipper = defaultSkipper

	for _, opt := range opts {
		opt(&m)
	}

	return m.Callback
}

func defaultSkipper(_ *http.Request) bool {
	return false
}
