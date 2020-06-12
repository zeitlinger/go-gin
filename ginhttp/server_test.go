package ginhttp

import (
	"github.com/opentracing/opentracing-go/ext"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
)

func TestTags(t *testing.T) {
	tests := []struct {
		name                  string
		handler               gin.HandlerFunc
		options               []MWOption
		expectedOperationName string
		expectedSpanTags      []map[string]interface{}
	}{
		{
			name: "OK",
			handler: func(c *gin.Context) {
				c.String(http.StatusOK, "OK")
			},
			expectedSpanTags: []map[string]interface{}{
				{
					string(ext.Component):      defaultComponentName,
					string(ext.HTTPMethod):     "GET",
					string(ext.HTTPStatusCode): uint16(http.StatusOK),
					string(ext.HTTPUrl):        "/hello?token=secret",
					string(ext.SpanKind):       ext.SpanKindRPCServerEnum,
				},
			},
		},
		{
			name: "span observer option",
			handler: func(c *gin.Context) {
				c.String(http.StatusOK, "OK")
			},
			options: []MWOption{MWSpanObserver(func(sp opentracing.Span, r *http.Request) {
				sp.SetTag("http.uri", r.URL.EscapedPath())
			})},
			expectedSpanTags: []map[string]interface{}{
				{
					string(ext.Component):      defaultComponentName,
					string(ext.HTTPMethod):     "GET",
					string(ext.HTTPStatusCode): uint16(http.StatusOK),
					string(ext.HTTPUrl):        "/hello?token=secret",
					"http.uri":                 "/hello",
					string(ext.SpanKind):       ext.SpanKindRPCServerEnum,
				},
			},
		},
		{
			name: "ComponentName option",
			handler: func(c *gin.Context) {
				c.String(http.StatusOK, "OK")
			},
			options: []MWOption{MWComponentName("comp")},
			expectedSpanTags: []map[string]interface{}{
				{
					string(ext.Component):      "comp",
					string(ext.HTTPMethod):     "GET",
					string(ext.HTTPStatusCode): uint16(http.StatusOK),
					string(ext.HTTPUrl):        "/hello?token=secret",
					string(ext.SpanKind):       ext.SpanKindRPCServerEnum,
				},
			},
		},
		{
			name: "URLTag option",
			handler: func(c *gin.Context) {
				c.String(http.StatusOK, "OK")
			},
			options: []MWOption{MWURLTagFunc(func(u *url.URL) string {
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
			handler: func(c *gin.Context) {
				c.String(http.StatusOK, "OK")
			},
			options: []MWOption{OperationNameFunc(func(r *http.Request) string {
				return "HTTP " + r.Method + ": /root"
			})},
			expectedOperationName: "HTTP GET: /root",
			expectedSpanTags: []map[string]interface{}{
				{
					string(ext.Component):      defaultComponentName,
					string(ext.HTTPMethod):     "GET",
					string(ext.HTTPStatusCode): uint16(http.StatusOK),
					string(ext.HTTPUrl):        "/hello?token=secret",
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
					string(ext.HTTPUrl):        "/hello?token=secret",
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
			options: []MWOption{MWErrorFunc(func(ctx *gin.Context) bool {
				return ctx.Writer.Status() >= http.StatusNotFound
			})},
			expectedSpanTags: []map[string]interface{}{
				{
					string(ext.Component):      defaultComponentName,
					string(ext.HTTPMethod):     "GET",
					string(ext.HTTPStatusCode): uint16(http.StatusNotFound),
					string(ext.HTTPUrl):        "/hello?token=secret",
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
					string(ext.HTTPUrl):        "/hello?token=secret",
					string(ext.SpanKind):       ext.SpanKindRPCServerEnum,
					string(ext.Error):          true,
				},
			},
		},
	}

	for _, tt := range tests {
		testCase := tt
		t.Run(testCase.name, func(t *testing.T) {
			tracer := &mocktracer.MockTracer{}
			r := gin.New()
			r.Use(gin.Recovery(), Middleware(tracer, testCase.options...))
			r.GET("/hello", testCase.handler)
			srv := httptest.NewServer(r)
			defer srv.Close()

			_, err := http.Get(srv.URL + "/hello?token=secret")
			if err != nil {
				t.Fatalf("server returned error: %v", err)
			}

			var tags []map[string]interface{}

			op := testCase.expectedOperationName
			if op == "" {
				op = "HTTP GET"
			}

			for _, span := range tracer.FinishedSpans() {
				tags = append(tags, span.Tags())
				assert.Equal(t, op, span.OperationName)
			}
			assert.Equal(t, testCase.expectedSpanTags, tags)
		})
	}
}
