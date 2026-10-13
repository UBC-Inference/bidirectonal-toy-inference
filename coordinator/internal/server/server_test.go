package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/UBC-Inference/bidirectonal-toy-inference/coordinator/internal/asr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Coordinator server lifecycle", func() {
	DescribeTable("request tracking", func(contenders int) {
		entered, release := make(chan struct{}, contenders+1), make(chan struct{})
		var releaseOnce sync.Once
		requests := requestTracker{handler: http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
			entered <- struct{}{}
			<-release
			responseWriter.WriteHeader(http.StatusNoContent)
		})}
		statuses := make(chan int, contenders+1)
		var workers sync.WaitGroup
		workers.Add(contenders + 1)
		request := func() {
			defer GinkgoRecover()
			defer workers.Done()
			response := httptest.NewRecorder()
			requests.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
			statuses <- response.Code
		}
		completed := make(chan struct{})
		DeferCleanup(func() {
			releaseOnce.Do(func() { close(release) })
			Eventually(completed, asrTestTimeout).Should(BeClosed())
		})
		go request()
		start, stopped := make(chan struct{}), make(chan struct{})
		var startOnce sync.Once
		for i := 0; i < contenders; i++ {
			go func() { <-start; request() }()
		}
		go func() {
			workers.Wait()
			close(completed)
		}()
		go func() {
			<-start
			requests.stopAccepting()
			close(stopped)
		}()
		DeferCleanup(func() {
			startOnce.Do(func() { close(start) })
			Eventually(stopped, asrTestTimeout).Should(BeClosed())
		})
		Eventually(entered, asrTestTimeout).Should(Receive())
		startOnce.Do(func() { close(start) })
		Eventually(stopped, asrTestTimeout).Should(BeClosed())
		response := httptest.NewRecorder()
		requests.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
		Expect(response.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(response.Body.String()).To(ContainSubstring("server shutting down"))
		waiting, waited := make(chan struct{}), make(chan struct{})
		go func() {
			defer GinkgoRecover()
			close(waiting)
			requests.wait()
			close(waited)
		}()
		Eventually(waiting, asrTestTimeout).Should(BeClosed())
		Consistently(waited, 50*time.Millisecond).ShouldNot(BeClosed())
		releaseOnce.Do(func() { close(release) })
		Eventually(waited, asrTestTimeout).Should(BeClosed())
		Eventually(completed, asrTestTimeout).Should(BeClosed())
		accepted := 0
		for i := 0; i <= contenders; i++ {
			var status int
			Expect(statuses).To(Receive(&status))
			Expect(status).To(BeElementOf(http.StatusNoContent, http.StatusServiceUnavailable))
			if status == http.StatusNoContent {
				accepted++
			}
		}
		Expect(accepted).To(Equal(len(entered) + 1))
	},
		Entry("drains an admitted handler and rejects new requests", 0),
		Entry("serializes concurrent admission against shutdown", 32),
	)

	DescribeTable("router authentication", func(path, authorization string, status int) {
		var streams atomic.Int32
		recognizer := fakeASRRecognizer{newStream: func() (asrStream, error) {
			streams.Add(1)
			return newFakeASRStream(), nil
		}}
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", authorization)
		response := httptest.NewRecorder()
		newRouter(context.Background(), "secret", recognizer).ServeHTTP(response, request)
		Expect(response.Code).To(Equal(status))
		if path == "/healthz" {
			Expect(response.Body.String()).To(MatchJSON(`{"status":"ok"}`))
		}
		Expect(streams.Load()).To(BeZero())
	},
		Entry("health needs no credentials", "/healthz", "", http.StatusOK),
		Entry("realtime rejects missing credentials", "/v1/realtime", "", http.StatusUnauthorized),
		Entry("realtime rejects incorrect credentials", "/v1/realtime", "Bearer wrong", http.StatusUnauthorized),
		Entry("authorized realtime reaches WebSocket upgrade", "/v1/realtime", "Bearer secret", http.StatusBadRequest),
	)

	DescribeTable("configuration", func(port, key, expectedPort string) {
		for name, value := range map[string]string{
			"PORT": port, "COORDINATOR_API_KEY": key,
			"ASR_ENCODER_PATH": "encoder", "ASR_DECODER_PATH": "decoder",
			"ASR_JOINER_PATH": "joiner", "ASR_TOKENS_PATH": "tokens",
		} {
			GinkgoT().Setenv(name, value)
		}
		config, err := loadCoordinatorConfig()
		if key == "" {
			Expect(err).To(MatchError("COORDINATOR_API_KEY is required"))
			Expect(config).To(Equal(coordinatorConfig{}))
			return
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(config).To(Equal(coordinatorConfig{
			port: expectedPort, apiKey: key,
			asr: asr.Config{EncoderPath: "encoder", DecoderPath: "decoder", JoinerPath: "joiner", TokensPath: "tokens"},
		}))
	},
		Entry("defaults the port and maps all ASR paths", "", "secret", "8080"),
		Entry("honors the configured port", "9090", "secret", "9090"),
		Entry("requires an API key", "", "", ""),
	)

	It("returns listener failure without creating an ASR stream", func() {
		// macOS permits loopback and wildcard listeners to share a port.
		listener, err := net.Listen("tcp", ":0")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(listener.Close)
		ctx, cancel := context.WithTimeout(context.Background(), asrTestTimeout)
		DeferCleanup(cancel)
		var streams atomic.Int32
		recognizer := fakeASRRecognizer{newStream: func() (asrStream, error) {
			streams.Add(1)
			return newFakeASRStream(), nil
		}}
		result := make(chan error, 1)
		go func() {
			result <- serveCoordinator(ctx, coordinatorConfig{
				port: strconv.Itoa(listener.Addr().(*net.TCPAddr).Port), apiKey: "secret",
			}, recognizer)
		}()
		Eventually(result, asrTestTimeout).Should(Receive(&err))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("listen:"))
		Expect(errors.Is(err, syscall.EADDRINUSE)).To(BeTrue())
		Expect(streams.Load()).To(BeZero())
	})
})
