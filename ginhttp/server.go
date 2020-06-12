// +build go1.7

// This is the middleware from github.com/opentracing-contrib/go-stdlib
// tweaked slightly to work as a native gin middleware.
//
// It removes the need for the additional complexity of using a middleware
// adapter.

package ginhttp

import (
	"fmt"
	"github.com/stoewer/go-strcase"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

const defaultComponentName = "net/http"

type mwOptions struct {
	opNameFunc    func(r *http.Request) string
	spanObserver  func(span opentracing.Span, r *http.Request)
	urlTagFunc    func(u *url.URL) string
	errorFunc     func(ctx *gin.Context) bool
	componentName string
}

// MWOption controls the behavior of the Middleware.
type MWOption func(*mwOptions)

// OperationNameFunc returns a MWOption that uses given function f
// to generate operation name for each server-side span.
func OperationNameFunc(f func(r *http.Request) string) MWOption {
	if f == nil {
		panic("nil OperationNameFunc")
	}

	return func(options *mwOptions) {
		options.opNameFunc = f
	}
}

// MWComponentName returns a MWOption that sets the component name
// for the server-side span.
func MWComponentName(componentName string) MWOption {
	if componentName == "" {
		panic("empty componentName")
	}

	return func(options *mwOptions) {
		options.componentName = componentName
	}
}

// MWSpanObserver returns a MWOption that observe the span
// for the server-side span.
func MWSpanObserver(f func(span opentracing.Span, r *http.Request)) MWOption {
	if f == nil {
		panic("nil MWSpanObserver")
	}

	return func(options *mwOptions) {
		options.spanObserver = f
	}
}

// MWURLTagFunc returns a MWOption that uses given function f
// to set the span's http.url tag. Can be used to change the default
// http.url tag, eg to redact sensitive information.
func MWURLTagFunc(f func(u *url.URL) string) MWOption {
	if f == nil {
		panic("nil MWURLTagFunc")
	}

	return func(options *mwOptions) {
		options.urlTagFunc = f
	}
}

// MWErrorFunc returns a MWOption that sets the span error tag
func MWErrorFunc(f func(ctx *gin.Context) bool) MWOption {
	if f == nil {
		panic("nil MWErrorFunc")
	}

	return func(options *mwOptions) {
		options.errorFunc = f
	}
}

// Middleware is a gin native version of the equivalent middleware in:
//   https://github.com/opentracing-contrib/go-stdlib/
func Middleware(tr opentracing.Tracer, options ...MWOption) gin.HandlerFunc {
	opts := mwOptions{
		opNameFunc:   defaultOperationName,
		spanObserver: func(span opentracing.Span, r *http.Request) {},
		errorFunc: func(ctx *gin.Context) bool {
			return ctx.Writer.Status() >= http.StatusInternalServerError
		},
	}
	for _, opt := range options {
		opt(&opts)
	}

	return func(c *gin.Context) {
		carrier := opentracing.HTTPHeadersCarrier(c.Request.Header)
		ctx, _ := tr.Extract(opentracing.HTTPHeaders, carrier)
		op := opts.opNameFunc(c.Request)
		sp := tr.StartSpan(op, ext.RPCServerOption(ctx))
		ext.HTTPMethod.Set(sp, c.Request.Method)
		if opts.urlTagFunc != nil {
			ext.HTTPUrl.Set(sp, opts.urlTagFunc(c.Request.URL))
		} else {
			ext.HTTPUrl.Set(sp, urlTag(c))
		}
		setIp(c.Request.RemoteAddr, sp)

		opts.spanObserver(sp, c.Request)

		// set component name, use "net/http" if caller does not specify
		componentName := opts.componentName
		if componentName == "" {
			componentName = defaultComponentName
		}
		ext.Component.Set(sp, componentName)
		c.Request = c.Request.WithContext(
			opentracing.ContextWithSpan(c.Request.Context(), sp))

		defer recovery(sp)

		c.Next()

		if opts.errorFunc(c) {
			ext.Error.Set(sp, true)
		}
		ext.HTTPStatusCode.Set(sp, uint16(c.Writer.Status()))
		sp.Finish()
	}
}

func setIp(addr string, sp opentracing.Span) {
	idx := strings.LastIndex(addr, ":")
	if idx > 0 {
		addr = addr[:idx]
	}
	ip := net.ParseIP(addr)
	if ip != nil {
		if ip.To4() != nil {
			ext.PeerHostIPv4.SetString(sp, ip.String())
		} else {
			ext.PeerHostIPv6.Set(sp, ip.String())
		}
	}
}

func urlTag(c *gin.Context) string {
	var proto string
	if c.Request.TLS == nil {
		proto = "http"
	} else {
		proto = "https"
	}
	return fmt.Sprintf("%s://%s%s", proto, c.Request.Host, c.Request.URL.Path)
}

// DefaultOperationName is the default when tracer gets passed nil. It converts the
// URL path to CamelCase without a leading "api", e.g. "/api/v1//entities/" -> "V1Entities"
// or "/rest/kairosdbs/kairosdb/api/v1/datapoints/query" ->
// "RestKairosdbsKairosdbApiV1DatapointsQuery"
func defaultOperationName(r *http.Request) string {
	url := strings.Split(strings.Replace(r.URL.Path, "//", "/", -1)[1:], "/") // exclude leading "/"
	if url[0] == "api" {
		url = url[1:]
	}
	return strcase.UpperCamelCase(strings.Join(url, "_"))
}

func recovery(sp opentracing.Span) {
	if err := recover(); err != nil {
		ext.HTTPStatusCode.Set(sp, uint16(http.StatusInternalServerError))
		ext.Error.Set(sp, true)
		sp.Finish()
		panic(err)
	}
}
