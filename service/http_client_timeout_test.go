package service

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturingRoundTripper struct {
	lastReq atomic.Pointer[http.Request]
}

func (c *capturingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.lastReq.Store(req)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestRelayTimeoutRoundTripperUsesCurrentSetting(t *testing.T) {
	setting := operation_setting.GetGeneralSetting()
	original := setting.RelayTimeoutSeconds
	t.Cleanup(func() {
		setting.RelayTimeoutSeconds = original
	})

	capture := &capturingRoundTripper{}
	client := newRelayHTTPClient(capture)

	setting.RelayTimeoutSeconds = 0
	resp, err := client.Get("http://example.invalid/unlimited")
	require.NoError(t, err)
	drainClose(t, resp)
	_, hasDeadline := capture.lastReq.Load().Context().Deadline()
	assert.False(t, hasDeadline)

	setting.RelayTimeoutSeconds = 45
	resp, err = client.Get("http://example.invalid/limited")
	require.NoError(t, err)
	drainClose(t, resp)
	_, hasDeadline = capture.lastReq.Load().Context().Deadline()
	assert.True(t, hasDeadline)
}

func TestRelayTimeoutRoundTripperCancelsOnError(t *testing.T) {
	setting := operation_setting.GetGeneralSetting()
	original := setting.RelayTimeoutSeconds
	t.Cleanup(func() {
		setting.RelayTimeoutSeconds = original
	})
	setting.RelayTimeoutSeconds = 30

	var captured *http.Request
	client := newRelayHTTPClient(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		captured = req
		return nil, io.ErrUnexpectedEOF
	}))

	_, err := client.Get("http://example.invalid/fail")
	require.Error(t, err)
	require.NotNil(t, captured)
	require.Error(t, captured.Context().Err())
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
