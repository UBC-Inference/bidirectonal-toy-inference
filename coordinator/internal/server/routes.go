package server

import (
	"context"
	"net/http"

	"github.com/UBC-Inference/bidirectonal-toy-inference/coordinator/internal/asr"
	"github.com/UBC-Inference/bidirectonal-toy-inference/coordinator/internal/realtime"
	"github.com/gin-gonic/gin"
)

func newRouter(sessionContext context.Context, apiKey string, recognizer asr.Recognizer) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/healthz", func(ginContext *gin.Context) {
		ginContext.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/v1/realtime", requireAuth(apiKey), gin.WrapF(func(responseWriter http.ResponseWriter, request *http.Request) {
		realtime.Handle(sessionContext, recognizer, responseWriter, request)
	}))
	return router
}
