package server

import (
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = DescribeTable("bearer authentication middleware", func(header string, status int) {
	router := gin.New()
	router.GET("/protected", requireAuth("secret"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", header)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	Expect(response.Code).To(Equal(status))
	if status == http.StatusUnauthorized {
		Expect(response.Header().Get("WWW-Authenticate")).To(Equal("Bearer"))
	}
},
	Entry("accepts the configured token", "Bearer secret", http.StatusNoContent),
	Entry("rejects missing credentials", "", http.StatusUnauthorized),
	Entry("rejects an incorrect token", "Bearer wrong", http.StatusUnauthorized),
)
