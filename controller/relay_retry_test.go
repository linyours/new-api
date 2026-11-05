package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestShouldRetrySkipsTimeoutDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	timeout := types.NewError(context.DeadlineExceeded, types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry(), types.ErrOptionWithStatusCode(http.StatusGatewayTimeout), types.ErrOptionWithHideErrMsg("upstream error: do request failed"))
	assert.False(t, service.ShouldRetryRelayError(c, timeout, 3))

	hiddenTimeout := types.NewError(errors.New("upstream error: do request failed"), types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry(), types.ErrOptionWithStatusCode(http.StatusGatewayTimeout))
	assert.False(t, service.ShouldRetryRelayError(c, hiddenTimeout, 3))

	refused := types.NewError(errors.New("connection refused"), types.ErrorCodeDoRequestFailed)
	assert.True(t, service.ShouldRetryRelayError(c, refused, 3))

	serverErr := types.NewOpenAIError(errors.New("upstream 500"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)
	assert.True(t, service.ShouldRetryRelayError(c, serverErr, 3))
}

func TestShouldRetryTaskRelaySkipsTimeoutDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	assert.False(t, decideTaskRetry(c, &taskdto.TaskError{
		StatusCode: http.StatusInternalServerError,
		Message:    "context deadline exceeded",
		Error:      context.DeadlineExceeded,
	}, 3).Action == "retry")
	assert.False(t, decideTaskRetry(c, &taskdto.TaskError{
		StatusCode: http.StatusGatewayTimeout,
		Message:    "gateway timeout",
	}, 3).Action == "retry")
	assert.True(t, decideTaskRetry(c, &taskdto.TaskError{
		StatusCode: http.StatusInternalServerError,
		Message:    "connection refused",
		Error:      errors.New("connection refused"),
	}, 3).Action == "retry")
}
