package ginhttp_test

import (
	"fmt"
	"github.com/opentracing/opentracing-go/ext"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"github.com/zeitlinger/go-gin/ginhttp"
)

const defaultComponentName = "net/http"

type testCase struct {
	name                  string
	handler               gin.HandlerFunc
	options               []ginhttp.MWOption
	expectedOperationName string
	expectedSpanTags      []map[string]interface{}
}

func TestTags(t *testing.T) {
	tests := []testCase{
		{
			name: "OK",
			expectedSpanTags: []map[string]interface{}{
				{
					string(ext.Component):      defaultComponentName,
					string(ext.HTTPMethod):     "GET",
					string(ext.HTTPStatusCode): uint16(http.StatusOK),
					string(ext.SpanKind):       ext.SpanKindRPCServerEnum,
				},
			},
		},
		{
			name: "span observer option",
			options: []ginhttp.MWOption{ginhttp.MWSpanObserver(func(sp opentracing.Span, r *http.Request) {
				sp.SetTag("http.uri", r.URL.EscapedPath())
			})},
			expectedSpanTags: []map[string]interface{}{
				{
					string(ext.Component):      defaultComponentName,
					string(ext.HTTPMethod):     "GET",
					string(ext.HTTPStatusCode): uint16(http.StatusOK),
					"http.uri":                 "/hello",
					string(ext.SpanKind):       ext.SpanKindRPCServerEnum,
				},
			},
		},
		{
			name:    "ComponentName option",
			options: []ginhttp.MWOption{ginhttp.MWComponentName("comp")},
			expectedSpanTags: []map[string]interface{}{
				{
					string(ext.Component):      "comp",
					string(ext.HTTPMethod):     "GET",
					string(ext.HTTPStatusCode): uint16(http.StatusOK),
					string(ext.SpanKind):       ext.SpanKindRPCServerEnum,
				},
			},
		},
		{
			name: "URLTag option",
			options: []ginhttp.MWOption{ginhttp.MWURLTagFunc(func(u *url.URL) string {
				// Log path only (no query parameters etc)
				return u.Path
			})},
			expectedSpanTags: []map[string]interface{}{
				{
					string(ext.Component):      defaultComponentName,
					string(ext.HTTPMethod):     "GET",
					string(ext.HTTPStatusCode): uint16(http.StatusOK),
					string(ext.HTTPUrl):        "/hello",
					string(ext.SpanKind):       ext.SpanKindRPCServerEnum,
				},
			},
		},
		{
			name: "OperationName option",
			options: []ginhttp.MWOption{ginhttp.OperationNameFunc(func(r *http.Request) string {
				return "HTTP " + r.Method + ": /root"
			})},
			expectedOperationName: "HTTP GET: /root",
			expectedSpanTags: []map[string]interface{}{
				{
					string(ext.Component):      defaultComponentName,
					string(ext.HTTPMethod):     "GET",
					string(ext.HTTPStatusCode): uint16(http.StatusOK),
					string(ext.SpanKind):       ext.SpanKindRPCServerEnum,
				},
			},
		},
		{
			name: "Error",
			handler: func(c *gin.Context) {
				c.String(http.StatusInternalServerError, "OK")
			},
			expectedSpanTags: []map[string]interface{}{
				{
					string(ext.Component):      defaultComponentName,
					string(ext.HTTPMethod):     "GET",
					string(ext.HTTPStatusCode): uint16(http.StatusInternalServerError),
					string(ext.SpanKind):       ext.SpanKindRPCServerEnum,
					string(ext.Error):          true,
				},
			},
		},
		{
			name: "Error func option",
			handler: func(c *gin.Context) {
				c.String(http.StatusNotFound, "OK")
			},
			options: []ginhttp.MWOption{ginhttp.MWErrorFunc(func(ctx *gin.Context) bool {
				return ctx.Writer.Status() >= http.StatusNotFound
			})},
			expectedSpanTags: []map[string]interface{}{
				{
					string(ext.Component):      defaultComponentName,
					string(ext.HTTPMethod):     "GET",
					string(ext.HTTPStatusCode): uint16(http.StatusNotFound),
					string(ext.SpanKind):       ext.SpanKindRPCServerEnum,
					string(ext.Error):          true,
				},
			},
		},
		{
			name: "Panic",
			handler: func(c *gin.Context) {
				panic("panic test")
			},
			expectedSpanTags: []map[string]interface{}{
				{
					string(ext.Component):      defaultComponentName,
					string(ext.HTTPMethod):     "GET",
					string(ext.HTTPStatusCode): uint16(http.StatusInternalServerError),
					string(ext.SpanKind):       ext.SpanKindRPCServerEnum,
					string(ext.Error):          true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracer := &mocktracer.MockTracer{}
			if tt.handler == nil {
				tt.handler = func(c *gin.Context) {
					c.String(http.StatusOK, "OK")
				}
			}
			srv := httptest.NewServer(engine(tracer, tt.handler, tt.options))
			defer srv.Close()

			setDefaults(&tt, srv.Listener)

			request, err := http.NewRequest("GET", srv.URL+"/hello?token=secret", nil)
			assert.NoError(t, err)
			_, err = http.DefaultClient.Do(request)
			assert.NoError(t, err)

			var tags []map[string]interface{}

			for _, span := range tracer.FinishedSpans() {
				tags = append(tags, span.Tags())
				assert.Equal(t, tt.expectedOperationName, span.OperationName)
			}
			assert.Equal(t, tt.expectedSpanTags, tags)
		})
	}
}

func setDefaults(tt *testCase, listener net.Listener) {
	if tt.expectedOperationName == "" {
		tt.expectedOperationName = "Hello"
	}

	for _, tags := range tt.expectedSpanTags {
		tags[string(ext.PeerHostIPv4)] = "127.0.0.1"
		if _, ok := tags[string(ext.HTTPUrl)]; !ok {
			tags[string(ext.HTTPUrl)] = fmt.Sprintf("http://%s/hello", listener.Addr())
		}
	}
}

func engine(tracer *mocktracer.MockTracer, h gin.HandlerFunc, options []ginhttp.MWOption) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), ginhttp.Middleware(tracer, options...))
	r.GET("/hello", h)
	return r
}
