package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func ClaudeHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {

	info.InitChannelMeta(c)

	claudeReq, ok := info.Request.(*dto.ClaudeRequest)

	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected *dto.ClaudeRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(claudeReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ClaudeRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	if request.MaxTokens == nil || *request.MaxTokens == 0 {
		defaultMaxTokens := uint(model_setting.GetClaudeSettings().GetDefaultMaxTokens(request.Model))
		request.MaxTokens = &defaultMaxTokens
	}

	if baseModel, effortLevel, ok := reasoning.TrimEffortSuffix(request.Model); ok && effortLevel != "" &&
		(strings.HasPrefix(request.Model, "claude-opus-4-6") ||
			strings.HasPrefix(request.Model, "claude-opus-4-7") ||
			strings.HasPrefix(request.Model, "claude-opus-4-8")) {
		request.Model = baseModel
		request.Thinking = &dto.Thinking{
			Type: "adaptive",
		}
		request.OutputConfig = json.RawMessage(fmt.Sprintf(`{"effort":"%s"}`, effortLevel))
		if isClaude47Or48Model(request.Model) {
			// Opus 4.7/4.8 reject non-default temperature/top_p/top_k with 400
			// and defaults display to "omitted"; restore the 4.6 visible summary.
			request.Thinking.Display = "summarized"
			request.Temperature = nil
			request.TopP = nil
			request.TopK = nil
		} else {
			request.Temperature = common.GetPointer[float64](1.0)
		}
		info.UpstreamModelName = request.Model
	} else if model_setting.GetClaudeSettings().ThinkingAdapterEnabled &&
		strings.HasSuffix(request.Model, "-thinking") {
		if request.Thinking == nil {
			baseModel := strings.TrimSuffix(request.Model, "-thinking")
			if isClaude47Or48Model(baseModel) {
				// Opus 4.7/4.8 reject thinking.type="enabled"; use adaptive at high effort.
				request.Thinking = &dto.Thinking{Type: "adaptive", Display: "summarized"}
				request.OutputConfig = json.RawMessage(`{"effort":"high"}`)
				request.Temperature = nil
				request.TopP = nil
				request.TopK = nil
			} else {
				// 因为BudgetTokens 必须大于1024
				if request.MaxTokens == nil || *request.MaxTokens < 1280 {
					request.MaxTokens = common.GetPointer[uint](1280)
				}

				// BudgetTokens 为 max_tokens 的 80%
				request.Thinking = &dto.Thinking{
					Type:         "enabled",
					BudgetTokens: common.GetPointer[int](int(float64(*request.MaxTokens) * model_setting.GetClaudeSettings().ThinkingAdapterBudgetTokensPercentage)),
				}
				// TODO: 临时处理
				// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#important-considerations-when-using-extended-thinking
				request.Temperature = common.GetPointer[float64](1.0)
			}
		}
		if !model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) {
			request.Model = strings.TrimSuffix(request.Model, "-thinking")
		}
		info.UpstreamModelName = request.Model
	}
	if isClaude47Or48Model(request.Model) {
		// Claude 4.7/4.8 family do not accept these sampling params on /v1/messages.
		request.Temperature = nil
		request.TopP = nil
		request.TopK = nil
	}

	if info.ChannelSetting.SystemPrompt != "" {
		if request.System == nil {
			request.SetStringSystem(info.ChannelSetting.SystemPrompt)
		} else if info.ChannelSetting.SystemPromptOverride {
			common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
			if request.IsStringSystem() {
				existing := strings.TrimSpace(request.GetStringSystem())
				if existing == "" {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt)
				} else {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt + "\n" + existing)
				}
			} else {
				systemContents := request.ParseSystem()
				newSystem := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
				newSystem.SetText(info.ChannelSetting.SystemPrompt)
				if len(systemContents) == 0 {
					request.System = []dto.ClaudeMediaMessage{newSystem}
				} else {
					request.System = append([]dto.ClaudeMediaMessage{newSystem}, systemContents...)
				}
			}
		}
	}

	if !model_setting.GetGlobalSettings().PassThroughRequestEnabled &&
		!info.ChannelSetting.PassThroughBodyEnabled &&
		service.ShouldChatCompletionsUseResponsesGlobal(info.ChannelId, info.ChannelType, info.OriginModelName) {
		openAIRequest, convErr := service.ClaudeToOpenAIRequest(*request, info)
		if convErr != nil {
			return types.NewError(convErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		usage, newApiErr := chatCompletionsViaResponses(c, info, adaptor, openAIRequest)
		if newApiErr != nil {
			return newApiErr
		}

		service.PostTextConsumeQuota(c, info, usage, nil)
		return nil
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	// buildRequestBody is called per attempt.
	// buildRequestBody 会在每次尝试前构建请求体。
	// attempt=0 uses pass-through raw body when enabled;
	// attempt=0 在开启透传时优先使用原始请求体。
	// attempt=1 forces marshal-from-struct so self-healed request changes are applied.
	// attempt=1 强制从 request struct 重新序列化，确保自愈后的改动生效。
	buildRequestBody := func(forceMarshalFromRequest bool) (io.Reader, func(), *types.NewAPIError) {
		if (model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled) && !forceMarshalFromRequest {
			storage, bodyErr := common.GetBodyStorage(c)
			if bodyErr != nil {
				return nil, nil, types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			info.UpstreamRequestBodySize = storage.Size()
			return common.ReaderOnly(storage), nil, nil
		}

		var outboundRequest any
		if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
			outboundRequest = request
		} else {
			convertedRequest, convErr := adaptor.ConvertClaudeRequest(c, info, request)
			if convErr != nil {
				return nil, nil, types.NewError(convErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
			outboundRequest = convertedRequest
		}

		jsonData, marshalErr := common.Marshal(outboundRequest)
		if marshalErr != nil {
			return nil, nil, types.NewError(marshalErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		if !model_setting.GetGlobalSettings().PassThroughRequestEnabled && !info.ChannelSetting.PassThroughBodyEnabled {
			jsonData, marshalErr = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
			if marshalErr != nil {
				return nil, nil, types.NewError(marshalErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}

			if len(info.ParamOverride) > 0 {
				jsonData, marshalErr = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
				if marshalErr != nil {
					return nil, nil, newAPIErrorFromParamOverride(marshalErr)
				}
			}
		}

		logger.LogDebug(c, "requestBody: %s", jsonData)
		body, size, closer, bodyErr := relaycommon.NewOutboundJSONBody(jsonData)
		if bodyErr != nil {
			return nil, nil, types.NewError(bodyErr, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		info.UpstreamRequestBodySize = size
		// buildRequestBody 约定返回 func()（无返回值）用于释放资源；
		// 这里将 closer.Close()（func() error）包一层，忽略关闭错误，避免类型不匹配。
		return body, func() { _ = closer.Close() }, nil
	}

	// Targeted self-healing retry:
	// 定向自愈重试：
	// only one extra retry (attempt=1) is allowed, and only when the first
	// upstream response is a 400 invalid thinking signature error.
	// 仅允许一次额外重试（attempt=1），且只在首次上游返回
	// 400 invalid thinking signature 时触发。
	for attempt := 0; attempt < 2; attempt++ {
		forceMarshalFromRequest := attempt > 0
		requestBody, release, bodyErr := buildRequestBody(forceMarshalFromRequest)
		if bodyErr != nil {
			return bodyErr
		}

		resp, doReqErr := adaptor.DoRequest(c, info, requestBody)
		if release != nil {
			release()
		}
		if doReqErr != nil {
			return types.NewOpenAIError(doReqErr, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
		}

		if resp == nil {
			break
		}
		httpResp := resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			if attempt == 0 {
				// Self-heal the request body for invalid thinking signature errors,
				// then resend once with rebuilt JSON body.
				// 命中 invalid thinking signature 错误时先修复请求体，
				// 再用重建后的 JSON 请求体补发一次。
				// Use original upstream status code for self-healing decision.
				// status_code_mapping may rewrite newAPIError.StatusCode and should not
				// affect whether we retry/fix the outbound body.
				changed := service.FixClaudeRequestOnFirstRetry(request, httpResp.StatusCode, newAPIError.Error())
				if changed > 0 {
					// logger.LogInfo 不是 printf 风格，先格式化字符串再传入。
					logger.LogInfo(c, fmt.Sprintf("retry claude request after stripping invalid thinking blocks, stripped_count=%d", changed))
					continue
				}
			}
			return newAPIError
		}

		usage, handleRespErr := adaptor.DoResponse(c, httpResp, info)
		if handleRespErr != nil {
			// reset status code 重置状态码
			service.ResetStatusCode(handleRespErr, statusCodeMappingStr)
			return handleRespErr
		}

		service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
		return nil
	}

	return types.NewErrorWithStatusCode(fmt.Errorf("empty upstream response"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
}

func isClaude47Or48Model(model string) bool {
	baseModel, _, _ := reasoning.TrimEffortSuffix(model)
	baseModel = strings.TrimSuffix(baseModel, "-thinking")
	return strings.Contains(baseModel, "4-7") || strings.Contains(baseModel, "4-8")
}
