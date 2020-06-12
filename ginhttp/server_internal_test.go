package ginhttp

import (
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/mocktracer"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSetIp(t *testing.T) {
	tests := []struct {
		name             string
		addr             string
		expectedSpanTags map[string]interface{}
	}{
		{
			name: "IPV4",
			addr: "192.168.0.1:124",
			expectedSpanTags: map[string]interface{}{
				string(ext.PeerHostIPv4): "192.168.0.1",
			},
		},
		{
			name: "IPV6",
			addr: "2001:db8::68:124",
			expectedSpanTags: map[string]interface{}{
				string(ext.PeerHostIPv6): "2001:db8::68",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracer := mocktracer.MockTracer{}
			span := tracer.StartSpan("op")
			setIp(tt.addr, span)
			span.Finish()
			assert.Equal(t, tt.expectedSpanTags, tracer.FinishedSpans()[0].Tags())
		})
	}
}
